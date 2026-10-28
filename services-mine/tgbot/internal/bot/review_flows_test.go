package bot

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestPrivateDialogAndPendingReplaceEachOther(t *testing.T) {
	const id int64 = 86701
	t.Cleanup(func() { clearUserUIState(id) })
	setters := []func(){
		func() { setWgPending(id, wgPending{}) },
		func() { setAwgPending(id, wgPending{}) },
		func() { setOvpnPending(id, ovpnPending{}) },
		func() { setMtpPending(id, mtpPending{}) },
		func() { setUsrPending(id, usrPending{}) },
		func() { setAccAdminPending(id, accAdminPending{}) },
	}
	for i, set := range setters {
		setConv(id, id, ConvState{Mode: ConvAdminBroadcastText})
		set()
		if _, ok := getConv(id, id); ok {
			t.Fatalf("section %d left old dialog", i)
		}
		setConv(id, id, ConvState{Mode: ConvAdminMsgAwaitText})
		if clearPrivatePendings(id) {
			t.Fatalf("dialog left section %d pending", i)
		}
		if _, ok := getConv(id, id); !ok {
			t.Fatal("new dialog missing")
		}
	}
	// A group wizard must not replace input in the private chat.
	setMtpPending(id, mtpPending{})
	setConv(-10, id, ConvState{Mode: ConvSupportAwaitText})
	if _, ok := popMtpPending(id); !ok {
		t.Fatal("group dialog consumed private pending")
	}
}

func TestDeleteUserClearsAllUIState(t *testing.T) {
	bot, _ := newFakeTelegram(t)
	us := newTestUsersStore(t, disabledFixture)
	ctx := adminCtx(t, bot, us)
	const id int64 = 400
	t.Cleanup(func() { clearUserUIState(id) })
	setUsrPending(id, usrPending{Kind: usrPendingRename})
	setConv(-10, id, ConvState{Mode: ConvSupportAwaitText})
	scope := wgAdminScope{Mode: wgScopeAll}
	setAdminScope(id, scope)
	setAwgAdminScope(id, scope)
	setOvpnAdminScope(id, scope)
	setMtpAdminScope(id, scope)
	handleUsrDeleteConfirm(ctx, idStr(id))
	if _, ok, _ := us.GetByID(id); ok {
		t.Fatal("user was not deleted")
	}
	if clearPrivatePendings(id) {
		t.Fatal("pending survived deletion")
	}
	if _, ok := getConv(-10, id); ok {
		t.Fatal("group dialog survived deletion")
	}
	for _, get := range []func(int64) (wgAdminScope, bool){getAdminScope, getAwgAdminScope, getOvpnAdminScope, getMtpAdminScope} {
		if _, ok := get(id); ok {
			t.Fatal("scope survived deletion")
		}
	}
	if _, err := us.Add("petya", id); err != nil {
		t.Fatal(err)
	}
	if _, ok := getConv(id, id); ok {
		t.Fatal("readded user inherited a dialog")
	}
}

func TestJoinCooldownOnlyAfterDelivery(t *testing.T) {
	t.Cleanup(func() {
		joinPerUser.mu.Lock()
		defer joinPerUser.mu.Unlock()
		for _, id := range []int64{86702, 86703, 86704} {
			delete(joinPerUser.last, id)
		}
	})
	useTestSettings(t)
	joinGlobalMu.Lock()
	saved := joinGlobalSent
	joinGlobalSent = nil
	joinGlobalMu.Unlock()
	t.Cleanup(func() { joinGlobalMu.Lock(); joinGlobalSent = saved; joinGlobalMu.Unlock() })
	bot, ft := newFakeTelegram(t)
	us := newTestUsersStore(t, `[{"telegram_id":100,"name":"boss","roles":["Admin"]}]`)
	msg := func(id int64) *tgbotapi.Message {
		return &tgbotapi.Message{From: &tgbotapi.User{ID: id}, Chat: &tgbotapi.Chat{ID: id, Type: "private"}, Text: "hi"}
	}
	cooling := func(id int64) bool {
		joinPerUser.mu.Lock()
		defer joinPerUser.mu.Unlock()
		_, ok := joinPerUser.last[id]
		return ok
	}
	// Global admission denied: retry after the window is still allowed.
	const limited int64 = 86702
	for i := 0; i < joinGlobalMax; i++ {
		joinGlobalAllow(time.Now())
	}
	notifyAdminsAboutStranger(bot, us, msg(limited))
	if cooling(limited) {
		t.Fatal("global denial started a daily cooldown")
	}
	joinGlobalMu.Lock()
	joinGlobalSent = nil
	joinGlobalMu.Unlock()
	notifyAdminsAboutStranger(bot, us, msg(limited))
	if !cooling(limited) || ft.count("sendMessage:100") != 1 {
		t.Fatal("retry was not delivered")
	}
	// No admins: no daily cooldown.
	empty := newTestUsersStore(t, `[]`)
	notifyAdminsAboutStranger(bot, empty, msg(86703))
	if cooling(86703) {
		t.Fatal("no-admin request started a daily cooldown")
	}
	// Telegram rejects every send: no daily cooldown.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/bottest/getMe" {
			fmt.Fprint(w, `{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"t"}}`)
			return
		}
		fmt.Fprint(w, `{"ok":false,"error_code":403,"description":"Forbidden"}`)
	}))
	defer srv.Close()
	failing, err := tgbotapi.NewBotAPIWithAPIEndpoint("test", srv.URL+"/bot%s/%s")
	if err != nil {
		t.Fatal(err)
	}
	notifyAdminsAboutStranger(failing, us, msg(86704))
	if cooling(86704) {
		t.Fatal("failed delivery started a daily cooldown")
	}
}
