package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type State struct {
	Cursor   CursorState                 `json:"cursor"`
	Profiles map[string]ProfileRiskState `json:"profiles"`
}

type CursorState struct {
	Offset          int64    `json:"offset"`
	FileSize        int64    `json:"file_size,omitempty"`
	FileModTime     string   `json:"file_mod_time,omitempty"`
	LastSeenTime    string   `json:"last_seen_time,omitempty"`
	RecentEventKeys []string `json:"recent_event_keys,omitempty"`
}

type ProfileRiskState struct {
	LastRuleRisk          int            `json:"last_rule_risk,omitempty"`
	LastNotifyAt          string         `json:"last_notify_at,omitempty"`
	LastNotifiedScore     int            `json:"last_notified_score,omitempty"`
	LastMatchedDomain     string         `json:"last_matched_domain,omitempty"`
	LastMatchedReason     string         `json:"last_matched_reason,omitempty"`
	LastAction            string         `json:"last_action,omitempty"`
	LastBlockAt           string         `json:"last_block_at,omitempty"`
	LastNotificationEvent string         `json:"last_notification_event,omitempty"`
	LastProfileID         string         `json:"last_profile_id,omitempty"`
	LastProfileIP         string         `json:"last_profile_ip,omitempty"`
	PendingBlockAt        string         `json:"pending_block_at,omitempty"`
	PendingBlockWindow    string         `json:"pending_block_window,omitempty"`
	PendingBlockScore     int            `json:"pending_block_score,omitempty"`
	Buckets               map[string]int `json:"buckets,omitempty"`
}

func loadState(path string) (*State, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &State{Profiles: map[string]ProfileRiskState{}}, nil
		}
		return nil, fmt.Errorf("read state: %w", err)
	}
	var st State
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	if st.Profiles == nil {
		st.Profiles = map[string]ProfileRiskState{}
	}
	for key, ps := range st.Profiles {
		if ps.Buckets == nil {
			ps.Buckets = map[string]int{}
		}
		st.Profiles[key] = ps
	}
	if st.Cursor.RecentEventKeys == nil {
		st.Cursor.RecentEventKeys = []string{}
	}
	return &st, nil
}

func saveState(path string, st *State) error {
	if st.Profiles == nil {
		st.Profiles = map[string]ProfileRiskState{}
	}
	for key, ps := range st.Profiles {
		if ps.Buckets == nil {
			ps.Buckets = map[string]int{}
		}
		st.Profiles[key] = ps
	}
	if st.Cursor.RecentEventKeys == nil {
		st.Cursor.RecentEventKeys = []string{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("mkdir state dir: %w", err)
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return fmt.Errorf("write state temp: %w", err)
	}
	return os.Rename(tmp, path)
}
