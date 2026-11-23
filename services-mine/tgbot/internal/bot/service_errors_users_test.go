package bot

import (
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestMtprotoSectionNeedsToken(t *testing.T) {
	t.Setenv("TELEMT_TOKEN", "")
	reg := NewCommandRegistry(NewRouter())
	RegisterMtprotoHandlers(reg)
	if _, ok := reg.R.handlers["/mtproto"]; ok {
		t.Fatal("/mtproto registered without TELEMT_TOKEN")
	}

	bot, ft := newFakeTelegram(t)
	ctx := &Ctx{Bot: bot, ChatID: 100, TgID: 100, User: User{Roles: map[Role]bool{RoleAdmin: true}}, IsPrivate: true}
	handleMtprotoCallback(ctx, "mtp:a:scope:all")
	if got := ft.lastText(); got != mtprotoDisabledText {
		t.Fatalf("stale button answered %q", got)
	}

	t.Setenv("TELEMT_TOKEN", "tok")
	reg = NewCommandRegistry(NewRouter())
	RegisterMtprotoHandlers(reg)
	if _, ok := reg.R.handlers["/mtproto"]; !ok {
		t.Fatal("/mtproto not registered with TELEMT_TOKEN")
	}
}

func TestWgProfilesHideConfigFromUsers(t *testing.T) {
	t.Setenv("WG_HOST", "")
	t.Setenv("WG_PASSWORD", "")
	bot, ft := newFakeTelegram(t)
	ctx := &Ctx{Bot: bot, ChatID: 400, TgID: 400, IsPrivate: true,
		User: User{Name: "petya", Roles: map[Role]bool{RoleWgUserControl: true}, WgProfiles: []string{"petya"}}}

	handleWgProfiles(ctx, "")
	if got := ft.lastText(); strings.Contains(got, "WG_") || !strings.Contains(got, "(WireGuard)") {
		t.Fatalf("user saw: %q", got)
	}
}

func TestDomainsHideFileErrorsFromUsers(t *testing.T) {
	bot, ft := newFakeTelegram(t)
	dir := t.TempDir() // a directory instead of the domains file: reading fails
	ctx := &Ctx{Bot: bot, ChatID: 400, TgID: 400, Domains: NewStore(dir),
		User: User{Name: "petya", Roles: map[Role]bool{RoleDomainEditor: true}}}

	handleList(ctx, "")
	if got := ft.lastText(); strings.Contains(got, dir) || !strings.Contains(got, "(домены)") {
		t.Fatalf("user saw: %q", got)
	}

	// input errors are still explained
	handleAdd(ctx, "not a domain!")
	if got := ft.lastText(); !strings.HasPrefix(got, "Ошибка: ") {
		t.Fatalf("input error: %q", got)
	}
}

func TestBrokenUsersFileLooksLikeStranger(t *testing.T) {
	bot, ft := newFakeTelegram(t)
	us := newTestUsersStore(t, `{broken`)

	handleCallback(bot, us, nil, nil, &tgbotapi.CallbackQuery{
		ID: "q", From: &tgbotapi.User{ID: 999_003}, Data: "usr:list",
		Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 999_003, Type: "private"}},
	})
	if got := ft.lastText(); strings.Contains(got, "users.json") || strings.Contains(got, us.Path) || got == "" {
		t.Fatalf("stranger saw: %q", got)
	}
}
