package bot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type aghClient struct {
	baseURL string
	login   string
	pass    string
	hc      *http.Client
}

// Filtering status response is not stable across all AGH builds, so we parse it defensively.
// We try to extract last update timestamps for blocklists and whitelist filters.
func (c *aghClient) lastFilterUpdateTimes() (blocklists time.Time, whitelists time.Time, err error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/control/filtering/status", nil)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	req.SetBasicAuth(c.login, c.pass)
	resp, err := c.hc.Do(req)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20)) // 2MB
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(bodyBytes))
		if msg == "" {
			msg = resp.Status
		}
		return time.Time{}, time.Time{}, fmt.Errorf("AdGuard API error: %s", msg)
	}

	var root map[string]any
	if err := json.Unmarshal(bodyBytes, &root); err != nil {
		return time.Time{}, time.Time{}, err
	}

	blocklists = maxTimeFromArray(root["filters"])
	// Some builds may use different key names
	if blocklists.IsZero() {
		blocklists = maxTimeFromArray(root["block_filters"])
	}

	whitelists = maxTimeFromArray(root["whitelist_filters"])
	if whitelists.IsZero() {
		whitelists = maxTimeFromArray(root["whitelistFilters"])
	}

	return blocklists, whitelists, nil
}

func maxTimeFromArray(v any) time.Time {
	arr, ok := v.([]any)
	if !ok {
		return time.Time{}
	}
	var max time.Time
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		t := extractAnyTimestamp(m)
		if t.After(max) {
			max = t
		}
	}
	return max
}

func extractAnyTimestamp(m map[string]any) time.Time {
	// Common keys seen in different AGH builds
	keys := []string{"last_updated", "last_update", "lastUpdated", "lastUpdate", "updated", "update_time", "updateTime"}
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if t, ok := parseAghTime(v); ok {
				return t
			}
		}
	}
	// Some builds nest it
	for _, v := range m {
		if sub, ok := v.(map[string]any); ok {
			if t := extractAnyTimestamp(sub); !t.IsZero() {
				return t
			}
		}
	}
	return time.Time{}
}

func parseAghTime(v any) (time.Time, bool) {
	switch t := v.(type) {
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return time.Time{}, false
		}
		// Try RFC3339 / RFC3339Nano
		if tt, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return tt, true
		}
		if tt, err := time.Parse(time.RFC3339, s); err == nil {
			return tt, true
		}
		// Some builds may return "2006-01-02 15:04:05"
		if tt, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
			return tt, true
		}
		return time.Time{}, false
	case float64:
		// unix seconds
		if t <= 0 {
			return time.Time{}, false
		}
		return time.Unix(int64(t), 0), true
	case int64:
		if t <= 0 {
			return time.Time{}, false
		}
		return time.Unix(t, 0), true
	case int:
		if t <= 0 {
			return time.Time{}, false
		}
		return time.Unix(int64(t), 0), true
	default:
		return time.Time{}, false
	}
}

func newAghClientFromEnv() (*aghClient, error) {
	host := envTrim("AGH_HOST")
	port := envTrim("AGH_PORT")
	login := envTrim("AGH_LOGIN")
	pass := envTrim("AGH_PASSWORD")
	scheme := envTrim("AGH_SCHEME")
	timeoutEnv := envTrim("AGH_TIMEOUT_SECONDS")
	if scheme == "" {
		scheme = "http"
	}
	if host == "" || port == "" || login == "" || pass == "" {
		return nil, fmt.Errorf("missing env: AGH_HOST, AGH_PORT, AGH_LOGIN, AGH_PASSWORD")
	}
	host = strings.TrimSpace(host)
	port = strings.TrimSpace(port)

	base := fmt.Sprintf("%s://%s:%s", scheme, host, port)

	// Updating filter lists can be slow (download + parsing).
	// Default timeout is 5 minutes; override via AGH_TIMEOUT_SECONDS.
	timeout := 5 * time.Minute
	if timeoutEnv != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(timeoutEnv)); err == nil && n > 0 {
			timeout = time.Duration(n) * time.Second
		}
	}
	return &aghClient{
		baseURL: base,
		login:   login,
		pass:    pass,
		hc: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

type aghRefreshReq struct {
	Whitelist bool `json:"whitelist"`
}

type aghRefreshResp struct {
	Updated int `json:"updated"`
}

func (c *aghClient) refreshFilters(whitelist bool) (int, error) {
	reqBody, _ := json.Marshal(aghRefreshReq{Whitelist: whitelist})
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/control/filtering/refresh", bytes.NewReader(reqBody))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(c.login, c.pass)

	resp, err := c.hc.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(bodyBytes))
		if msg == "" {
			msg = resp.Status
		}
		return 0, fmt.Errorf("AdGuard API error: %s", msg)
	}

	var rr aghRefreshResp
	if err := json.Unmarshal(bodyBytes, &rr); err != nil {
		// some builds may return an empty body on success
		return 0, nil
	}
	return rr.Updated, nil
}
