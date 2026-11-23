package bot

import "sync"

// Keep last known sender label per chat, so we can include it in logs
// even from helper functions that only know chatID (e.g., reply()).
var (
	gChatUserMu    sync.RWMutex
	tgChatUserByID = map[int64]string{}
)

func setChatUserLabel(chatID int64, label string) {
	if chatID == 0 {
		return
	}
	gChatUserMu.Lock()
	if label == "" {
		// keep previous non-empty label if any
		if _, ok := tgChatUserByID[chatID]; !ok {
			tgChatUserByID[chatID] = ""
		}
	} else {
		tgChatUserByID[chatID] = label
	}
	gChatUserMu.Unlock()
}

func getChatUserLabel(chatID int64) string {
	gChatUserMu.RLock()
	defer gChatUserMu.RUnlock()
	return tgChatUserByID[chatID]
}
