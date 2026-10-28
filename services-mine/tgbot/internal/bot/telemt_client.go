package bot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Client for the telemt (MTProto proxy) control API.
// Docs: https://github.com/telemt/telemt/blob/main/docs/Architecture/API/API.md

var telemtUserNameRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

type telemtLinks struct {
	TLS        []string `json:"tls"`
	TLSDomains []struct {
		Domain string `json:"domain"`
		Link   string `json:"link"`
	} `json:"tls_domains"`
}

type telemtUser struct {
	Username            string      `json:"username"`
	Enabled             bool        `json:"enabled"`
	MaxTCPConns         *uint64     `json:"max_tcp_conns"`
	MaxUniqueIPs        *uint64     `json:"max_unique_ips"`
	ExpirationRFC3339   *string     `json:"expiration_rfc3339"`
	DataQuotaBytes      *uint64     `json:"data_quota_bytes"`
	RateLimitUpBps      *uint64     `json:"rate_limit_up_bps"`
	RateLimitDownBps    *uint64     `json:"rate_limit_down_bps"`
	CurrentConnections  uint64      `json:"current_connections"`
	ActiveUniqueIPs     uint64      `json:"active_unique_ips"`
	ActiveUniqueIPsList []string    `json:"active_unique_ips_list"`
	TotalOctets         uint64      `json:"total_octets"`
	Links               telemtLinks `json:"links"`
}

// TLSLink returns the native tg://proxy fake-TLS link, or "".
func (u telemtUser) TLSLink() string {
	if len(u.Links.TLS) > 0 {
		return u.Links.TLS[0]
	}
	for _, d := range u.Links.TLSDomains {
		if d.Link != "" {
			return d.Link
		}
	}
	return ""
}

type telemtClient struct {
	BaseURL string
	Token   string
	http    *http.Client
}

func newTelemtClientFromEnv() (*telemtClient, error) {
	host := envTrim("TELEMT_HOST")
	if host == "" {
		host = "telemt.antizapret"
	}
	port := envTrim("TELEMT_PORT")
	if port == "" {
		port = "9091"
	}
	token := envTrim("TELEMT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("Не задан TELEMT_TOKEN (auth_header из config-mine/telemt/config.toml или TELEMT_API_TOKEN сервиса telemt)")
	}
	timeout := 15 * time.Second
	if v := envTrim("TELEMT_TIMEOUT_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			timeout = time.Duration(n) * time.Second
		}
	}
	return &telemtClient{
		BaseURL: fmt.Sprintf("http://%s:%s/v1", host, port),
		Token:   token,
		http:    &http.Client{Timeout: timeout},
	}, nil
}

// do sends a request and decodes the "data" field of the response envelope into out (if not nil).
func (c *telemtClient) do(method, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.BaseURL+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}

	var env struct {
		OK    bool            `json:"ok"`
		Data  json.RawMessage `json:"data"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("telemt API: HTTP %d: %s", resp.StatusCode, truncate(strings.TrimSpace(string(raw)), 500))
	}
	if !env.OK || resp.StatusCode >= 300 {
		if env.Error != nil {
			return fmt.Errorf("telemt API: %s (%s)", env.Error.Message, env.Error.Code)
		}
		return fmt.Errorf("telemt API: HTTP %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(env.Data, out)
}

func telemtUserPath(name string) string { return "/users/" + url.PathEscape(name) }

func (c *telemtClient) listUsers() ([]telemtUser, error) {
	var out []telemtUser
	err := c.do(http.MethodGet, "/users", nil, &out)
	return out, err
}

func (c *telemtClient) getUser(name string) (telemtUser, error) {
	var out telemtUser
	err := c.do(http.MethodGet, telemtUserPath(name), nil, &out)
	return out, err
}

func (c *telemtClient) createUser(name string) (telemtUser, error) {
	var out struct {
		User telemtUser `json:"user"`
	}
	err := c.do(http.MethodPost, "/users", map[string]any{"username": name}, &out)
	return out.User, err
}

// patchUser applies a JSON Merge Patch; a nil value removes the individual override.
func (c *telemtClient) patchUser(name string, patch map[string]any) (telemtUser, error) {
	var out telemtUser
	err := c.do(http.MethodPatch, telemtUserPath(name), patch, &out)
	return out, err
}

// setEnabled disables (and drops active sessions) or enables the user.
func (c *telemtClient) setEnabled(name string, enabled bool) (telemtUser, error) {
	act := "/disable"
	if enabled {
		act = "/enable"
	}
	var out telemtUser
	err := c.do(http.MethodPost, telemtUserPath(name)+act, nil, &out)
	return out, err
}

// rotateSecret generates a new secret; the old link stops working.
func (c *telemtClient) rotateSecret(name string) (telemtUser, error) {
	var out struct {
		User telemtUser `json:"user"`
	}
	err := c.do(http.MethodPost, telemtUserPath(name)+"/rotate-secret", nil, &out)
	return out.User, err
}

func (c *telemtClient) resetQuota(name string) error {
	return c.do(http.MethodPost, telemtUserPath(name)+"/reset-quota", nil, nil)
}

// telemtWebLink converts tg://proxy?... into https://t.me/proxy?... (clickable in messages and URL buttons).
func telemtWebLink(link string) string {
	if rest, ok := strings.CutPrefix(link, "tg://proxy?"); ok {
		return "https://t.me/proxy?" + rest
	}
	return link
}
