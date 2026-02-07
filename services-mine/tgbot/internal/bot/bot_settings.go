package bot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type BotSettings struct {
	// Legacy: kept for backward compatibility with old bot_settings.json
	LoggingEnabled bool `json:"logging_enabled"`

	// New split flags:
	LogErrorsEnabled   bool `json:"log_errors_enabled"`
	LogCommandsEnabled bool `json:"log_commands_enabled"`

	// How many lines to show for /log (tail). If 0 -> default.
	LogTailLines int `json:"log_tail_lines"`

	BotEnabledForUsers bool `json:"bot_enabled_for_users"` // if false -> non-admin gets "Bot temporarily disabled"
}

type SettingsStore struct {
	Path string
	mu   sync.Mutex
}

func DefaultBotSettings() BotSettings {
	return BotSettings{
		LoggingEnabled:     true,
		LogErrorsEnabled:   true,
		LogCommandsEnabled: true,
		LogTailLines:       80,
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

	// Apply defaults / migration from legacy flag.
	def := DefaultBotSettings()

	// If new flags are not explicitly set, derive them from legacy LoggingEnabled.
	// This prevents old config files from accidentally disabling logging.
	if !s.LogErrorsEnabled && !s.LogCommandsEnabled {
		// both false might mean "not present" (old file) OR explicitly disabled.
		// Use legacy flag as source of truth if it differs from default zero value.
		if s.LoggingEnabled {
			s.LogErrorsEnabled = true
			s.LogCommandsEnabled = true
		} else {
			// If legacy disabled, keep both disabled.
			s.LogErrorsEnabled = false
			s.LogCommandsEnabled = false
		}
	}

	if s.LogTailLines <= 0 {
		s.LogTailLines = def.LogTailLines
	}
	// BotEnabledForUsers default should be true if absent in older files.
	if !s.BotEnabledForUsers {
		// can't distinguish absent vs explicitly false; but legacy behavior had default true
		// and admins can flip it via /bot_off. Keep as-is.
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
