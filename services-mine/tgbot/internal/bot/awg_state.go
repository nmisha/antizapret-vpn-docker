package bot

import "sync"

var awgScopeMu sync.Mutex
var awgAdminScopes = map[int64]wgAdminScope{}

func setAwgAdminScope(tgID int64, s wgAdminScope) {
	awgScopeMu.Lock()
	defer awgScopeMu.Unlock()
	awgAdminScopes[tgID] = s
}

func getAwgAdminScope(tgID int64) (wgAdminScope, bool) {
	awgScopeMu.Lock()
	defer awgScopeMu.Unlock()
	s, ok := awgAdminScopes[tgID]
	return s, ok
}

func clearAwgAdminScope(tgID int64) {
	awgScopeMu.Lock()
	defer awgScopeMu.Unlock()
	delete(awgAdminScopes, tgID)
}

var awgPendMu sync.Mutex
var awgPendings = map[int64]wgPending{}

func setAwgPending(tgID int64, p wgPending) {
	awgPendMu.Lock()
	defer awgPendMu.Unlock()
	awgPendings[tgID] = p
}

func popAwgPending(tgID int64) (wgPending, bool) {
	awgPendMu.Lock()
	defer awgPendMu.Unlock()
	p, ok := awgPendings[tgID]
	if ok {
		delete(awgPendings, tgID)
	}
	return p, ok
}
