package bot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
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
	ovpnCertRowRe      = regexp.MustCompile(`(?is)<tr\b[^>]*>(.*?)</tr>`)
	ovpnHrefRe         = regexp.MustCompile(`href="([^"]+)"`)
	ovpnRestartHrefRe  = regexp.MustCompile(`(?is)<a\s+href="([^"]+)"[^>]*title="Restart OpenVPN container"`)
	ovpnNameValidRe    = regexp.MustCompile(`^[^\s]+$`)
)

var ovpnReservedCertificateRoutes = map[string]struct{}{
	"restart": {},
	"revoke":  {},
	"renew":   {},
	"burn":    {},
}

type ovpnProfile struct {
	Name                string
	RevokeURL           string
	BurnURL             string
	RestartContainerURL string
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
	profiles := parseOvpnProfilesFromCertificatesPage(body)
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

func parseOvpnProfilesFromCertificatesPage(body string) []ovpnProfile {
	rows := ovpnCertRowRe.FindAllStringSubmatch(body, -1)
	restartContainerURL := parseOvpnRestartContainerURL(body)
	seen := make(map[string]int)
	profiles := make([]ovpnProfile, 0, len(rows))
	for _, rowMatch := range rows {
		if len(rowMatch) < 2 {
			continue
		}
		profile, ok := parseOvpnProfileRow(rowMatch[1])
		if !ok {
			continue
		}
		profile.RestartContainerURL = restartContainerURL
		key := strings.ToLower(profile.Name)
		if idx, exists := seen[key]; exists {
			if profiles[idx].RevokeURL == "" {
				profiles[idx].RevokeURL = profile.RevokeURL
			}
			if profiles[idx].BurnURL == "" {
				profiles[idx].BurnURL = profile.BurnURL
			}
			if profiles[idx].RestartContainerURL == "" {
				profiles[idx].RestartContainerURL = profile.RestartContainerURL
			}
			continue
		}
		seen[key] = len(profiles)
		profiles = append(profiles, profile)
	}
	if len(profiles) > 0 {
		return profiles
	}

	matches := ovpnCertLinkRe.FindAllStringSubmatch(body, -1)
	profiles = make([]ovpnProfile, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		name, ok := parseOvpnProfileName(m[1])
		if !ok {
			continue
		}
		key := strings.ToLower(name)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = len(profiles)
		profiles = append(profiles, ovpnProfile{
			Name:                name,
			RestartContainerURL: restartContainerURL,
		})
	}
	return profiles
}

func parseOvpnRestartContainerURL(body string) string {
	match := ovpnRestartHrefRe.FindStringSubmatch(body)
	if len(match) < 2 {
		return ""
	}
	return html.UnescapeString(strings.TrimSpace(match[1]))
}

func parseOvpnProfileRow(row string) (ovpnProfile, bool) {
	hrefs := ovpnHrefRe.FindAllStringSubmatch(row, -1)
	profile := ovpnProfile{}
	for _, hrefMatch := range hrefs {
		if len(hrefMatch) < 2 {
			continue
		}
		href := html.UnescapeString(strings.TrimSpace(hrefMatch[1]))
		lower := strings.ToLower(href)
		switch {
		case strings.Contains(lower, "/revoke/"):
			profile.RevokeURL = href
		case strings.Contains(lower, "/burn/"):
			profile.BurnURL = href
		case strings.HasPrefix(lower, "/certificates/"):
			name, ok := parseOvpnProfileName(strings.TrimPrefix(href, "/certificates/"))
			if ok && profile.Name == "" {
				profile.Name = name
			}
		}
	}
	if profile.Name == "" {
		return ovpnProfile{}, false
	}
	return profile, true
}

func parseOvpnProfileName(raw string) (string, bool) {
	name, err := url.PathUnescape(raw)
	if err != nil {
		name = raw
	}
	name = strings.TrimSpace(name)
	if name == "" || strings.EqualFold(name, "server") {
		return "", false
	}
	if _, reserved := ovpnReservedCertificateRoutes[strings.ToLower(name)]; reserved {
		return "", false
	}
	return name, true
}

func (c *ovpnUIClient) executeProfileAction(actionURL string) error {
	if err := c.ensureSession(); err != nil {
		return err
	}
	actionURL = strings.TrimSpace(actionURL)
	if actionURL == "" {
		return fmt.Errorf("empty action url")
	}
	parsed, err := url.Parse(actionURL)
	if err != nil {
		return err
	}
	if !parsed.IsAbs() {
		base, err := url.Parse(c.BaseURL)
		if err != nil {
			return err
		}
		actionURL = base.ResolveReference(parsed).String()
	}

	req, err := http.NewRequest(http.MethodGet, actionURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Errorf("OpenVPN action failed: %s: %s", resp.Status, string(body))
	}
	if ovpnLoginFormCheck.Match(body) || strings.Contains(strings.ToLower(resp.Request.URL.Path), "/login") {
		return fmt.Errorf("OpenVPN UI authentication rejected")
	}
	return nil
}

func (c *ovpnUIClient) restartServer(signalName string) error {
	if err := c.ensureSession(); err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]string{
		"sname": strings.TrimSpace(signalName),
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodDelete, c.BaseURL+"/signal", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("OpenVPN restart failed: %s: %s", resp.Status, string(body))
	}
	if ovpnLoginFormCheck.Match(body) || strings.Contains(strings.ToLower(resp.Request.URL.Path), "/login") {
		return fmt.Errorf("OpenVPN UI authentication rejected")
	}
	return nil
}

func (c *ovpnUIClient) createProfile(name string) error {
	name = strings.TrimSpace(name)
	if !ovpnNameValidRe.MatchString(name) {
		return fmt.Errorf("certificate name must not contain spaces")
	}
	if err := c.ensureSession(); err != nil {
		return err
	}

	form := url.Values{}
	form.Set("Name", name)
	form.Set("EasyRSACertExpire", "8250")
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/certificates", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Errorf("create certificate failed: %s: %s", resp.Status, string(body))
	}
	if ovpnLoginFormCheck.Match(body) || strings.Contains(strings.ToLower(resp.Request.URL.Path), "/login") {
		return fmt.Errorf("OpenVPN UI authentication rejected")
	}

	profiles := parseOvpnProfilesFromCertificatesPage(string(body))
	want := strings.ToLower(name)
	for _, profile := range profiles {
		if strings.ToLower(strings.TrimSpace(profile.Name)) == want {
			return nil
		}
	}
	// Fallback to a fresh fetch in case the POST response redirected somewhere else.
	currentProfiles, err := c.listProfiles()
	if err != nil {
		return fmt.Errorf("create certificate result is unclear: %w", err)
	}
	for _, profile := range currentProfiles {
		if strings.ToLower(strings.TrimSpace(profile.Name)) == want {
			return nil
		}
	}
	return fmt.Errorf("certificate %q was not found after create request", name)
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
	if strings.Contains(strings.ToLower(loginResp.Request.URL.Path), "/login") && ovpnLoginFormCheck.Match(loginBody) {
		return fmt.Errorf("OpenVPN UI authentication rejected")
	}
	return nil
}
