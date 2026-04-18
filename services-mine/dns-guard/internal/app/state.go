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
	Offset int64 `json:"offset"`
}

type ProfileRiskState struct {
	LastRisk              int    `json:"last_risk"`
	LastNotifyAt          string `json:"last_notify_at,omitempty"`
	LastNotifiedRisk      int    `json:"last_notified_risk,omitempty"`
	LastMatchedDomain     string `json:"last_matched_domain,omitempty"`
	LastMatchedReason     string `json:"last_matched_reason,omitempty"`
	LastAction            string `json:"last_action,omitempty"`
	LastBlockAt           string `json:"last_block_at,omitempty"`
	LastNotificationEvent string `json:"last_notification_event,omitempty"`
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
	return &st, nil
}

func saveState(path string, st *State) error {
	if st.Profiles == nil {
		st.Profiles = map[string]ProfileRiskState{}
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
