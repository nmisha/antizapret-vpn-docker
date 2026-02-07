package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type aghClient struct {
	baseURL string
	login   string
	pass    string
	hc      *http.Client
}

func newAghClientFromEnv() (*aghClient, error) {
	host := envTrim("AGH_HOST")
	port := envTrim("AGH_PORT")
	login := envTrim("AGH_LOGIN")
	pass := envTrim("AGH_PASSWORD")
	scheme := envTrim("AGH_SCHEME")
	if scheme == "" {
		scheme = "http"
	}
	if host == "" || port == "" || login == "" || pass == "" {
		return nil, fmt.Errorf("missing env: AGH_HOST, AGH_PORT, AGH_LOGIN, AGH_PASSWORD")
	}
	host = strings.TrimSpace(host)
	port = strings.TrimSpace(port)

	base := fmt.Sprintf("%s://%s:%s", scheme, host, port)
	return &aghClient{
		baseURL: base,
		login:   login,
		pass:    pass,
		hc: &http.Client{
			Timeout: 25 * time.Second,
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
