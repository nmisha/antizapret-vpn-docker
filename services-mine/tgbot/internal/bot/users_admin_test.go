package bot

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func newTestUsersStore(t *testing.T, json string) *UsersStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "users.json")
	if err := os.WriteFile(path, []byte(json), 0600); err != nil {
		t.Fatal(err)
	}
	return NewUsersStore(path)
}

// fakeTelegram records chat_id of every sendMessage/editMessageText call.
type fakeTelegram struct {
	mu    sync.Mutex
	sends []string // "<method>:<chat_id>"
	texts []string // text of each send, same order
}

func newFakeTelegram(t *testing.T) (*tgbotapi.BotAPI, *fakeTelegram) {
	t.Helper()
	ft := &fakeTelegram{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = r.ParseForm()
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		if method == "getMe" {
			fmt.Fprint(w, `{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"t","username":"t_bot"}}`)
			return
		}
		ft.mu.Lock()
		ft.sends = append(ft.sends, method+":"+r.FormValue("chat_id"))
		ft.texts = append(ft.texts, r.FormValue("text"))
		ft.mu.Unlock()
		fmt.Fprint(w, `{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":1,"type":"private"}}}`)
	}))
	t.Cleanup(srv.Close)
	bot, err := tgbotapi.NewBotAPIWithAPIEndpoint("test", srv.URL+"/bot%s/%s")
	if err != nil {
		t.Fatal(err)
	}
	return bot, ft
}

func (f *fakeTelegram) lastText() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.texts) == 0 {
		return ""
	}
	return f.texts[len(f.texts)-1]
}

func (f *fakeTelegram) sent() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.sends...)
}

func useTestSettings(t *testing.T) {
	t.Helper()
	prev, prevCache := gSettings, getSettingsCached()
	gSettings = NewSettingsStore(filepath.Join(t.TempDir(), "users.json"))
	if err := gSettings.Ensure(); err != nil {
		t.Fatal(err)
	}
	loadSettingsIntoCache()
	t.Cleanup(func() { gSettings = prev; setSettingsCached(prevCache) })
}

func TestProfilePrefixes(t *testing.T) {
	us := newTestUsersStore(t, `[{"telegram_id":1,"name":"vasya","roles":[],"wg_profiles":["Vasya_"]}]`)

	u, err := us.AddProfilePrefix("Vasya", profileMtproto, "Vasya")
	if err != nil {
		t.Fatal(err)
	}
	if got := u.MtprotoProfilesRaw; len(got) != 1 || got[0] != "Vasya" {
		t.Fatalf("raw = %v", got)
	}
	if u.MtprotoProfiles[0] != "vasya" {
		t.Errorf("normalized = %v", u.MtprotoProfiles)
	}
	if _, err := us.AddProfilePrefix("vasya", profileMtproto, "VASYA"); err == nil {
		t.Error("case-insensitive duplicate accepted")
	}
	if _, err := us.AddProfilePrefix("vasya", profileOvpn, "a\nb"); err == nil {
		t.Error("control characters accepted")
	}

	// remove by value (case-insensitive) and by callback token
	if _, removed, err := us.RemoveProfilePrefix("vasya", profileWg, "vasya_"); err != nil || removed != "Vasya_" {
		t.Fatalf("remove by value: %q, %v", removed, err)
	}
	if _, removed, err := us.RemoveProfilePrefixByToken("vasya", profileMtproto, prefixToken("Vasya")); err != nil || removed != "Vasya" {
		t.Fatalf("remove by token: %q, %v", removed, err)
	}
	if _, _, err := us.RemoveProfilePrefix("vasya", profileMtproto, "nope"); err == nil {
		t.Error("missing prefix removed")
	}
}

func TestParsePrefixArgs(t *testing.T) {
	name, kind, prefix, err := parsePrefixArgs("vasya  OpenVPN  Vasya Phone")
	if err != nil || name != "vasya" || kind != profileOvpn || prefix != "Vasya Phone" {
		t.Fatalf("%q %q %q %v", name, kind, prefix, err)
	}
	if _, _, _, err := parsePrefixArgs("vasya xyz p"); err == nil {
		t.Error("unknown kind accepted")
	}
	if _, _, _, err := parsePrefixArgs("vasya wg"); err == nil {
		t.Error("missing prefix accepted")
	}
}

// Telegram rejects callback data longer than 64 bytes, which would break the whole keyboard.
func TestUsersAdminCallbackDataFits(t *testing.T) {
	maxID := idStr(9_223_372_036_854_775_807)
	longest := []string{
		usrCb("pd", string(profileMtproto), maxID, prefixToken(strings.Repeat("я", 64))),
		usrCb("rt", maxID, string(RoleMtprotoUserControl)),
		usrCb("j", "as", maxID, strings.Repeat("a", 32)),
		confirmCbPrefix + "usr:del:" + maxID,
	}
	for _, cb := range longest {
		if len(cb) > 64 {
			t.Errorf("%d bytes: %s", len(cb), cb)
		}
	}
}

func TestJoinRequestNotifiesAdminsOnly(t *testing.T) {
	useTestSettings(t)
	bot, ft := newFakeTelegram(t)
	us := newTestUsersStore(t, `[
		{"telegram_id":100,"name":"boss","roles":["Admin"]},
		{"telegram_id":200,"name":"user","roles":["WgUserControl"]}]`)

	const stranger int64 = 999_001
	msg := &tgbotapi.Message{
		From: &tgbotapi.User{ID: stranger, FirstName: "Петя", UserName: "Petya_1"},
		Chat: &tgbotapi.Chat{ID: stranger, Type: "private"},
		Text: "/start",
	}
	notifyAdminsAboutStranger(bot, us, msg)
	if got := ft.sent(); len(got) != 1 || got[0] != "sendMessage:100" {
		t.Fatalf("sends = %v, want only the admin", got)
	}

	// once per day per person
	notifyAdminsAboutStranger(bot, us, msg)
	if got := ft.sent(); len(got) != 1 {
		t.Fatalf("repeat notified: %v", got)
	}

	// group messages and ignored people are never reported
	group := *msg
	group.From = &tgbotapi.User{ID: stranger + 1}
	group.Chat = &tgbotapi.Chat{ID: -5, Type: "group"}
	notifyAdminsAboutStranger(bot, us, &group)

	if err := setJoinIgnored(stranger+2, true); err != nil {
		t.Fatal(err)
	}
	ignored := *msg
	ignored.From = &tgbotapi.User{ID: stranger + 2}
	ignored.Chat = &tgbotapi.Chat{ID: stranger + 2, Type: "private"}
	notifyAdminsAboutStranger(bot, us, &ignored)

	if got := ft.sent(); len(got) != 1 {
		t.Fatalf("group or ignored reported: %v", got)
	}

	// the ignore list is persisted
	s, err := gSettings.Load()
	if err != nil || len(s.IgnoredJoinIDs) != 1 || s.IgnoredJoinIDs[0] != stranger+2 {
		t.Fatalf("persisted = %v, %v", s.IgnoredJoinIDs, err)
	}
}

func TestJoinGlobalLimit(t *testing.T) {
	joinGlobalMu.Lock()
	saved := joinGlobalSent
	joinGlobalSent = nil
	joinGlobalMu.Unlock()
	t.Cleanup(func() { joinGlobalMu.Lock(); joinGlobalSent = saved; joinGlobalMu.Unlock() })

	now := time.Now()
	for i := 0; i < joinGlobalMax; i++ {
		if !joinGlobalAllow(now) {
			t.Fatalf("blocked at %d", i)
		}
	}
	if joinGlobalAllow(now) {
		t.Fatal("limit not enforced")
	}
	if !joinGlobalAllow(now.Add(joinGlobalWindow)) {
		t.Fatal("window did not expire")
	}
}

func TestJoinAddCreatesUserAndWelcomes(t *testing.T) {
	useTestSettings(t)
	bot, ft := newFakeTelegram(t)
	us := newTestUsersStore(t, `[{"telegram_id":100,"name":"boss","roles":["Admin"]}]`)
	admin, _, _ := us.GetByID(100)
	ctx := &Ctx{Bot: bot, ChatID: 100, TgID: 100, User: admin, UsersStore: us, IsPrivate: true}

	handleUsersAdminCallback(ctx, usrCb("j", "as", "555", "petya_1"))

	u, ok, err := us.GetByID(555)
	if err != nil || !ok || u.Name != "petya_1" || len(u.RolesRaw) != 0 {
		t.Fatalf("user = %+v, %v, %v", u, ok, err)
	}
	welcomed := false
	for _, s := range ft.sent() {
		if s == "sendMessage:555" {
			welcomed = true
		}
	}
	if !welcomed {
		t.Errorf("new user not welcomed: %v", ft.sent())
	}
}

func TestToggleRoleKeepsOwnAdmin(t *testing.T) {
	bot, _ := newFakeTelegram(t)
	us := newTestUsersStore(t, `[{"telegram_id":100,"name":"boss","roles":["Admin"]},{"telegram_id":200,"name":"user","roles":[]}]`)
	admin, _, _ := us.GetByID(100)
	ctx := &Ctx{Bot: bot, ChatID: 100, TgID: 100, User: admin, UsersStore: us, IsPrivate: true}

	toggleUserRole(ctx, 100, "Admin")
	if u, _, _ := us.GetByID(100); !u.HasExact(RoleAdmin) {
		t.Fatal("admin removed own Admin role")
	}

	toggleUserRole(ctx, 200, "mtprotousercontrol")
	if u, _, _ := us.GetByID(200); !u.HasExact(RoleMtprotoUserControl) {
		t.Fatal("role not granted")
	}
	toggleUserRole(ctx, 200, "MtprotoUserControl")
	if u, _, _ := us.GetByID(200); u.HasExact(RoleMtprotoUserControl) {
		t.Fatal("role not revoked")
	}
}

func TestUsersAdminRequiresAdmin(t *testing.T) {
	bot, _ := newFakeTelegram(t)
	us := newTestUsersStore(t, `[{"telegram_id":200,"name":"user","roles":["WgUserControl"]}]`)
	u, _, _ := us.GetByID(200)
	ctx := &Ctx{Bot: bot, ChatID: 200, TgID: 200, User: u, UsersStore: us, IsPrivate: true}

	handleUsersAdminCallback(ctx, usrCb("rt", "200", "Admin"))
	if u, _, _ := us.GetByID(200); u.HasExact(RoleAdmin) {
		t.Fatal("non-admin granted itself Admin")
	}
}

func TestPrefixRemovalSeparatesValueAndToken(t *testing.T) {
	us := newTestUsersStore(t, `[{"telegram_id":1,"name":"a","roles":[]}]`)
	token := prefixToken("some_prefix")
	for _, p := range []string{"some_prefix", token} {
		if _, err := us.AddProfilePrefix("a", profileWg, p); err != nil {
			t.Fatal(err)
		}
	}
	if _, removed, err := us.RemoveProfilePrefix("a", profileWg, token); err != nil || removed != token {
		t.Fatalf("value removal = %q, %v", removed, err)
	}
	if _, removed, err := us.RemoveProfilePrefixByToken("a", profileWg, token); err != nil || removed != "some_prefix" {
		t.Fatalf("token removal = %q, %v", removed, err)
	}
}

func TestPrefixTokenCollisionDoesNotRemoveAnything(t *testing.T) {
	const first, second = "pb8a5b6213584", "pbaa888a42e75"
	if prefixToken(first) != prefixToken(second) {
		t.Fatal("fixture is not a CRC32 collision")
	}
	us := newTestUsersStore(t, `[{"telegram_id":1,"name":"a","roles":[]}]`)
	for _, p := range []string{first, second} {
		if _, err := us.AddProfilePrefix("a", profileWg, p); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := us.RemoveProfilePrefixByToken("a", profileWg, prefixToken(first)); err == nil {
		t.Fatal("ambiguous token accepted")
	}
	u, _, _ := us.GetByID(1)
	if len(u.WgProfilesRaw) != 2 {
		t.Fatal("collision changed the list")
	}
	if _, removed, err := us.RemoveProfilePrefix("a", profileWg, second); err != nil || removed != second {
		t.Fatalf("exact removal = %q, %v", removed, err)
	}
}
