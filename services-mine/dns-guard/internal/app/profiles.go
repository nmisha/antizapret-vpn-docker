package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
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

var (
	ovpnLoginTokenRe   = regexp.MustCompile(`name="_xsrf"\s+value="([^"]+)"`)
	ovpnLoginFormCheck = regexp.MustCompile(`(?i)<form[^>]+action="[^"]*/login"`)
)

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
