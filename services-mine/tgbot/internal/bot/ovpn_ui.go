package bot

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

var (
	ovpnLoginTokenRe   = regexp.MustCompile(`name="_xsrf"\s+value="([^"]+)"`)
	ovpnCertLinkRe     = regexp.MustCompile(`href="/certificates/([^"/?#]+)"`)
	ovpnLoginFormCheck = regexp.MustCompile(`(?i)<form[^>]+action="[^"]*/login"`)
)

type ovpnProfile struct {
	Name string
}

type ovpnSessionClient struct {
	CommonName      string
	RealAddress     string
	VirtualAddress  string
	BytesReceived   uint64
	BytesSent       uint64
	ConnectedSince  string
	ConnectedSinceT string
	Username        string
}

type ovpnSessionStatus struct {
	ClientList []ovpnSessionClient
}

type ovpnAPIResponse struct {
	Status  string            `json:"status"`
	Message string            `json:"message"`
	Data    ovpnSessionStatus `json:"data"`
}

type ovpnUIClient struct {
	BaseURL  string
	Username string
	Password string
	http     *http.Client
}

func newOvpnUIClient(host, port, username, password string) (*ovpnUIClient, error) {
	if strings.TrimSpace(username) == "" || strings.TrimSpace(password) == "" {
		return nil, fmt.Errorf("OVPN_USERNAME and OVPN_PASSWORD required")
	}
	host = strings.TrimSpace(host)
	if host == "" {
		host = "openvpn-ui.antizapret"
	}
	port = strings.TrimSpace(port)
	if port == "" {
		port = "8080"
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &ovpnUIClient{
		BaseURL:  fmt.Sprintf("http://%s:%s", host, port),
		Username: username,
		Password: password,
		http: &http.Client{
			Timeout: 25 * time.Second,
			Jar:     jar,
		},
	}, nil
}

func newOvpnUIClientFromEnv() (*ovpnUIClient, error) {
	return newOvpnUIClient(
		envTrim("OVPN_HOST"),
		envTrim("OVPN_PORT"),
		envTrim("OVPN_USERNAME"),
		envTrim("OVPN_PASSWORD"),
	)
}

func (c *ovpnUIClient) listProfiles() ([]ovpnProfile, error) {
	body, err := c.getCertificatesPage()
	if err != nil {
		return nil, err
	}
	matches := ovpnCertLinkRe.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("no OpenVPN profiles found on certificates page")
	}

	seen := make(map[string]struct{}, len(matches))
	profiles := make([]ovpnProfile, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		name, err := url.PathUnescape(m[1])
		if err != nil {
			name = m[1]
		}
		name = strings.TrimSpace(name)
		if name == "" || strings.EqualFold(name, "server") {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		profiles = append(profiles, ovpnProfile{Name: name})
	}
	if len(profiles) == 0 {
		return nil, fmt.Errorf("no OpenVPN client profiles found")
	}
	slices.SortFunc(profiles, func(a, b ovpnProfile) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return profiles, nil
}

func (c *ovpnUIClient) downloadProfile(name string) ([]byte, string, error) {
	if err := c.ensureSession(); err != nil {
		return nil, "", err
	}
	escaped := url.PathEscape(strings.TrimSpace(name))
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/certificates/"+escaped, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, "", fmt.Errorf("download profile failed: %s: %s", resp.Status, string(b))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	filename := name + ".ovpn"
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if _, params, err := mime.ParseMediaType(cd); err == nil {
			if v := strings.TrimSpace(params["filename"]); v != "" {
				filename = v
			}
		}
	}
	return data, filename, nil
}

func (c *ovpnUIClient) getCertificatesPage() (string, error) {
	if err := c.ensureSession(); err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/certificates", nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("certificates page failed: %s: %s", resp.Status, string(body))
	}
	if ovpnLoginFormCheck.Match(body) || strings.Contains(strings.ToLower(resp.Request.URL.Path), "/login") {
		return "", fmt.Errorf("OpenVPN UI login failed or session not established")
	}
	return string(body), nil
}

func (c *ovpnUIClient) listSessions() (*ovpnSessionStatus, error) {
	if err := c.ensureSession(); err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/api/v1/session/", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("session api failed: %s: %s", resp.Status, string(body))
	}
	var data ovpnAPIResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	if strings.ToLower(strings.TrimSpace(data.Status)) != "success" {
		if strings.TrimSpace(data.Message) != "" {
			return nil, fmt.Errorf("session api error: %s", data.Message)
		}
		return nil, fmt.Errorf("session api returned status %q", data.Status)
	}
	return &data.Data, nil
}

func (c *ovpnUIClient) ensureSession() error {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/login", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("login page failed: %s: %s", resp.Status, string(body))
	}

	xsrf := ""
	if m := ovpnLoginTokenRe.FindStringSubmatch(string(body)); len(m) == 2 {
		xsrf = m[1]
	}

	form := url.Values{}
	form.Set("login", c.Username)
	form.Set("password", c.Password)
	if xsrf != "" {
		form.Set("_xsrf", xsrf)
	}
	loginReq, err := http.NewRequest(http.MethodPost, c.BaseURL+"/login", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginResp, err := c.http.Do(loginReq)
	if err != nil {
		return err
	}
	defer loginResp.Body.Close()
	loginBody, err := io.ReadAll(io.LimitReader(loginResp.Body, 1<<20))
	if err != nil {
		return err
	}
	if loginResp.StatusCode < 200 || loginResp.StatusCode >= 400 {
		return fmt.Errorf("login failed: %s: %s", loginResp.Status, string(loginBody))
	}
	if ovpnLoginFormCheck.Match(loginBody) && !strings.Contains(strings.ToLower(string(loginBody)), "successfully logged in") {
		return fmt.Errorf("OpenVPN UI authentication rejected")
	}
	return nil
}
