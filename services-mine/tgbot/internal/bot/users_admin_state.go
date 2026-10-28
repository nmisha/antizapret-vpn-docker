package bot

import "sync"

type usrPendingKind string

const (
	usrPendingAdd    usrPendingKind = "add"    // "<name> <tg_id>", or "<name>" when TargetID is known (join request)
	usrPendingRename usrPendingKind = "rename" // new name for TargetID
	usrPendingPrefix usrPendingKind = "prefix" // new Profile prefix for TargetID
)

type usrPending struct {
	Kind     usrPendingKind
	TargetID int64
	Profile  profileKind
}

var usrPendMu sync.Mutex
var usrPendings = map[int64]usrPending{}

func setUsrPending(tgID int64, p usrPending) {
	usrPendMu.Lock()
	defer usrPendMu.Unlock()
	usrPendings[tgID] = p
}

func popUsrPending(tgID int64) (usrPending, bool) {
	usrPendMu.Lock()
	defer usrPendMu.Unlock()
	p, ok := usrPendings[tgID]
	if ok {
		delete(usrPendings, tgID)
	}
	return p, ok
}
