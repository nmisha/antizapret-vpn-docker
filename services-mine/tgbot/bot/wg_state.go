package main

import "sync"

type wgScopeMode string

const (
	wgScopeMy   wgScopeMode = "my"
	wgScopeUser wgScopeMode = "user"
	wgScopeAll  wgScopeMode = "all"
)

type wgAdminScope struct {
	Mode     wgScopeMode
	UserName string // normalized username (users.json name)
}

var wgScopeMu sync.Mutex
var wgAdminScopes = map[int64]wgAdminScope{} // key: TelegramID (admin)

func setAdminScope(tgID int64, s wgAdminScope) {
	wgScopeMu.Lock()
	defer wgScopeMu.Unlock()
	wgAdminScopes[tgID] = s
}

func getAdminScope(tgID int64) (wgAdminScope, bool) {
	wgScopeMu.Lock()
	defer wgScopeMu.Unlock()
	s, ok := wgAdminScopes[tgID]
	return s, ok
}

func clearAdminScope(tgID int64) {
	wgScopeMu.Lock()
	defer wgScopeMu.Unlock()
	delete(wgAdminScopes, tgID)
}

type wgPendingKind string

const (
	wgPendingRename wgPendingKind = "rename"
	wgPendingAdd    wgPendingKind = "add"
)

type wgPending struct {
	Kind    wgPendingKind
	PeerID  string // for rename
	Message string
}

var wgPendMu sync.Mutex
var wgPendings = map[int64]wgPending{} // key: TelegramID

func setWgPending(tgID int64, p wgPending) {
	wgPendMu.Lock()
	defer wgPendMu.Unlock()
	wgPendings[tgID] = p
}

func popWgPending(tgID int64) (wgPending, bool) {
	wgPendMu.Lock()
	defer wgPendMu.Unlock()
	p, ok := wgPendings[tgID]
	if ok {
		delete(wgPendings, tgID)
	}
	return p, ok
}
