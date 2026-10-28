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
		io.WriteString(w, `{"ok":false,"error":{"code":"last_user_forbidden","message":"cannot delete last user"},"request_id":1}`)
	})
	err := c.deleteUser("admin")
	if err == nil || !strings.Contains(err.Error(), "last_user_forbidden") {
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
