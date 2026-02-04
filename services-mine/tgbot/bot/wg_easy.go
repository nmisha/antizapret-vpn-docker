package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
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
	jar, _ := cookiejar.New(nil)
	return &wgEasyClient{
		BaseURL:  fmt.Sprintf("http://%s:%s", host, port),
		Password: password,
		http: &http.Client{
			Timeout: 20 * time.Second,
			Jar:     jar,
		},
	}, nil
}

func (c *wgEasyClient) login() error {
	body, _ := json.Marshal(map[string]string{"password": c.Password})
	req, err := http.NewRequest("POST", c.BaseURL+"/api/session", bytes.NewReader(body))
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
		return fmt.Errorf("login failed: %s", resp.Status)
	}
	return nil
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
	if err := c.login(); err != nil {
		return nil, err
	}
	req, err := http.NewRequest("GET", c.BaseURL+"/api/wireguard/client", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("list peers failed: %s", resp.Status)
	}
	var peers []wgEasyPeer
	if err := json.NewDecoder(resp.Body).Decode(&peers); err != nil {
		return nil, err
	}
	return peers, nil
}
