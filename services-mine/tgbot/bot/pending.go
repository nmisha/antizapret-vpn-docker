package main

import "sync"

type PendingAdd struct {
	Candidate     string
	ParentDomain  string
	ParentSection string
	TargetSection string
	ChatID        int64
	MessageID     int
}

var pendingMu sync.Mutex
var pending = map[int64]PendingAdd{} // key: TelegramID

func setPending(tgID int64, p PendingAdd) {
	pendingMu.Lock()
	defer pendingMu.Unlock()
	pending[tgID] = p
}

func getPending(tgID int64) (PendingAdd, bool) {
	pendingMu.Lock()
	defer pendingMu.Unlock()
	p, ok := pending[tgID]
	return p, ok
}

func clearPending(tgID int64) {
	pendingMu.Lock()
	defer pendingMu.Unlock()
	delete(pending, tgID)
}

const (
	cbAddReplace = "add:replace"
	cbAddSub     = "add:sub"
	cbAddCancel  = "add:cancel"
)
