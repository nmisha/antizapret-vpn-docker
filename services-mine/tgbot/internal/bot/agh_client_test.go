package bot

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAghRefreshFilters(t *testing.T) {
	for _, whitelist := range []bool{false, true} {
		t.Run(map[bool]string{false: "blocklists", true: "whitelists"}[whitelist], func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				user, pass, ok := r.BasicAuth()
				if r.Method != http.MethodPost || r.URL.Path != "/control/filtering/refresh" || !ok || user != "admin" || pass != "secret" {
					t.Error("unexpected refresh request")
				}
				var body aghRefreshReq
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Whitelist != whitelist {
					t.Errorf("unexpected request body: %+v, %v", body, err)
				}
				_, _ = w.Write([]byte(`{"updated":3}`))
			}))
			defer srv.Close()
			c := &aghClient{baseURL: srv.URL, login: "admin", pass: "secret", hc: srv.Client()}
			if updated, err := c.refreshFilters(whitelist); err != nil || updated != 3 {
				t.Fatalf("refresh = %d, %v", updated, err)
			}
		})
	}
}

func TestAghRefreshTimeoutReportsUnknownOutcome(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(200 * time.Millisecond):
		}
	}))
	defer srv.Close()
	c := &aghClient{baseURL: srv.URL, hc: &http.Client{Timeout: 30 * time.Millisecond}}
	_, err := c.refreshFilters(false)
	if err == nil || !strings.Contains(err.Error(), "может продолжать обновление") {
		t.Fatalf("expected uncertain refresh outcome, got %v", err)
	}
}

func TestAghDefaultTimeout(t *testing.T) {
	for k, v := range map[string]string{"AGH_HOST": "localhost", "AGH_PORT": "3000", "AGH_LOGIN": "admin", "AGH_PASSWORD": "secret", "AGH_TIMEOUT_SECONDS": ""} {
		t.Setenv(k, v)
	}
	c, err := newAghClientFromEnv()
	if err != nil || c.hc.Timeout != 15*time.Minute {
		t.Fatalf("unexpected client: %+v, %v", c, err)
	}
	t.Setenv("AGH_TIMEOUT_SECONDS", "42")
	c, err = newAghClientFromEnv()
	if err != nil || c.hc.Timeout != 42*time.Second {
		t.Fatalf("unexpected timeout override: %+v, %v", c, err)
	}
}
