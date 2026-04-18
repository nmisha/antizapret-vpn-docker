package app

import (
	"encoding/json"
	"fmt"
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
