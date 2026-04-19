package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestAdGuardQueryLogSourceSyncMarksCurrentHead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/control/querylog"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		_ = json.NewEncoder(w).Encode(adGuardQueryLogResponse{
			Oldest: "2026-04-19T10:00:00Z",
			Data: []adGuardQueryLogEntry{
				makeAPIEntry("10.1.166.10", "2026-04-19T10:01:00Z", "a.example", "A", "rule-a"),
				makeAPIEntry("10.1.166.11", "2026-04-19T10:01:00Z", "b.example", "A", "rule-b"),
				makeAPIEntry("10.1.166.12", "2026-04-19T10:00:00Z", "c.example", "AAAA", "rule-c"),
			},
		})
	}))
	defer srv.Close()

	cfg := testAdGuardConfig(srv.URL)
	source := newAdGuardQueryLogSource(cfg)
	cursor := CursorState{}
	if err := source.Sync(&cursor); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if cursor.LastSeenTime != "2026-04-19T10:01:00Z" {
		t.Fatalf("last_seen_time = %q", cursor.LastSeenTime)
	}
	if len(cursor.RecentEventKeys) != 2 {
		t.Fatalf("recent keys = %d, want 2", len(cursor.RecentEventKeys))
	}
	if cursor.Offset != 0 || cursor.FileSize != 0 || cursor.FileModTime != "" {
		t.Fatalf("expected file cursor fields to be cleared, got %+v", cursor)
	}
}

func TestAdGuardQueryLogSourceProcessLoadsOnlyNewEntries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("older_than") {
		case "":
			_ = json.NewEncoder(w).Encode(adGuardQueryLogResponse{
				Oldest: "2026-04-19T10:01:00Z",
				Data: []adGuardQueryLogEntry{
					makeAPIEntry("10.1.166.12", "2026-04-19T10:03:00Z", "newest.example", "A", "rule-3"),
					makeAPIEntry("10.1.166.11", "2026-04-19T10:02:00Z", "newer.example", "A", "rule-2"),
					makeAPIEntry("10.1.166.10", "2026-04-19T10:01:00Z", "same-ts.example", "A", "rule-1"),
				},
			})
		case "2026-04-19T10:01:00Z":
			_ = json.NewEncoder(w).Encode(adGuardQueryLogResponse{
				Oldest: "2026-04-19T09:59:00Z",
				Data: []adGuardQueryLogEntry{
					makeAPIEntry("10.1.166.9", "2026-04-19T10:00:00Z", "old.example", "A", "rule-0"),
				},
			})
		default:
			t.Fatalf("unexpected older_than=%q", r.URL.Query().Get("older_than"))
		}
	}))
	defer srv.Close()

	cfg := testAdGuardConfig(srv.URL)
	source := newAdGuardQueryLogSource(cfg)
	cursor := CursorState{
		LastSeenTime: "2026-04-19T10:01:00Z",
		RecentEventKeys: []string{
			"2026-04-19T10:01:00Z|10.1.166.99|other.example|A",
		},
	}

	var got []string
	err := source.Process(&cursor, func(entry QueryLogEntry) error {
		got = append(got, entry.T+"|"+entry.IP+"|"+entry.QH+"|"+entry.QT)
		return nil
	})
	if err != nil {
		t.Fatalf("process: %v", err)
	}

	want := []string{
		"2026-04-19T10:01:00Z|10.1.166.10|same-ts.example|A",
		"2026-04-19T10:02:00Z|10.1.166.11|newer.example|A",
		"2026-04-19T10:03:00Z|10.1.166.12|newest.example|A",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestLoadConfigAcceptsAPIQueryLogSource(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.json"
	raw := `{
		"enabled": true,
		"querylog_source": "api",
		"state_path": "/state/state.json",
		"notification_api_url": "http://example",
		"adguard": {
			"host": "adguard.antizapret",
			"port": "3000",
			"username": "admin",
			"password": "secret"
		},
		"wg": {"enabled": true},
		"awg": {"enabled": true},
		"ovpn": {"enabled": true}
	}`
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, _, err := loadConfigFromPath(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got := queryLogSource(cfg); got != "api" {
		t.Fatalf("querylog source = %q, want api", got)
	}
	if cfg.AdGuard.PageLimit != 500 || cfg.AdGuard.TimeoutSeconds != 15 {
		t.Fatalf("unexpected adguard defaults: %+v", cfg.AdGuard)
	}
}

func makeAPIEntry(client, ts, name, qtype, rule string) adGuardQueryLogEntry {
	var entry adGuardQueryLogEntry
	entry.Client = client
	entry.Time = ts
	entry.Question.Name = name
	entry.Question.Type = qtype
	entry.Rules = []struct {
		Text string `json:"text"`
	}{{Text: rule}}
	return entry
}

func testAdGuardConfig(serverURL string) AdGuardAPIConfig {
	hostPort := strings.TrimPrefix(serverURL, "http://")
	parts := strings.SplitN(hostPort, ":", 2)
	return AdGuardAPIConfig{
		Scheme:         "http",
		Host:           parts[0],
		Port:           parts[1],
		Username:       "u",
		Password:       "p",
		PageLimit:      3,
		TimeoutSeconds: 5,
	}
}
