package bot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type wgEasyClient struct {
	BaseURL  string
	Password string
	http     *http.Client
}

func newWgEasyClient(host, port, password string) (*wgEasyClient, error) {
	if host == "" || port == "" || password == "" {
		return nil, fmt.Errorf("WG_HOST, WG_PORT, WG_PASSWORD required")
	}
	return &wgEasyClient{
		BaseURL:  fmt.Sprintf("http://%s:%s", host, port),
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
	// wg-easy v2 supports password via Authorization header (see Server.js)
	req.Header.Set("Authorization", c.Password)
	return req, nil
}

type wgEasyPeer struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	Enabled             bool   `json:"enabled"`
	LatestHandshake     string `json:"latestHandshakeAt"`
	TransferRx          int64  `json:"transferRx"`
	TransferTx          int64  `json:"transferTx"`
	Address             string `json:"address"`
	CreatedAt           string `json:"createdAt"`
	UpdatedAt           string `json:"updatedAt"`
	PublicKey           string `json:"publicKey"`
	PersistentKeepalive string `json:"persistentKeepalive"`
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
	req, err := c.newReq("GET", "/api/wireguard/client", nil)
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
	req, err := c.newReq("GET", "/api/wireguard/client/"+clientID+"/configuration", nil)
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
	req, err := c.newReq("GET", "/api/wireguard/client/"+clientID+"/qrcode.svg", nil)
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
	req, err := c.newReq("POST", "/api/wireguard/client/"+clientID+"/enable", nil)
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
	req, err := c.newReq("POST", "/api/wireguard/client/"+clientID+"/disable", nil)
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
	body, _ := json.Marshal(map[string]string{"name": name})
	req, err := c.newReq("POST", "/api/wireguard/client", bytes.NewReader(body))
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
	req, err := c.newReq("DELETE", "/api/wireguard/client/"+clientID, nil)
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
	body, _ := json.Marshal(map[string]string{"name": newName})
	req, err := c.newReq("PUT", "/api/wireguard/client/"+clientID+"/name", bytes.NewReader(body))
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
