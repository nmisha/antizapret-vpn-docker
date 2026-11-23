package bot

import "testing"

func TestCancelNeverReachesPendingHandlers(t *testing.T) {
	bot, ft := newFakeTelegram(t)
	const id int64 = 76547
	admin := User{Name: "boss", Roles: map[Role]bool{RoleAdmin: true}}
	ctx := &Ctx{Bot: bot, ChatID: id, TgID: id, User: admin, IsPrivate: true}

	// /cancel while a WG rename waits for the new name: answered here, the profile is not renamed
	for _, text := range []string{"/cancel", "/CANCEL@test_bot"} {
		setWgPending(id, wgPending{Kind: wgPendingRename, PeerID: "1"})
		cmd, _ := splitCmd(text)
		if !resetPrivateFlowsOnCommand(ctx, text, cmd) {
			t.Fatalf("%s not consumed", text)
		}
		if got := ft.lastText(); got != "Ок, отменил." {
			t.Fatalf("%s answered %q", text, got)
		}
		if handleWgPendingIfAny(ctx, text) {
			t.Fatalf("%s reached the WG rename handler", text)
		}
	}

	// nothing to cancel
	if !resetPrivateFlowsOnCommand(ctx, "/cancel", "/cancel") || ft.lastText() != "Нечего отменять." {
		t.Fatalf("empty cancel answered %q", ft.lastText())
	}

	// a wizard counts too
	setConv(id, id, ConvState{Mode: ConvSupportAwaitText})
	if !resetPrivateFlowsOnCommand(ctx, "/cancel", "/cancel") || ft.lastText() != "Ок, отменил." {
		t.Fatalf("wizard cancel answered %q", ft.lastText())
	}
	if _, ok := getConv(id, id); ok {
		t.Fatal("wizard kept")
	}
}

func TestOtherCommandsDropPendingAndRun(t *testing.T) {
	const id int64 = 76548
	ctx := &Ctx{ChatID: id, TgID: id, User: User{Roles: map[Role]bool{RoleAdmin: true}}, IsPrivate: true}

	setAccAdminPending(id, accAdminPending{Kind: accAdminSetPass, Name: "x"})
	if resetPrivateFlowsOnCommand(ctx, "/help", "/help") {
		t.Fatal("/help consumed: it must still be dispatched")
	}
	if _, ok := popAccAdminPending(id); ok {
		t.Fatal("pending kept after another command")
	}

	// plain text is input for the pending handler
	setAccAdminPending(id, accAdminPending{Kind: accAdminSetPass, Name: "x"})
	t.Cleanup(func() { popAccAdminPending(id) })
	if resetPrivateFlowsOnCommand(ctx, "secret", "secret") {
		t.Fatal("plain text consumed")
	}
	if _, ok := popAccAdminPending(id); !ok {
		t.Fatal("plain text dropped the pending input")
	}
}

func TestGroupCommandsKeepPrivatePending(t *testing.T) {
	const id int64 = 76549
	ctx := &Ctx{ChatID: -5, TgID: id, User: User{Roles: map[Role]bool{RoleAdmin: true}}}

	setMtpPending(id, mtpPending{Kind: mtpPendingAdd})
	t.Cleanup(func() { popMtpPending(id) })
	if resetPrivateFlowsOnCommand(ctx, "/cancel", "/cancel") {
		t.Fatal("group /cancel consumed")
	}
	if _, ok := popMtpPending(id); !ok {
		t.Fatal("group command dropped the private pending input")
	}
}
