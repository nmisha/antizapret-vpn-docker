package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type NotificationEvent struct {
	ID           string   `json:"id"`
	Type         string   `json:"type"`
	ProfileKind  string   `json:"profile_kind"`
	ProfileName  string   `json:"profile_name"`
	ProfileIP    string   `json:"profile_ip,omitempty"`
	Risk         int      `json:"risk"`
	Reason       string   `json:"reason"`
	Domains      []string `json:"domains"`
	MatchedRule  string   `json:"matched_rule,omitempty"`
	DetectedAt   string   `json:"detected_at"`
	Action       string   `json:"action"`
	ActionResult string   `json:"action_result,omitempty"`
}

func writeNotificationEvent(dir string, evt NotificationEvent) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir notification dir: %w", err)
	}
	name := sanitizeFilename(evt.ID)
	if name == "" {
		name = fmt.Sprintf("risk-%d", time.Now().UnixNano())
	}
	path := filepath.Join(dir, name+".json")
	b, err := json.MarshalIndent(evt, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}
	return os.WriteFile(path, b, 0644)
}

func deliverNotificationEvent(cfg Config, evt NotificationEvent) error {
	if strings.TrimSpace(cfg.NotificationAPIURL) != "" {
		if err := postNotificationEvent(cfg.NotificationAPIURL, cfg.NotificationAPIToken, evt); err != nil {
			if strings.TrimSpace(cfg.NotificationInboxDir) == "" {
				return err
			}
			if fileErr := writeNotificationEvent(cfg.NotificationInboxDir, evt); fileErr != nil {
				return fmt.Errorf("notification api failed: %v; fallback file delivery failed: %w", err, fileErr)
			}
			return fmt.Errorf("notification api failed, wrote fallback file: %w", err)
		}
		return nil
	}
	return writeNotificationEvent(cfg.NotificationInboxDir, evt)
}

func postNotificationEvent(url, token string, evt NotificationEvent) error {
	b, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("build notification request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send notification request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("notification api status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func sanitizeFilename(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('-')
	}
	return strings.Trim(b.String(), "-")
}
