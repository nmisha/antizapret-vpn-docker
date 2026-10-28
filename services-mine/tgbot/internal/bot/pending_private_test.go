package bot

import "testing"

// Admin text-input waits are keyed by user only, so input from a group must not consume them.
func TestPendingHandlersIgnoreGroupInput(t *testing.T) {
	const id int64 = 76545
	admin := User{Roles: map[Role]bool{RoleAdmin: true}}

	cases := []struct {
		name   string
		set    func()
		handle func(*Ctx, string) bool
		pop    func() bool
	}{
		{"wg", func() { setWgPending(id, wgPending{Kind: wgPendingAdd}) }, handleWgPendingIfAny,
			func() bool { _, ok := popWgPending(id); return ok }},
		{"awg", func() { setAwgPending(id, wgPending{Kind: wgPendingAdd}) }, handleAwgPendingIfAny,
			func() bool { _, ok := popAwgPending(id); return ok }},
		{"ovpn", func() { setOvpnPending(id, ovpnPending{Kind: ovpnPendingAdd}) }, handleOvpnPendingIfAny,
			func() bool { _, ok := popOvpnPending(id); return ok }},
		{"accounts", func() { setAccAdminPending(id, accAdminPending{Kind: accAdminAdd}) }, handleAccAdminPendingIfAny,
			func() bool { _, ok := popAccAdminPending(id); return ok }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.set()
			t.Cleanup(func() { tc.pop() })
			ctx := &Ctx{TgID: id, ChatID: -123, User: admin}
			if tc.handle(ctx, "name") {
				t.Fatal("group input was consumed")
			}
			if !tc.pop() {
				t.Fatal("pending lost")
			}
		})
	}
}

func TestClearPrivatePendings(t *testing.T) {
	const id int64 = 76546
	setWgPending(id, wgPending{Kind: wgPendingAdd})
	setAwgPending(id, wgPending{Kind: wgPendingAdd})
	setOvpnPending(id, ovpnPending{Kind: ovpnPendingAdd})
	setMtpPending(id, mtpPending{Kind: mtpPendingAdd})
	setAccAdminPending(id, accAdminPending{Kind: accAdminAdd})

	clearPrivatePendings(id)

	if _, ok := popWgPending(id); ok {
		t.Error("wg pending left")
	}
	if _, ok := popAwgPending(id); ok {
		t.Error("awg pending left")
	}
	if _, ok := popOvpnPending(id); ok {
		t.Error("ovpn pending left")
	}
	if _, ok := popMtpPending(id); ok {
		t.Error("mtproto pending left")
	}
	if _, ok := popAccAdminPending(id); ok {
		t.Error("accounts pending left")
	}
}
