package bot

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestHelpLongUnknownGroup(t *testing.T) {
	helpTestRegistry(t)
	for _, input := range []string{strings.Repeat("a", 4000), strings.Repeat("я<&", 1300)} {
		bot, ft := newFakeTelegram(t)
		ctx := &Ctx{Bot: bot, ChatID: 100, TgID: 100, IsPrivate: true,
			User: User{Roles: map[Role]bool{RoleAdmin: true}}}
		handleHelp(ctx, input)
		got := ft.lastText()
		if !utf8.ValidString(got) || utf8.RuneCountInString(got) > helpScreenMaxRunes {
			t.Fatal("unknown-group response exceeds the screen limit or contains invalid UTF-8")
		}
		if len(ft.sent()) != 1 || !strings.Contains(got, "не найден") || !strings.Contains(got, "Выбери раздел") || !strings.Contains(got, "…") {
			t.Fatalf("unexpected unknown-group response: %q", got)
		}
		if strings.Contains(got, "я<&") {
			t.Fatal("group name was not HTML-escaped")
		}
	}
}

func helpTestRegistry(t *testing.T) *CommandRegistry {
	t.Helper()
	t.Setenv("TELEMT_TOKEN", "tok")
	reg := NewCommandRegistry(NewRouter())
	for _, register := range []func(*CommandRegistry){
		RegisterAdminHandlers, RegisterUsersAdminHandlers, RegisterAdminAccountsHandlers, RegisterAdminAccountsUIHandlers,
		RegisterAdminSettingsHandlers, RegisterDomainHandlers, RegisterGuardNotifyHandlers, RegisterServiceHandlers,
		RegisterWgProfilesHandlers, RegisterAwgStatsHandlers, RegisterAwgProfilesHandlers, RegisterOvpnStatsHandlers,
		RegisterOvpnProfilesHandlers, RegisterMtprotoHandlers, RegisterWgNameCommands, RegisterAwgNameCommands,
		RegisterOvpnNameCommands, RegisterNetHandlers, RegisterSupportHandlers, RegisterAIHandlers,
	} {
		register(reg)
	}
	prev := gCmdRegistry
	gCmdRegistry = reg
	t.Cleanup(func() { gCmdRegistry = prev })
	return reg
}

func TestHelpShortListStaysFlat(t *testing.T) {
	helpTestRegistry(t)
	bot, ft := newFakeTelegram(t)
	ctx := &Ctx{Bot: bot, ChatID: 400, TgID: 400, IsPrivate: true,
		User: User{Roles: map[Role]bool{RoleWgUserControl: true, RoleWgStats: true, RoleMtprotoUserControl: true, RoleDomainEditor: true}}}

	handleHelp(ctx, "")
	got := ft.lastText()
	if len(ft.sent()) != 1 || strings.Contains(got, "Выбери раздел") || !strings.Contains(got, "/wg_profiles") || !strings.Contains(got, "/mtproto") {
		t.Fatalf("short help: %d messages, %q", len(ft.sent()), got)
	}
	if strings.Contains(got, "/wg_profiles_admin") {
		t.Fatal("admin command shown to a user")
	}
}

func TestHelpAdminGroups(t *testing.T) {
	reg := helpTestRegistry(t)
	admin := User{Roles: map[Role]bool{RoleAdmin: true}}

	groups := helpGroupsForUser(reg, admin)
	keys := []string{}
	total := 0
	for _, g := range groups {
		keys = append(keys, g.Key)
		total += g.Count
	}
	if strings.Join(keys, ",") != "start,wg,awg,ovpn,mtproto,dns,accounts,users,bot" {
		t.Fatalf("groups %v", keys)
	}
	if want := len(reg.VisibleSpecsForUser(admin)); total != want {
		t.Fatalf("groups hold %d commands, visible %d", total, want)
	}
	if p := HelpProblems(reg); len(p) != 0 {
		t.Fatalf("help problems: %v", p)
	}

	// commands without arguments (menus) come before the ones with arguments
	for _, g := range groups {
		for _, bl := range g.Blocks {
			seenArgs := false
			for _, it := range bl.Items {
				if it.Args != "" {
					seenArgs = true
				} else if seenArgs {
					t.Fatalf("%s: %s after commands with arguments", bl.Title, it.Cmd)
				}
			}
		}
	}
}

func TestHelpAdminScreens(t *testing.T) {
	helpTestRegistry(t)
	bot, ft := newFakeTelegram(t)
	ctx := &Ctx{Bot: bot, ChatID: 100, TgID: 100, IsPrivate: true, User: User{Roles: map[Role]bool{RoleAdmin: true}}}

	handleHelp(ctx, "")
	if got := ft.lastText(); !strings.Contains(got, "Выбери раздел") || strings.Contains(got, "/wg_conf") {
		t.Fatalf("admin index: %q", got)
	}

	handleHelp(ctx, "WG")
	got := ft.lastText()
	for _, cmd := range []string{"/wg_profiles", "/wg_conf", "/wg_profiles_admin", "Админ: WireGuard"} {
		if !strings.Contains(got, cmd) {
			t.Fatalf("wg group misses %s: %q", cmd, got)
		}
	}
	if strings.Contains(got, "/awg_") || strings.Contains(got, "/ovpn_") {
		t.Fatal("wg group shows AWG or OpenVPN")
	}

	// a button press edits the menu in place
	ctx.MessageID = 7
	handleHelpCallback(ctx, helpCbPrefix+"dns")
	if ft.count("editMessageText:100") != 1 || !strings.Contains(ft.lastText(), "/agh_update_lists") {
		t.Fatalf("dns callback: %v %q", ft.sent(), ft.lastText())
	}
	ctx.MessageID = 0

	handleHelp(ctx, "nope")
	if got := ft.lastText(); !strings.Contains(got, "не найден") || !strings.Contains(got, "Выбери раздел") {
		t.Fatalf("unknown group: %q", got)
	}

	before := len(ft.sent())
	handleHelp(ctx, "all")
	if n := len(ft.sent()) - before; n < 2 {
		t.Fatalf("/help all sent %d messages, want the full list in chunks", n)
	}

	before = len(ft.sent())
	handleStart(ctx, "deep-link-payload")
	if len(ft.sent())-before != 1 || !strings.Contains(ft.lastText(), "Выбери раздел") {
		t.Fatalf("/start with payload: %q", ft.lastText())
	}
}
