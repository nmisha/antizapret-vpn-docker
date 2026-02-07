package bot

import "sync"

type accAdminPendingKind string

const (
	accAdminAdd      accAdminPendingKind = "add"
	accAdminRename   accAdminPendingKind = "rename"
	accAdminSetLogin accAdminPendingKind = "setlogin"
	accAdminSetPass  accAdminPendingKind = "setpass"
)

type accAdminPending struct {
	Kind accAdminPendingKind
	Name string // existing account name for rename/set* (case-insensitive key)
}

var (
	accAdminPendingsMu sync.Mutex
	accAdminPendings   = map[int64]accAdminPending{} // tgID -> pending
)

func setAccAdminPending(tgID int64, p accAdminPending) {
	accAdminPendingsMu.Lock()
	defer accAdminPendingsMu.Unlock()
	accAdminPendings[tgID] = p
}

func popAccAdminPending(tgID int64) (accAdminPending, bool) {
	accAdminPendingsMu.Lock()
	defer accAdminPendingsMu.Unlock()
	p, ok := accAdminPendings[tgID]
	if ok {
		delete(accAdminPendings, tgID)
	}
	return p, ok
}
