package bot

import (
	"fmt"
	"slices"
	"sync"
)

var (
	gSettingsCache BotSettings
	gSettingsMu    sync.RWMutex
)

func loadSettingsIntoCache() {
	if gSettings == nil {
		return
	}
	s, err := gSettings.Load()
	if err != nil {
		return
	}
	setSettingsCached(s)
}

func getSettingsCached() BotSettings {
	gSettingsMu.RLock()
	defer gSettingsMu.RUnlock()
	s := gSettingsCache
	s.IgnoredJoinIDs = slices.Clone(s.IgnoredJoinIDs)
	return s
}

func setSettingsCached(s BotSettings) {
	// The cache owns its slice; callers may keep or modify their copy.
	s.IgnoredJoinIDs = slices.Clone(s.IgnoredJoinIDs)
	gSettingsMu.Lock()
	gSettingsCache = s
	gSettingsMu.Unlock()
}

// gSettingsUpdateMu serializes read-modify-write of the settings: concurrent handlers
// must not overwrite each other's changes.
var gSettingsUpdateMu sync.Mutex

// updateSettings applies fn to the current settings, saves them and refreshes the cache.
func updateSettings(fn func(s *BotSettings)) (BotSettings, error) {
	gSettingsUpdateMu.Lock()
	defer gSettingsUpdateMu.Unlock()
	if gSettings == nil {
		return BotSettings{}, fmt.Errorf("настройки бота не инициализированы")
	}
	s := getSettingsCached()
	fn(&s)
	if err := gSettings.Save(s); err != nil {
		return s, err
	}
	setSettingsCached(s)
	return s, nil
}
