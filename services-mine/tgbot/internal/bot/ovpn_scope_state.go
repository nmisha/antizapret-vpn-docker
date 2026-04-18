package bot

import "sync"

var ovpnScopeMu sync.Mutex
var ovpnAdminScopes = map[int64]wgAdminScope{}

func setOvpnAdminScope(tgID int64, s wgAdminScope) {
	ovpnScopeMu.Lock()
	defer ovpnScopeMu.Unlock()
	ovpnAdminScopes[tgID] = s
}

func getOvpnAdminScope(tgID int64) (wgAdminScope, bool) {
	ovpnScopeMu.Lock()
	defer ovpnScopeMu.Unlock()
	s, ok := ovpnAdminScopes[tgID]
	return s, ok
}

func clearOvpnAdminScope(tgID int64) {
	ovpnScopeMu.Lock()
	defer ovpnScopeMu.Unlock()
	delete(ovpnAdminScopes, tgID)
}

