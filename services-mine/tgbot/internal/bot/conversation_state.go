package bot

import "sync"

type ConvMode string

const (
	ConvNone                  ConvMode = ""
	ConvSupportAwaitText      ConvMode = "support_await_text"
	ConvNetDNSAwaitDomain     ConvMode = "net_dns_await_domain"
	ConvAdminBroadcastText    ConvMode = "admin_broadcast_text"
	ConvAdminBroadcastConfirm ConvMode = "admin_broadcast_confirm"
	ConvAdminMsgSelectUser    ConvMode = "admin_msg_select_user"
	ConvAdminMsgSearchUser    ConvMode = "admin_msg_search_user"
	ConvAdminMsgAwaitText     ConvMode = "admin_msg_await_text"
	ConvAdminMsgConfirm       ConvMode = "admin_msg_confirm"
)

type ConvState struct {
	Mode         ConvMode
	Draft        string
	TargetID     int64
	TargetName   string
	SearchPrefix string
}

type convKey struct {
	chatID int64
	tgID   int64
}

var (
	convMu sync.Mutex
	conv   = map[convKey]ConvState{}
)

func getConv(chatID, tgID int64) (ConvState, bool) {
	convMu.Lock()
	defer convMu.Unlock()
	s, ok := conv[convKey{chatID: chatID, tgID: tgID}]
	return s, ok
}

func setConv(chatID, tgID int64, st ConvState) {
	// Private Telegram chat IDs equal the user's ID. Group dialogs stay independent.
	if chatID == tgID && st.Mode != ConvNone {
		privatePendingMu.Lock()
		defer privatePendingMu.Unlock()
		clearPrivatePendings(tgID)
	}
	convMu.Lock()
	defer convMu.Unlock()
	if st.Mode == ConvNone {
		delete(conv, convKey{chatID: chatID, tgID: tgID})
		return
	}
	conv[convKey{chatID: chatID, tgID: tgID}] = st
}

func clearConv(chatID, tgID int64) {
	setConv(chatID, tgID, ConvState{Mode: ConvNone})
}

// clearUserUIState discards all flows belonging to a removed or disabled user.
func clearUserUIState(tgID int64) {
	privatePendingMu.Lock()
	defer privatePendingMu.Unlock()
	clearPrivatePendings(tgID)
	convMu.Lock()
	for key := range conv {
		if key.tgID == tgID {
			delete(conv, key)
		}
	}
	convMu.Unlock()
	clearAdminScope(tgID)
	clearAwgAdminScope(tgID)
	clearOvpnAdminScope(tgID)
	clearMtpAdminScope(tgID)
}
