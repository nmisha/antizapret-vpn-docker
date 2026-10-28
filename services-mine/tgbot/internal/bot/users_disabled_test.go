package bot

import (
	"os"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Fixture: active admin 100, disabled admin 300 with Support, disabled owner 200 of wg "vasya_" profiles.
const disabledFixture = `[
	{"telegram_id":100,"name":"boss","roles":["Admin","Support"]},
	{"telegram_id":200,"name":"vasya","roles":["WgUserControl"],"wg_profiles":["vasya_"],"disabled":true},
	{"telegram_id":300,"name":"oldboss","roles":["Admin","Support"],"disabled":true},
	{"telegram_id":400,"name":"petya","roles":[]}]`

func (f *fakeTelegram) count(entry string) int {
	n := 0
	for _, s := range f.sent() {
		if s == entry {
			n++
		}
	}
	return n
}

func (f *fakeTelegram) sentTo(chatID string) bool {
	for _, s := range f.sent() {
		if strings.HasSuffix(s, ":"+chatID) {
			return true
		}
	}
	return false
}

func adminCtx(t *testing.T, bot *tgbotapi.BotAPI, us *UsersStore) *Ctx {
	t.Helper()
	admin, ok, err := us.GetByID(100)
	if err != nil || !ok {
		t.Fatalf("admin fixture: %v", err)
	}
	return &Ctx{Bot: bot, ChatID: 100, TgID: 100, User: admin, UsersStore: us, IsPrivate: true}
}

func TestDisabledFieldOmittedForActiveUsers(t *testing.T) {
	us := newTestUsersStore(t, `[{"telegram_id":1,"name":"a","roles":[]}]`)
	if _, err := us.SetDisabled("a", true); err != nil {
		t.Fatal(err)
	}
	if _, err := us.SetDisabled("a", false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(us.Path)
	if strings.Contains(string(b), "disabled") {
		t.Fatalf("active user persisted with the attribute:\n%s", b)
	}
}

func TestDisabledUserCallbackGetsJokeOnly(t *testing.T) {
	bot, ft := newFakeTelegram(t)
	us := newTestUsersStore(t, disabledFixture)

	handleCallback(bot, us, nil, nil, &tgbotapi.CallbackQuery{
		ID: "q", From: &tgbotapi.User{ID: 300}, Data: usrCb("list"),
		Message: &tgbotapi.Message{MessageID: 7, Chat: &tgbotapi.Chat{ID: 300, Type: "private"}},
	})
	if ft.count("sendMessage:300") != 1 || ft.count("editMessageText:300") != 0 {
		t.Fatalf("disabled admin reached the UI: %v", ft.sent())
	}
}

func TestListAdminsSkipsDisabled(t *testing.T) {
	us := newTestUsersStore(t, disabledFixture)
	admins, err := us.ListAdmins()
	if err != nil || len(admins) != 1 || admins[0].TelegramID != 100 {
		t.Fatalf("admins = %+v, %v", admins, err)
	}
}

func TestBroadcastSkipsDisabled(t *testing.T) {
	bot, ft := newFakeTelegram(t)
	us := newTestUsersStore(t, disabledFixture)
	sendBroadcast(adminCtx(t, bot, us), "hi", nil)
	if ft.sentTo("200") || ft.sentTo("300") || !ft.sentTo("400") {
		t.Fatalf("sends = %v", ft.sent())
	}
}

func TestDirectMessageToDisabledRefused(t *testing.T) {
	bot, ft := newFakeTelegram(t)
	us := newTestUsersStore(t, disabledFixture)
	sendAdminMessageToUser(adminCtx(t, bot, us), 200, "vasya", "hi", nil)
	if ft.sentTo("200") {
		t.Fatalf("message delivered to disabled user: %v", ft.sent())
	}
}

func TestSupportSkipsDisabled(t *testing.T) {
	bot, ft := newFakeTelegram(t)
	us := newTestUsersStore(t, disabledFixture)
	petya, _, _ := us.GetByID(400)
	ctx := &Ctx{Bot: bot, ChatID: 400, TgID: 400, User: petya, UsersStore: us, IsPrivate: true}
	sendSupportMessage(ctx, "help", nil)
	if ft.sentTo("300") || !ft.sentTo("100") {
		t.Fatalf("sends = %v", ft.sent())
	}
}

func TestRiskNotificationSkipsDisabled(t *testing.T) {
	bot, ft := newFakeTelegram(t)
	us := newTestUsersStore(t, disabledFixture)
	err := deliverRiskNotification(bot, us, riskNotificationEvent{ProfileKind: "wg", ProfileName: "vasya_phone", Action: "notify"})
	if err != nil {
		t.Fatal(err)
	}
	if ft.sentTo("200") || ft.sentTo("300") || !ft.sentTo("100") {
		t.Fatalf("sends = %v", ft.sent())
	}
	if msg := formatRiskAdminMessage(riskNotificationEvent{}, []User{{Name: "vasya", TelegramID: 200, Disabled: true}}); !strings.Contains(msg, "отключён") {
		t.Errorf("admin message does not mark the disabled owner:\n%s", msg)
	}
}

func TestJoinRequestSkipsDisabledAdmins(t *testing.T) {
	useTestSettings(t)
	bot, ft := newFakeTelegram(t)
	us := newTestUsersStore(t, disabledFixture)
	notifyAdminsAboutStranger(bot, us, &tgbotapi.Message{
		From: &tgbotapi.User{ID: 999_002}, Chat: &tgbotapi.Chat{ID: 999_002, Type: "private"}, Text: "hi",
	})
	if ft.sentTo("300") || !ft.sentTo("100") {
		t.Fatalf("sends = %v", ft.sent())
	}
}

func TestSetUserDisabled(t *testing.T) {
	bot, ft := newFakeTelegram(t)
	us := newTestUsersStore(t, disabledFixture)
	ctx := adminCtx(t, bot, us)

	if _, err := setUserDisabled(ctx, ctx.User, true); err == nil {
		t.Fatal("admin disabled itself")
	}

	// disabling is silent and drops unfinished UI state
	petya, _, _ := us.GetByID(400)
	setMtpPending(400, mtpPending{Kind: mtpPendingAdd})
	setConv(400, 400, ConvState{Mode: ConvSupportAwaitText})
	if _, err := setUserDisabled(ctx, petya, true); err != nil {
		t.Fatal(err)
	}
	if u, _, _ := us.GetByID(400); !u.Disabled {
		t.Fatal("not disabled")
	}
	if _, ok := popMtpPending(400); ok {
		t.Error("pending kept")
	}
	if _, ok := getConv(400, 400); ok {
		t.Error("conversation kept")
	}
	if ft.sentTo("400") {
		t.Errorf("disabled user was notified: %v", ft.sent())
	}

	// enabling notifies the user
	petya, _, _ = us.GetByID(400)
	if _, err := setUserDisabled(ctx, petya, false); err != nil {
		t.Fatal(err)
	}
	if u, _, _ := us.GetByID(400); u.Disabled {
		t.Fatal("not enabled")
	}
	if !ft.sentTo("400") {
		t.Errorf("enabled user was not notified: %v", ft.sent())
	}
}
