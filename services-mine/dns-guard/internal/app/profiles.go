package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type ProfileRef struct {
	Kind string
	Name string
	ID   string
	IP   string
}

func resolveProfile(ip string, cfg Config) (ProfileRef, error) {
	kind, err := resolveProfileKindByIP(ip, cfg.Subnets)
	if err != nil {
		return ProfileRef{}, err
	}
	switch kind {
	case "wg":
		if !cfg.WG.Enabled {
			return ProfileRef{}, nil
		}
		ref, ok, err := findWGProfile(ip, "wg", cfg.WG)
		if err != nil {
			return ProfileRef{}, err
		}
		if ok {
			return ref, nil
		}
	case "awg":
		if !cfg.AWG.Enabled {
			return ProfileRef{}, nil
		}
		ref, ok, err := findWGProfile(ip, "awg", cfg.AWG)
		if err != nil {
			return ProfileRef{}, err
		}
		if ok {
			return ref, nil
		}
	case "ovpn":
		if !cfg.OVPN.Enabled {
			return ProfileRef{}, nil
		}
		ref, ok, err := findOVPNProfile(ip, cfg.OVPN)
		if err != nil {
			return ProfileRef{}, err
		}
		if ok {
			return ref, nil
		}
	}
	return ProfileRef{Kind: kind, IP: ip}, nil
}

func resolveProfileKindByIP(ip string, subnets map[string][]string) (string, error) {
	parsedIP := net.ParseIP(strings.TrimSpace(ip))
	if parsedIP == nil {
		return "", fmt.Errorf("invalid profile ip %q", ip)
	}
	for _, kind := range []string{"wg", "awg", "ovpn"} {
		for _, cidr := range subnets[kind] {
			cidr = strings.TrimSpace(cidr)
			if cidr == "" {
				continue
			}
			_, network, err := net.ParseCIDR(cidr)
			if err != nil {
				return "", fmt.Errorf("parse %s subnet %q: %w", kind, cidr, err)
			}
			if network.Contains(parsedIP) {
				return kind, nil
			}
		}
	}
	return "", nil
}

type wgPeer struct {
	ID          any    `json:"id"`
	Name        string `json:"name"`
	IPv4Address string `json:"ipv4Address"`
}

func findWGProfile(ip, kind string, cfg GuardAPIConfig) (ProfileRef, bool, error) {
	peers, err := listWGPeers(cfg)
	if err != nil {
		return ProfileRef{}, false, err
	}
	for _, p := range peers {
		if strings.TrimSpace(p.IPv4Address) == ip {
			return ProfileRef{
				Kind: kind,
				Name: strings.TrimSpace(p.Name),
				ID:   stringifyID(p.ID),
				IP:   ip,
			}, true, nil
		}
	}
	return ProfileRef{}, false, nil
}

func listWGPeers(cfg GuardAPIConfig) ([]wgPeer, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s:%s/api/client", cfg.Host, cfg.Port), nil)
	if err != nil {
		return nil, err
	}
	token := base64.StdEncoding.EncodeToString([]byte(cfg.Username + ":" + cfg.Password))
	req.Header.Set("Authorization", "Basic "+token)
	hc := &http.Client{Timeout: 25 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("list wg peers failed: %s: %s", resp.Status, string(b))
	}
	var peers []wgPeer
	if err := json.NewDecoder(resp.Body).Decode(&peers); err != nil {
		return nil, err
	}
	return peers, nil
}

func disableWGProfile(ref ProfileRef, cfg GuardAPIConfig) error {
	if ref.ID == "" {
		return fmt.Errorf("profile id is empty")
	}
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://%s:%s/api/client/%s/disable", cfg.Host, cfg.Port, ref.ID), nil)
	if err != nil {
		return err
	}
	token := base64.StdEncoding.EncodeToString([]byte(cfg.Username + ":" + cfg.Password))
	req.Header.Set("Authorization", "Basic "+token)
	hc := &http.Client{Timeout: 25 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("disable wg profile failed: %s: %s", resp.Status, string(b))
	}
	return nil
}

func stringifyID(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case int64:
		return strconv.FormatInt(t, 10)
	case json.Number:
		return t.String()
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

type ovpnSessionStatus struct {
	ClientList []ovpnSessionClient `json:"client_list"`
}

type ovpnSessionClient struct {
	CommonName     string `json:"common_name"`
	VirtualAddress string `json:"virtual_address"`
}

type ovpnAPIResponse struct {
	Status  string            `json:"status"`
	Message string            `json:"message"`
	Data    ovpnSessionStatus `json:"data"`
}

func (s *ovpnSessionStatus) UnmarshalJSON(data []byte) error {
	type alias ovpnSessionStatus
	var raw struct {
		alias
		ClientListCamel []ovpnSessionClient `json:"ClientList"`
		ClientListSnake []ovpnSessionClient `json:"client_list"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*s = ovpnSessionStatus(raw.alias)
	switch {
	case len(raw.ClientListSnake) > 0:
		s.ClientList = raw.ClientListSnake
	case len(raw.ClientListCamel) > 0:
		s.ClientList = raw.ClientListCamel
	}
	return nil
}

func (c *ovpnSessionClient) UnmarshalJSON(data []byte) error {
	type alias ovpnSessionClient
	var raw struct {
		alias
		CommonNameCamel     string `json:"CommonName"`
		CommonNameSnake     string `json:"common_name"`
		VirtualAddressCamel string `json:"VirtualAddress"`
		VirtualAddressSnake string `json:"virtual_address"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*c = ovpnSessionClient(raw.alias)
	if strings.TrimSpace(c.CommonName) == "" {
		c.CommonName = strings.TrimSpace(nonEmpty(raw.CommonNameSnake, raw.CommonNameCamel))
	}
	if strings.TrimSpace(c.VirtualAddress) == "" {
		c.VirtualAddress = strings.TrimSpace(nonEmpty(raw.VirtualAddressSnake, raw.VirtualAddressCamel))
	}
	return nil
}

var (
	ovpnLoginTokenRe   = regexp.MustCompile(`name="_xsrf"\s+value="([^"]+)"`)
	ovpnLoginFormCheck = regexp.MustCompile(`(?i)<form[^>]+action="[^"]*/login"`)
	ovpnCertRowRe      = regexp.MustCompile(`(?is)<tr\b[^>]*>(.*?)</tr>`)
	ovpnHrefRe         = regexp.MustCompile(`href="([^"]+)"`)
	ovpnRestartHrefRe  = regexp.MustCompile(`(?is)<a\s+href="([^"]+)"[^>]*title="Restart OpenVPN container"`)
)

type ovpnCertificateProfile struct {
	Name                string
	RevokeURL           string
	RestartContainerURL string
}

func findOVPNProfile(ip string, cfg OVPNAPIConfig) (ProfileRef, bool, error) {
	sessions, err := listOVPNSessions(cfg)
	if err != nil {
		return ProfileRef{}, false, err
	}
	for _, s := range sessions {
		if normalizeVirtualIP(s.VirtualAddress) == ip {
			return ProfileRef{
				Kind: "ovpn",
				Name: strings.TrimSpace(s.CommonName),
				IP:   ip,
			}, true, nil
		}
	}
	return ProfileRef{}, false, nil
}

func listOVPNSessions(cfg OVPNAPIConfig) ([]ovpnSessionClient, error) {
	base := fmt.Sprintf("http://%s:%s", cfg.Host, cfg.Port)
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	hc := &http.Client{Timeout: 25 * time.Second, Jar: jar}
	if err := ovpnLogin(hc, base, cfg.Username, cfg.Password); err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, base+"/api/v1/session/", nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ovpn session api failed: %s: %s", resp.Status, string(body))
	}
	var data ovpnAPIResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	if strings.ToLower(strings.TrimSpace(data.Status)) != "success" {
		if strings.TrimSpace(data.Message) != "" {
			return nil, fmt.Errorf("ovpn session api error: %s", data.Message)
		}
		return nil, fmt.Errorf("ovpn session api returned status %q", data.Status)
	}
	return data.Data.ClientList, nil
}

func listOVPNCertificates(cfg OVPNAPIConfig) ([]ovpnCertificateProfile, error) {
	base := fmt.Sprintf("http://%s:%s", cfg.Host, cfg.Port)
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	hc := &http.Client{Timeout: 25 * time.Second, Jar: jar}
	if err := ovpnLogin(hc, base, cfg.Username, cfg.Password); err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, base+"/certificates", nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ovpn certificates page failed: %s: %s", resp.Status, string(body))
	}
	if ovpnLoginFormCheck.Match(body) || strings.Contains(strings.ToLower(resp.Request.URL.Path), "/login") {
		return nil, fmt.Errorf("ovpn authentication rejected")
	}
	return parseOVPNCertificateProfiles(string(body)), nil
}

func revokeOVPNProfile(ref ProfileRef, cfg OVPNAPIConfig) error {
	profiles, err := listOVPNCertificates(cfg)
	if err != nil {
		return err
	}
	want := strings.ToLower(strings.TrimSpace(ref.Name))
	revokeURL := ""
	for _, profile := range profiles {
		if strings.ToLower(strings.TrimSpace(profile.Name)) == want {
			revokeURL = strings.TrimSpace(profile.RevokeURL)
			break
		}
	}
	if revokeURL == "" {
		return fmt.Errorf("ovpn revoke action is unavailable for profile %q", ref.Name)
	}
	return executeOVPNAction(cfg, revokeURL)
}

func restartOVPNServer(cfg OVPNAPIConfig) error {
	base := fmt.Sprintf("http://%s:%s", cfg.Host, cfg.Port)
	jar, err := cookiejar.New(nil)
	if err != nil {
		return err
	}
	hc := &http.Client{Timeout: 25 * time.Second, Jar: jar}
	if err := ovpnLogin(hc, base, cfg.Username, cfg.Password); err != nil {
		return err
	}
	payload := []byte(`{"sname":"SIGUSR1"}`)
	req, err := http.NewRequest(http.MethodDelete, base+"/signal", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ovpn restart failed: %s: %s", resp.Status, string(body))
	}
	if ovpnLoginFormCheck.Match(body) || strings.Contains(strings.ToLower(resp.Request.URL.Path), "/login") {
		return fmt.Errorf("ovpn authentication rejected")
	}
	return nil
}

func executeOVPNAction(cfg OVPNAPIConfig, actionURL string) error {
	base := fmt.Sprintf("http://%s:%s", cfg.Host, cfg.Port)
	jar, err := cookiejar.New(nil)
	if err != nil {
		return err
	}
	hc := &http.Client{Timeout: 25 * time.Second, Jar: jar}
	if err := ovpnLogin(hc, base, cfg.Username, cfg.Password); err != nil {
		return err
	}
	parsed, err := url.Parse(strings.TrimSpace(actionURL))
	if err != nil {
		return err
	}
	if !parsed.IsAbs() {
		baseURL, err := url.Parse(base)
		if err != nil {
			return err
		}
		actionURL = baseURL.ResolveReference(parsed).String()
	}
	req, err := http.NewRequest(http.MethodGet, actionURL, nil)
	if err != nil {
		return err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Errorf("ovpn action failed: %s: %s", resp.Status, string(body))
	}
	if ovpnLoginFormCheck.Match(body) || strings.Contains(strings.ToLower(resp.Request.URL.Path), "/login") {
		return fmt.Errorf("ovpn authentication rejected")
	}
	return nil
}

func parseOVPNCertificateProfiles(body string) []ovpnCertificateProfile {
	rows := ovpnCertRowRe.FindAllStringSubmatch(body, -1)
	restartContainerURL := parseOVPNRestartContainerURL(body)
	seen := make(map[string]int)
	out := make([]ovpnCertificateProfile, 0, len(rows))
	for _, row := range rows {
		if len(row) < 2 {
			continue
		}
		profile, ok := parseOVPNCertificateRow(row[1])
		if !ok {
			continue
		}
		profile.RestartContainerURL = restartContainerURL
		key := strings.ToLower(profile.Name)
		if idx, exists := seen[key]; exists {
			if out[idx].RevokeURL == "" {
				out[idx].RevokeURL = profile.RevokeURL
			}
			if out[idx].RestartContainerURL == "" {
				out[idx].RestartContainerURL = profile.RestartContainerURL
			}
			continue
		}
		seen[key] = len(out)
		out = append(out, profile)
	}
	return out
}

func parseOVPNRestartContainerURL(body string) string {
	match := ovpnRestartHrefRe.FindStringSubmatch(body)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(html.UnescapeString(match[1]))
}

func parseOVPNCertificateRow(row string) (ovpnCertificateProfile, bool) {
	hrefs := ovpnHrefRe.FindAllStringSubmatch(row, -1)
	profile := ovpnCertificateProfile{}
	for _, hrefMatch := range hrefs {
		if len(hrefMatch) < 2 {
			continue
		}
		href := strings.TrimSpace(html.UnescapeString(hrefMatch[1]))
		lower := strings.ToLower(href)
		switch {
		case strings.Contains(lower, "/revoke/"):
			profile.RevokeURL = href
		case strings.HasPrefix(lower, "/certificates/"):
			name, ok := parseOVPNCertificateName(strings.TrimPrefix(href, "/certificates/"))
			if ok && profile.Name == "" {
				profile.Name = name
			}
		}
	}
	if profile.Name == "" {
		return ovpnCertificateProfile{}, false
	}
	return profile, true
}

func parseOVPNCertificateName(raw string) (string, bool) {
	name, err := url.PathUnescape(raw)
	if err != nil {
		name = raw
	}
	name = strings.TrimSpace(name)
	if name == "" || strings.EqualFold(name, "server") {
		return "", false
	}
	switch strings.ToLower(name) {
	case "restart", "revoke", "renew", "burn":
		return "", false
	default:
		return name, true
	}
}


func ovpnLogin(hc *http.Client, baseURL, username, password string) error {
	req, err := http.NewRequest(http.MethodGet, baseURL+"/login", nil)
	if err != nil {
		return err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ovpn login page failed: %s", resp.Status)
	}
	xsrf := ""
	if m := ovpnLoginTokenRe.FindStringSubmatch(string(body)); len(m) == 2 {
		xsrf = m[1]
	}
	form := url.Values{}
	form.Set("login", username)
	form.Set("password", password)
	if xsrf != "" {
		form.Set("_xsrf", xsrf)
	}
	loginReq, err := http.NewRequest(http.MethodPost, baseURL+"/login", bytes.NewReader([]byte(form.Encode())))
	if err != nil {
		return err
	}
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginResp, err := hc.Do(loginReq)
	if err != nil {
		return err
	}
	defer loginResp.Body.Close()
	loginBody, err := io.ReadAll(io.LimitReader(loginResp.Body, 1<<20))
	if err != nil {
		return err
	}
	if loginResp.StatusCode < 200 || loginResp.StatusCode >= 400 {
		return fmt.Errorf("ovpn login failed: %s", loginResp.Status)
	}
	if strings.Contains(strings.ToLower(loginResp.Request.URL.Path), "/login") && ovpnLoginFormCheck.Match(loginBody) {
		return fmt.Errorf("ovpn authentication rejected")
	}
	return nil
}

func normalizeVirtualIP(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "/, "); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
