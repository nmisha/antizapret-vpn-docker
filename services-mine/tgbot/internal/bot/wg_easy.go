package bot

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

type wgEasyID string

func (id *wgEasyID) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) {
		*id = ""
		return nil
	}

	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*id = wgEasyID(s)
		return nil
	}

	var n int64
	if err := json.Unmarshal(data, &n); err == nil {
		*id = wgEasyID(strconv.FormatInt(n, 10))
		return nil
	}

	return fmt.Errorf("unsupported wg-easy id: %s", string(data))
}

type wgEasyClient struct {
	BaseURL  string
	Username string
	Password string
	http     *http.Client
}

func newWgEasyClient(host, port, username, password string) (*wgEasyClient, error) {
	if host == "" || port == "" || password == "" {
		return nil, fmt.Errorf("WG_HOST, WG_PORT, WG_PASSWORD required")
	}
	return &wgEasyClient{
		BaseURL:  fmt.Sprintf("http://%s:%s", host, port),
		Username: username,
		Password: password,
		http: &http.Client{
			Timeout: 25 * time.Second,
		},
	}, nil
}

func (c *wgEasyClient) newReq(method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, c.BaseURL+path, body)
	if err != nil {
		return nil, err
	}
	if c.Username != "" {
		token := base64.StdEncoding.EncodeToString([]byte(c.Username + ":" + c.Password))
		req.Header.Set("Authorization", "Basic "+token)
	} else {
		// Backward-compatible fallback for older wg-easy deployments.
		req.Header.Set("Authorization", c.Password)
	}
	return req, nil
}

type wgEasyPeer struct {
	ID                  wgEasyID `json:"id"`
	Name                string   `json:"name"`
	Enabled             bool     `json:"enabled"`
	LatestHandshake     string   `json:"latestHandshakeAt"`
	TransferRx          int64    `json:"transferRx"`
	TransferTx          int64    `json:"transferTx"`
	Address             string   `json:"address"`
	CreatedAt           string   `json:"createdAt"`
	UpdatedAt           string   `json:"updatedAt"`
	PublicKey           string   `json:"publicKey"`
	PersistentKeepalive int      `json:"persistentKeepalive"`
	ExpiresAt           any      `json:"expiresAt"`
	IPv4Address         string   `json:"ipv4Address"`
	IPv6Address         string   `json:"ipv6Address"`
	PreUp               string   `json:"preUp"`
	PostUp              string   `json:"postUp"`
	PreDown             string   `json:"preDown"`
	PostDown            string   `json:"postDown"`
	AllowedIPs          []string `json:"allowedIps"`
	ServerAllowedIPs    []string `json:"serverAllowedIps"`
	FirewallIPs         []string `json:"firewallIps"`
	MTU                 int      `json:"mtu"`
	JC                  *int     `json:"jC"`
	JMin                *int     `json:"jMin"`
	JMax                *int     `json:"jMax"`
	I1                  *string  `json:"i1"`
	I2                  *string  `json:"i2"`
	I3                  *string  `json:"i3"`
	I4                  *string  `json:"i4"`
	I5                  *string  `json:"i5"`
	ServerEndpoint      *string  `json:"serverEndpoint"`
	DNS                 []string `json:"dns"`
}

func (p wgEasyPeer) latestHandshakeTime() time.Time {
	if p.LatestHandshake == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, p.LatestHandshake)
	if err != nil {
		// try RFC3339Nano (wg-easy sometimes returns ms)
		t2, err2 := time.Parse(time.RFC3339Nano, p.LatestHandshake)
		if err2 != nil {
			return time.Time{}
		}
		return t2
	}
	return t
}

func (c *wgEasyClient) listPeers() ([]wgEasyPeer, error) {
	req, err := c.newReq("GET", "/api/client", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("list peers failed: %s: %s", resp.Status, string(b))
	}
	var peers []wgEasyPeer
	if err := json.NewDecoder(resp.Body).Decode(&peers); err != nil {
		return nil, err
	}
	return peers, nil
}

func (c *wgEasyClient) getConfiguration(clientID string) ([]byte, string, error) {
	req, err := c.newReq("GET", "/api/client/"+clientID+"/configuration", nil)
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
		return nil, "", fmt.Errorf("get configuration failed: %s: %s", resp.Status, string(b))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	// try to extract filename from Content-Disposition
	filename := ""
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		// naive parse: attachment; filename="X.conf"
		const key = "filename=\""
		if i := bytes.Index([]byte(cd), []byte(key)); i >= 0 {
			j := i + len(key)
			if k := bytes.IndexByte([]byte(cd)[j:], '"'); k >= 0 {
				filename = cd[j : j+k]
			}
		}
	}
	if filename == "" {
		filename = clientID + ".conf"
	}
	return data, filename, nil
}

func (c *wgEasyClient) getQRCodeSVG(clientID string) ([]byte, error) {
	req, err := c.newReq("GET", "/api/client/"+clientID+"/qrcode.svg", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("get qrcode failed: %s: %s", resp.Status, string(b))
	}
	return io.ReadAll(resp.Body)
}

func (c *wgEasyClient) enableClient(clientID string) error {
	req, err := c.newReq("POST", "/api/client/"+clientID+"/enable", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("enable failed: %s: %s", resp.Status, string(b))
	}
	return nil
}

func (c *wgEasyClient) disableClient(clientID string) error {
	req, err := c.newReq("POST", "/api/client/"+clientID+"/disable", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("disable failed: %s: %s", resp.Status, string(b))
	}
	return nil
}

func (c *wgEasyClient) createClient(name string) error {
	body, _ := json.Marshal(map[string]any{
		"name":      name,
		"expiresAt": nil,
	})
	req, err := c.newReq("POST", "/api/client", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("create client failed: %s: %s", resp.Status, string(b))
	}
	return nil
}

func (c *wgEasyClient) deleteClient(clientID string) error {
	req, err := c.newReq("DELETE", "/api/client/"+clientID, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("delete client failed: %s: %s", resp.Status, string(b))
	}
	return nil
}

func (c *wgEasyClient) renameClient(clientID, newName string) error {
	peer, err := c.getPeer(clientID)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{
		"name":                newName,
		"enabled":             peer.Enabled,
		"expiresAt":           peer.ExpiresAt,
		"ipv4Address":         peer.IPv4Address,
		"ipv6Address":         peer.IPv6Address,
		"preUp":               peer.PreUp,
		"postUp":              peer.PostUp,
		"preDown":             peer.PreDown,
		"postDown":            peer.PostDown,
		"allowedIps":          peer.AllowedIPs,
		"serverAllowedIps":    peer.ServerAllowedIPs,
		"firewallIps":         peer.FirewallIPs,
		"mtu":                 peer.MTU,
		"jC":                  peer.JC,
		"jMin":                peer.JMin,
		"jMax":                peer.JMax,
		"i1":                  peer.I1,
		"i2":                  peer.I2,
		"i3":                  peer.I3,
		"i4":                  peer.I4,
		"i5":                  peer.I5,
		"persistentKeepalive": peer.PersistentKeepalive,
		"serverEndpoint":      peer.ServerEndpoint,
		"dns":                 peer.DNS,
	})
	req, err := c.newReq("POST", "/api/client/"+clientID, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("rename client failed: %s: %s", resp.Status, string(b))
	}
	return nil
}

func (c *wgEasyClient) getPeer(clientID string) (*wgEasyPeer, error) {
	req, err := c.newReq("GET", "/api/client/"+clientID, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("get client failed: %s: %s", resp.Status, string(b))
	}
	var peer wgEasyPeer
	if err := json.NewDecoder(resp.Body).Decode(&peer); err != nil {
		return nil, err
	}
	return &peer, nil
}
