package bot

import "sync"

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
	gSettingsMu.Lock()
	gSettingsCache = s
	gSettingsMu.Unlock()
}

func getSettingsCached() BotSettings {
	gSettingsMu.RLock()
	defer gSettingsMu.RUnlock()
	return gSettingsCache
}

func setSettingsCached(s BotSettings) {
	gSettingsMu.Lock()
	gSettingsCache = s
	gSettingsMu.Unlock()
}
