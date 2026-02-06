package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type BotSettings struct {
	LoggingEnabled     bool `json:"logging_enabled"`
	BotEnabledForUsers bool `json:"bot_enabled_for_users"` // if false -> non-admin gets "Bot temporarily disabled"
}

type SettingsStore struct {
	Path string
	mu   sync.Mutex
}

func DefaultBotSettings() BotSettings {
	return BotSettings{
		LoggingEnabled:     true,
		BotEnabledForUsers: true,
	}
}

func NewSettingsStore(usersFilePath string) *SettingsStore {
	dir := filepath.Dir(usersFilePath)
	return &SettingsStore{Path: filepath.Join(dir, "bot_settings.json")}
}

func (ss *SettingsStore) Ensure() error {
	scoped := func() error {
		if _, err := os.Stat(ss.Path); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		b, err := json.MarshalIndent(DefaultBotSettings(), "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(ss.Path, b, 0644)
	}
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return scoped()
}

func (ss *SettingsStore) Load() (BotSettings, error) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	b, err := os.ReadFile(ss.Path)
	if err != nil {
		return BotSettings{}, err
	}
	var s BotSettings
	if err := json.Unmarshal(b, &s); err != nil {
		return BotSettings{}, err
	}
	return s, nil
}

func (ss *SettingsStore) Save(s BotSettings) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := ss.Path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, ss.Path)
}
