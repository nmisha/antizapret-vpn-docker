package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestResetProfileRiskStateRemovesStoredState(t *testing.T) {
	cfg := Config{
		StatePath: t.TempDir() + "/state.json",
	}
	state := &State{
		Profiles: map[string]ProfileRiskState{
			"wg:alice": {
				LastProfileIP:      "10.1.166.10",
				LastMatchedDomain:  "max.ru",
				LastAction:         "block_pending",
				PendingBlockAt:     "2026-04-18T10:15:00Z",
				PendingBlockWindow: "15m",
				PendingBlockScore:  50,
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

	resp, ok, err := resetProfileRiskState(cfg, "wg", "Alice", mustParseRFC3339(t, "2026-04-18T10:12:00Z"))
	if err != nil {
		t.Fatalf("reset state: %v", err)
	}
	if !ok {
		t.Fatal("expected profile state to be reset")
	}
	if !resp.HadState || !resp.Reset {
		t.Fatalf("unexpected reset response: %+v", resp)
	}

	reloaded, err := loadState(cfg.StatePath)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if _, exists := reloaded.Profiles["wg:alice"]; exists {
		t.Fatalf("expected profile state to be removed, got %+v", reloaded.Profiles["wg:alice"])
	}
}

func TestResetProfileRiskStateReturnsNoopForExistingProfileWithoutState(t *testing.T) {
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
		StatePath: t.TempDir() + "/state.json",
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

	resp, ok, err := resetProfileRiskState(cfg, "wg", "Alice", mustParseRFC3339(t, "2026-04-18T10:12:00Z"))
	if err != nil {
		t.Fatalf("reset state: %v", err)
	}
	if !ok {
		t.Fatal("expected existing profile reset to succeed")
	}
	if resp.HadState {
		t.Fatalf("expected reset without stored state, got %+v", resp)
	}
}

func TestResetProfileRiskStateReturnsNotFound(t *testing.T) {
	cfg := Config{
		StatePath: t.TempDir() + "/state.json",
	}
	if err := saveState(cfg.StatePath, &State{Profiles: map[string]ProfileRiskState{}}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	_, ok, err := resetProfileRiskState(cfg, "wg", "missing", mustParseRFC3339(t, "2026-04-18T10:12:00Z"))
	if err != nil {
		t.Fatalf("reset state: %v", err)
	}
	if ok {
		t.Fatal("expected missing profile reset to return not found")
	}
}

func TestProfileRiskResetHandler(t *testing.T) {
	cfg := Config{
		StatePath: t.TempDir() + "/state.json",
	}
	state := &State{
		Profiles: map[string]ProfileRiskState{
			"wg:alice": {
				LastProfileIP:      "10.1.166.10",
				LastMatchedDomain:  "max.ru",
				PendingBlockAt:     "2026-04-18T10:15:00Z",
				PendingBlockWindow: "15m",
				PendingBlockScore:  50,
				Buckets: map[string]int{
					"2026-04-18T10:10:00Z": 15,
				},
			},
		},
	}
	if err := saveState(cfg.StatePath, state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	token := "secret-token"
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/profile-risk/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !matchBearerToken(r.Header.Get("Authorization"), token) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		resp, ok, err := resetProfileRiskState(cfg, r.URL.Query().Get("kind"), r.URL.Query().Get("name"), mustParseRFC3339(t, "2026-04-18T10:12:00Z"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "profile not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/profile-risk/reset?kind=wg&name=Alice", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var resp profileRiskResetAPIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Reset || !resp.HadState || resp.ProfileKey != "wg:alice" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	raw, err := os.ReadFile(cfg.StatePath)
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}
	if strings.Contains(string(raw), "wg:alice") {
		t.Fatalf("expected state file without profile entry, got %s", string(raw))
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
