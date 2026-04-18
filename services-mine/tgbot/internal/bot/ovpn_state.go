package bot

import "sync"

type ovpnPendingKind string

const (
	ovpnPendingAdd ovpnPendingKind = "add"
)

type ovpnPending struct {
	Kind ovpnPendingKind
}

var ovpnPendMu sync.Mutex
var ovpnPendings = map[int64]ovpnPending{}

func setOvpnPending(tgID int64, p ovpnPending) {
	ovpnPendMu.Lock()
	defer ovpnPendMu.Unlock()
	ovpnPendings[tgID] = p
}

func popOvpnPending(tgID int64) (ovpnPending, bool) {
	ovpnPendMu.Lock()
	defer ovpnPendMu.Unlock()
	p, ok := ovpnPendings[tgID]
	if ok {
		delete(ovpnPendings, tgID)
	}
	return p, ok
}
