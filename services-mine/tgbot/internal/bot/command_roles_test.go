package bot

import (
	"strings"
	"testing"
)

// The roles that show a command in /help are enforced when it is called.
func TestCommandsEnforceNeedAny(t *testing.T) {
	reg := helpTestRegistry(t)
	bot, ft := newFakeTelegram(t)
	nobody := &Ctx{Bot: bot, ChatID: 400, TgID: 400, IsPrivate: true, User: User{Name: "nobody", Roles: map[Role]bool{}}}

	checked := 0
	for _, spec := range reg.Specs() {
		if len(spec.NeedAny) == 0 {
			continue
		}
		if !reg.R.Dispatch(nobody, spec.Cmd, "arg") {
			t.Fatalf("%s is not routed", spec.Cmd)
		}
		if got := ft.lastText(); got != needAnyDeniedText(spec.NeedAny) {
			t.Fatalf("%s without roles answered %q", spec.Cmd, got)
		}
		checked++
	}
	if checked < 80 {
		t.Fatalf("only %d commands checked", checked)
	}

	// a role from NeedAny passes the check
	admin := &Ctx{Bot: bot, ChatID: 100, TgID: 100, IsPrivate: true, User: User{Roles: map[Role]bool{RoleAdmin: true}}}
	reg.R.Dispatch(admin, "/roles_catalog", "")
	if got := ft.lastText(); strings.HasPrefix(got, "Недостаточно прав") || !strings.Contains(got, "Доступные роли") {
		t.Fatalf("admin /roles_catalog: %q", got)
	}
}

func TestNeedAnyDeniedText(t *testing.T) {
	for _, tc := range []struct {
		roles []Role
		want  string
	}{
		{[]Role{RoleAdmin}, "Недостаточно прав. Нужна роль Admin."},
		{[]Role{RoleWgStats}, "Недостаточно прав. Нужна роль WgStats (или Admin)."},
		{[]Role{RoleDomainEditor, RoleServiceManager}, "Недостаточно прав. Нужна роль DomainEditor или ServiceManager (или Admin)."},
	} {
		if got := needAnyDeniedText(tc.roles); got != tc.want {
			t.Errorf("%v: %q", tc.roles, got)
		}
	}
}
