package bot

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestTelemtClient(t *testing.T, h http.HandlerFunc) *telemtClient {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &telemtClient{BaseURL: srv.URL + "/v1", Token: "tok", http: srv.Client()}
}

func TestTelemtClientListUsers(t *testing.T) {
	c := newTestTelemtClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "tok" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Method != http.MethodGet || r.URL.Path != "/v1/users" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		io.WriteString(w, `{"ok":true,"revision":"x","data":[{"username":"vasya","enabled":true,
			"max_tcp_conns":null,"max_unique_ips":2,"expiration_rfc3339":null,"current_connections":5,
			"active_unique_ips":1,"active_unique_ips_list":["1.2.3.4"],"total_octets":1048576,
			"links":{"classic":[],"secure":[],"tls":["tg://proxy?server=tp.example.com&port=443&secret=ee00"],"tls_domains":[]}}]}`)
	})
	users, err := c.listUsers()
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Fatalf("got %d users", len(users))
	}
	u := users[0]
	if u.Username != "vasya" || u.MaxTCPConns != nil || u.MaxUniqueIPs == nil || *u.MaxUniqueIPs != 2 || u.TotalOctets != 1048576 {
		t.Errorf("bad decode: %+v", u)
	}
	if got := telemtWebLink(u.TLSLink()); got != "https://t.me/proxy?server=tp.example.com&port=443&secret=ee00" {
		t.Errorf("web link = %q", got)
	}
}

func TestTelemtClientError(t *testing.T) {
	c := newTestTelemtClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		io.WriteString(w, `{"ok":false,"error":{"code":"revision_conflict","message":"config changed"},"request_id":1}`)
	})
	err := c.resetQuota("admin")
	if err == nil || !strings.Contains(err.Error(), "revision_conflict") {
		t.Fatalf("err = %v", err)
	}
}

func TestTelemtClientCreateAndPatch(t *testing.T) {
	var gotBody map[string]any
	c := newTestTelemtClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/users":
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"ok":true,"data":{"user":{"username":"new","enabled":true,"links":{"tls":["tg://proxy?a=1"]}},"secret":"00"}}`)
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/users/new":
			io.WriteString(w, `{"ok":true,"data":{"username":"new","enabled":true}}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})

	u, err := c.createUser("new")
	if err != nil || u.Username != "new" || u.TLSLink() != "tg://proxy?a=1" {
		t.Fatalf("create: %+v, %v", u, err)
	}
	if gotBody["username"] != "new" {
		t.Errorf("create body = %v", gotBody)
	}

	if _, err := c.patchUser("new", map[string]any{"max_unique_ips": nil}); err != nil {
		t.Fatal(err)
	}
	if v, ok := gotBody["max_unique_ips"]; !ok || v != nil {
		t.Errorf("patch must send explicit null, body = %v", gotBody)
	}
}

func TestParseMtpLimitValue(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	field := func(k string) mtpLimitField { f, _ := mtpLimitFieldByKey(k); return f }

	tests := []struct {
		key, in string
		want    any
		wantErr bool
	}{
		{"ips", "2", uint64(2), false},
		{"ips", "0", nil, true},
		{"conns", "abc", nil, true},
		{"ips", "none", nil, false},
		{"quota", "10G", uint64(10 << 30), false},
		{"quota", "500m", uint64(500 << 20), false},
		{"up", "10M", uint64(10_000_000), false},
		{"down", "512K", uint64(512_000), false},
		{"exp", "2026-12-31", "2026-12-31T23:59:59Z", false},
		{"exp", "2027-01-01T00:00:00+03:00", "2026-12-31T21:00:00Z", false},
		{"exp", "30d", "2026-11-27T12:00:00Z", false},
		{"exp", "tomorrow", nil, true},
	}
	for _, tt := range tests {
		got, err := parseMtpLimitValue(field(tt.key), tt.in, now)
		if (err != nil) != tt.wantErr {
			t.Errorf("%s=%q: err = %v", tt.key, tt.in, err)
			continue
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("%s=%q: got %#v, want %#v", tt.key, tt.in, got, tt.want)
		}
	}
}

func TestFilterMtpUsersByPrefixes(t *testing.T) {
	users := []telemtUser{{Username: "Vasya_phone"}, {Username: "petya"}, {Username: "admin"}}
	got := filterMtpUsersByPrefixes(users, []string{"vasya", "petya"})
	if len(got) != 2 || got[0].Username != "Vasya_phone" || got[1].Username != "petya" {
		t.Errorf("got %+v", got)
	}
	if len(filterMtpUsersByPrefixes(users, nil)) != 0 {
		t.Error("no prefixes must match nothing")
	}
}

func TestTelemtQuotaPeriodAndRemoval(t *testing.T) {
	var requests []map[string]any
	c := newTestTelemtClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/v1/users/alice" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, body)
		io.WriteString(w, `{"ok":true,"data":{"username":"alice","data_quota_bytes":1048576,"quota_period":"monthly"}}`)
	})
	quota, _ := mtpLimitFieldByKey("quota")
	period, _ := mtpLimitFieldByKey("period")
	u, err := c.patchUser("alice", mtpLimitPatch(period, "monthly"))
	if err != nil || u.QuotaPeriod == nil || *u.QuotaPeriod != "monthly" {
		t.Fatalf("decode: %+v, %v", u, err)
	}
	if _, err := c.patchUser("alice", mtpLimitPatch(quota, nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := c.patchUser("alice", mtpLimitPatch(period, nil)); err != nil {
		t.Fatal(err)
	}
	if len(requests[0]) != 1 || requests[0]["quota_period"] != "monthly" {
		t.Fatalf("period: %v", requests[0])
	}
	for _, key := range []string{"data_quota_bytes", "quota_period"} {
		if value, ok := requests[1][key]; !ok || value != nil {
			t.Fatalf("quota removal: %v", requests[1])
		}
	}
	if len(requests[2]) != 1 {
		t.Fatalf("disabling schedule must preserve quota: %v", requests[2])
	}
	if value, ok := requests[2]["quota_period"]; !ok || value != nil {
		t.Fatalf("schedule removal: %v", requests[2])
	}
}

func TestTelemtResetQuotaTargetsOneUser(t *testing.T) {
	calls := 0
	c := newTestTelemtClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.Path != "/v1/users/alice/reset-quota" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if len(body) != 0 {
			t.Errorf("reset must not patch quota settings: %s", body)
		}
		io.WriteString(w, `{"ok":true,"data":{"username":"alice","used_bytes":0}}`)
	})
	if err := c.resetQuota("alice"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("requests = %d", calls)
	}
}

func TestParseMtpQuotaPeriod(t *testing.T) {
	field, _ := mtpLimitFieldByKey("period")
	for _, input := range []string{"daily", "weekly", "monthly", " MONTHLY ", "none", "-", "yearly", ""} {
		got, err := parseMtpLimitValue(field, input, time.Now())
		switch input {
		case "yearly", "":
			if err == nil {
				t.Errorf("accepted %q", input)
			}
		case "none", "-":
			if err != nil || got != nil {
				t.Errorf("%q: %v %v", input, got, err)
			}
		default:
			if err != nil || got != strings.ToLower(strings.TrimSpace(input)) {
				t.Errorf("%q: %v %v", input, got, err)
			}
		}
	}
}

func TestTelemtQuotaView(t *testing.T) {
	limit := uint64(1024)
	for _, tc := range []struct {
		name, response string
		status         int
		wantQuota      bool
	}{
		{"selected", `{"ok":true,"data":{"users":[{"username":"other","data_quota_bytes":9999,"used_bytes":9999},{"username":"alice","data_quota_bytes":1024,"used_bytes":256,"quota_next_reset_epoch_secs":1793491200,"quota_schedule_state":"active"}]}}`, 200, true},
		{"missing", `{"ok":true,"data":{"users":[{"username":"other","used_bytes":9999}]}}`, 200, false},
		{"unavailable", `{"ok":false,"error":{"code":"not_found"}}`, 404, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestTelemtClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/stats/users/quota" || r.Header.Get("Authorization") != "tok" {
					t.Errorf("unexpected quota request: %s %s", r.Method, r.URL.Path)
				}
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.response)
			})
			u := c.withQuota(telemtUser{Username: "alice", Enabled: true, DataQuotaBytes: &limit, TotalOctets: 99999})
			if (u.Quota != nil) != tc.wantQuota {
				t.Fatalf("quota = %+v", u.Quota)
			}
			text := formatMtpUserInfo(u, false, time.Now())
			if tc.wantQuota {
				if u.Quota.UsedBytes != 256 || u.Quota.NextReset == nil {
					t.Fatalf("decode: %+v", u.Quota)
				}
				for _, want := range []string{"Расход квоты:", "Осталось:", "Следующий сброс:", "UTC"} {
					if !strings.Contains(text, want) {
						t.Errorf("missing %q: %s", want, text)
					}
				}
			} else if !strings.Contains(text, "Расход квоты: недоступен") {
				t.Errorf("fallback: %s", text)
			}
		})
	}
}

func TestTelemtQuotaViewWithoutLimitAndOverage(t *testing.T) {
	c := newTestTelemtClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("must not fetch quotas without a quota limit") })
	u := c.withQuota(telemtUser{Username: "alice"})
	if strings.Contains(formatMtpUserInfo(u, false, time.Now()), "Расход квоты") {
		t.Error("quota displayed without limit")
	}
	limit := uint64(100)
	u.DataQuotaBytes = &limit
	u.Quota = &telemtQuota{DataQuotaBytes: 100, UsedBytes: 200, ScheduleState: "overdue"}
	text := formatMtpUserInfo(u, false, time.Now())
	if !strings.Contains(text, "Осталось: "+humanBytesIEC(0)) || !strings.Contains(text, "задерживается") {
		t.Fatalf("overage: %s", text)
	}
}
