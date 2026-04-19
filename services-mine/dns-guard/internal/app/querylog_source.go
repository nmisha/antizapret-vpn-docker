package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type QueryLogSource interface {
	Sync(cursor *CursorState) error
	Process(cursor *CursorState, handler func(QueryLogEntry) error) error
	Description() string
}

func newQueryLogSource(cfg Config) (QueryLogSource, error) {
	switch queryLogSource(cfg) {
	case "api":
		return newAdGuardQueryLogSource(cfg.AdGuard), nil
	case "file":
		return &fileQueryLogSource{path: cfg.QueryLogPath}, nil
	default:
		return nil, fmt.Errorf("unsupported querylog_source %q", cfg.QueryLogSource)
	}
}

type fileQueryLogSource struct {
	path string
}

func (s *fileQueryLogSource) Sync(cursor *CursorState) error {
	return syncCursorToEOF(s.path, cursor)
}

func (s *fileQueryLogSource) Process(cursor *CursorState, handler func(QueryLogEntry) error) error {
	readResult, err := processNewEntries(s.path, *cursor, handler)
	if err != nil {
		return err
	}
	cursor.Offset = readResult.NewOffset
	cursor.FileSize = readResult.FileSize
	cursor.FileModTime = readResult.FileModTime.Format(time.RFC3339Nano)
	return nil
}

func (s *fileQueryLogSource) Description() string {
	return "file:" + s.path
}

type adGuardQueryLogSource struct {
	baseURL   string
	username  string
	password  string
	pageLimit int
	hc        *http.Client
}

func newAdGuardQueryLogSource(cfg AdGuardAPIConfig) QueryLogSource {
	baseURL := strings.TrimRight(fmt.Sprintf("%s://%s:%s/control", cfg.Scheme, cfg.Host, cfg.Port), "/")
	return &adGuardQueryLogSource{
		baseURL:   baseURL,
		username:  cfg.Username,
		password:  cfg.Password,
		pageLimit: cfg.PageLimit,
		hc: &http.Client{
			Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second,
		},
	}
}

func (s *adGuardQueryLogSource) Sync(cursor *CursorState) error {
	resp, err := s.fetchPage("")
	if err != nil {
		return err
	}
	cursor.Offset = 0
	cursor.FileSize = 0
	cursor.FileModTime = ""
	cursor.LastSeenTime = ""
	cursor.RecentEventKeys = nil
	if len(resp.Data) == 0 {
		return nil
	}
	latest := mapAdGuardQueryEntry(resp.Data[0])
	latestTime := parseEventTime(latest.T)
	if latestTime.IsZero() {
		return nil
	}
	cursor.LastSeenTime = latestTime.UTC().Format(time.RFC3339Nano)
	for _, item := range resp.Data {
		entry := mapAdGuardQueryEntry(item)
		if !parseEventTime(entry.T).Equal(latestTime) {
			break
		}
		if key := eventKey(entry); key != "" {
			cursor.RecentEventKeys = appendUniqueBounded(cursor.RecentEventKeys, key, maxRecentEventKeys)
		}
	}
	return nil
}

func (s *adGuardQueryLogSource) Process(cursor *CursorState, handler func(QueryLogEntry) error) error {
	lastSeen := parseEventTime(cursor.LastSeenTime)
	olderThan := ""
	stop := false
	entries := make([]QueryLogEntry, 0, s.pageLimit)

	for !stop {
		page, err := s.fetchPage(olderThan)
		if err != nil {
			return err
		}
		if len(page.Data) == 0 {
			break
		}
		for _, item := range page.Data {
			entry := mapAdGuardQueryEntry(item)
			eventTime := parseEventTime(entry.T)
			if !lastSeen.IsZero() && !eventTime.IsZero() && eventTime.Before(lastSeen) {
				stop = true
				break
			}
			entries = append(entries, entry)
		}
		if stop || strings.TrimSpace(page.Oldest) == "" {
			break
		}
		olderThan = page.Oldest
	}

	for i := len(entries) - 1; i >= 0; i-- {
		if handler == nil {
			continue
		}
		if err := handler(entries[i]); err != nil {
			return err
		}
	}
	return nil
}

func (s *adGuardQueryLogSource) Description() string {
	return "api:" + strings.TrimPrefix(s.baseURL, "http://")
}

func (s *adGuardQueryLogSource) fetchPage(olderThan string) (adGuardQueryLogResponse, error) {
	u, err := url.Parse(s.baseURL + "/querylog")
	if err != nil {
		return adGuardQueryLogResponse{}, fmt.Errorf("build adguard querylog url: %w", err)
	}
	q := u.Query()
	q.Set("response_status", "all")
	q.Set("limit", fmt.Sprintf("%d", s.pageLimit))
	if strings.TrimSpace(olderThan) != "" {
		q.Set("older_than", olderThan)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return adGuardQueryLogResponse{}, fmt.Errorf("build adguard querylog request: %w", err)
	}
	req.SetBasicAuth(s.username, s.password)

	resp, err := s.hc.Do(req)
	if err != nil {
		return adGuardQueryLogResponse{}, fmt.Errorf("call adguard querylog api: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return adGuardQueryLogResponse{}, fmt.Errorf("read adguard querylog response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = resp.Status
		}
		return adGuardQueryLogResponse{}, fmt.Errorf("adguard querylog api error: %s", msg)
	}

	var parsed adGuardQueryLogResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return adGuardQueryLogResponse{}, fmt.Errorf("parse adguard querylog response: %w", err)
	}
	return parsed, nil
}

type adGuardQueryLogResponse struct {
	Oldest string                 `json:"oldest"`
	Data   []adGuardQueryLogEntry `json:"data"`
}

type adGuardQueryLogEntry struct {
	Client   string `json:"client"`
	Time     string `json:"time"`
	Question struct {
		Name string `json:"name"`
		Host string `json:"host"`
		Type string `json:"type"`
	} `json:"question"`
	Rules []struct {
		Text string `json:"text"`
	} `json:"rules"`
}

func mapAdGuardQueryEntry(src adGuardQueryLogEntry) QueryLogEntry {
	dst := QueryLogEntry{
		T:  src.Time,
		QH: src.Question.Name,
		QT: src.Question.Type,
		IP: src.Client,
	}
	if dst.QH == "" {
		dst.QH = src.Question.Host
	}
	if len(src.Rules) > 0 {
		dst.Result.Rules = make([]struct {
			Text string `json:"Text"`
		}, 0, len(src.Rules))
		for _, rule := range src.Rules {
			dst.Result.Rules = append(dst.Result.Rules, struct {
				Text string `json:"Text"`
			}{Text: rule.Text})
		}
	}
	return dst
}
