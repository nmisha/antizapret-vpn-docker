package bot

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type dnsGuardProfileRisk struct {
	ProfileKind              string  `json:"profile_kind"`
	ProfileName              string  `json:"profile_name"`
	ProfileKey               string  `json:"profile_key"`
	LastProfileIP            string  `json:"last_profile_ip,omitempty"`
	LastMatchedDomain        string  `json:"last_matched_domain,omitempty"`
	LastMatchedReason        string  `json:"last_matched_reason,omitempty"`
	LastAction               string  `json:"last_action,omitempty"`
	LastBlockAt              string  `json:"last_block_at,omitempty"`
	PendingBlockAt           string  `json:"pending_block_at,omitempty"`
	Score15m                 int     `json:"score_15m"`
	Score24h                 int     `json:"score_24h"`
	EffectiveScore           int     `json:"effective_score"`
	NotifyThreshold          int     `json:"notify_threshold"`
	BlockThreshold15m        int     `json:"block_threshold_15m"`
	BlockThreshold24h        int     `json:"block_threshold_24h"`
	PercentToNotify          float64 `json:"percent_to_notify"`
	PercentToBlock15m        float64 `json:"percent_to_block_15m"`
	PercentToBlock24h        float64 `json:"percent_to_block_24h"`
	PercentToCritical        float64 `json:"percent_to_critical"`
	CriticalWindow           string  `json:"critical_window"`
	CriticalThreshold        int     `json:"critical_threshold"`
	CriticalThresholdReached bool    `json:"critical_threshold_reached"`
	GeneratedAt              string  `json:"generated_at"`
}

type dnsGuardProfileRiskReset struct {
	ProfileKind string `json:"profile_kind"`
	ProfileName string `json:"profile_name"`
	ProfileKey  string `json:"profile_key"`
	HadState    bool   `json:"had_state"`
	Reset       bool   `json:"reset"`
	GeneratedAt string `json:"generated_at"`
}

func fetchDNSGuardProfileRisk(kind, name string) (*dnsGuardProfileRisk, error) {
	return doDNSGuardProfileRequest[dnsGuardProfileRisk](http.MethodGet, "/api/v1/profile-risk", kind, name)
}

func resetDNSGuardProfileRisk(kind, name string) (*dnsGuardProfileRiskReset, error) {
	return doDNSGuardProfileRequest[dnsGuardProfileRiskReset](http.MethodPost, "/api/v1/profile-risk/reset", kind, name)
}

func doDNSGuardProfileRequest[T any](method, path, kind, name string) (*T, error) {
	baseURL := envTrim("DNS_GUARD_API_URL")
	if baseURL == "" {
		baseURL = "http://dns-guard.antizapret:8090"
	}
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return nil, fmt.Errorf("invalid DNS_GUARD_API_URL: %w", err)
	}
	u.Path = path
	q := u.Query()
	q.Set("kind", strings.ToLower(strings.TrimSpace(kind)))
	q.Set("name", strings.TrimSpace(name))
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(method, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if token := envTrim("DNS_GUARD_API_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("dns-guard api failed: %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var out T
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}
