package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBuildProfileRiskResponse(t *testing.T) {
	t.Setenv("DNS_GUARD_STATE", "")
	cfg := Config{
		StatePath:       t.TempDir() + "/state.json",
		ScoreNotifyAt:   25,
		ScoreBlockAt15m: 50,
		ScoreBlockAt24h: 100,
	}
	state := &State{
		Profiles: map[string]ProfileRiskState{
			"wg:alice": {
				LastProfileIP:     "10.1.166.10",
				LastMatchedDomain: "max.ru",
				Buckets: map[string]int{
					"2026-04-18T10:00:00Z": 20,
					"2026-04-18T10:10:00Z": 15,
				},
			},
		},
	}
	if err := saveState(cfg.StatePath, state); err != nil {
		t.Fatalf("save state: %v", err)
	}
	resp, ok, err := buildProfileRiskResponse(cfg, "wg", "alice", mustParseRFC3339(t, "2026-04-18T10:12:00Z"))
	if err != nil {
		t.Fatalf("build response: %v", err)
	}
	if !ok {
		t.Fatal("expected profile to be found")
	}
	if resp.Score15m != 35 || resp.Score24h != 35 || resp.EffectiveScore != 35 {
		t.Fatalf("unexpected scores: %+v", resp)
	}
	if resp.BlockThreshold15m != 50 || resp.BlockThreshold24h != 100 {
		t.Fatalf("unexpected thresholds: %+v", resp)
	}
	if resp.PercentToBlock15m != 70.0 || resp.PercentToCritical != 70.0 {
		t.Fatalf("unexpected critical percent: %+v", resp)
	}
}

func TestBuildProfileRiskResponseReturnsZeroForExistingProfileWithoutState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/client" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "1", "name": "Alice", "ipv4Address": "10.1.166.10"},
		})
	}))
	defer srv.Close()

	hostPort := strings.TrimPrefix(srv.URL, "http://")
	parts := strings.SplitN(hostPort, ":", 2)
	if len(parts) != 2 {
		t.Fatalf("unexpected test server url: %s", srv.URL)
	}

	cfg := Config{
		StatePath:       t.TempDir() + "/state.json",
		ScoreNotifyAt:   25,
		ScoreBlockAt15m: 50,
		ScoreBlockAt24h: 100,
		WG: GuardAPIConfig{
			Enabled:  true,
			Host:     parts[0],
			Port:     parts[1],
			Username: "admin",
			Password: "admin",
		},
	}
	if err := saveState(cfg.StatePath, &State{Profiles: map[string]ProfileRiskState{}}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	resp, ok, err := buildProfileRiskResponse(cfg, "wg", "Alice", mustParseRFC3339(t, "2026-04-18T10:12:00Z"))
	if err != nil {
		t.Fatalf("build response: %v", err)
	}
	if !ok {
		t.Fatal("expected existing profile to be returned")
	}
	if resp.EffectiveScore != 0 || resp.Score15m != 0 || resp.Score24h != 0 {
		t.Fatalf("expected zero scores, got %+v", resp)
	}
}

func mustParseRFC3339(t *testing.T, v string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, v)
	if err != nil {
		t.Fatalf("parse time: %v", err)
	}
	return ts
}
