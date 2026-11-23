package bot

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestReplyServiceErrorHidesDetailsFromUsers(t *testing.T) {
	bot, ft := newFakeTelegram(t)
	internal := errors.New(`Get "http://telemt.antizapret:9091/v1/users": dial tcp: lookup telemt.antizapret: no such host`)

	user := &Ctx{Bot: bot, ChatID: 400, TgID: 400, User: User{Name: "petya", Roles: map[Role]bool{}}}
	replyServiceError(user, "MTProto", "Не удалось получить список прокси", internal)
	got := ft.lastText()
	if strings.Contains(got, "telemt.antizapret") || !strings.Contains(got, "MTProto") || !strings.Contains(got, "/support") {
		t.Fatalf("user saw: %q", got)
	}

	admin := &Ctx{Bot: bot, ChatID: 100, TgID: 100, User: User{Name: "boss", Roles: map[Role]bool{RoleAdmin: true}}}
	replyServiceError(admin, "MTProto", "Не удалось получить список прокси", internal)
	if got := ft.lastText(); !strings.Contains(got, "telemt.antizapret") {
		t.Fatalf("admin did not see details: %q", got)
	}

	// messages meant for the user are shown as is, wrapped or not
	replyServiceError(user, "MTProto", "Доступ к прокси недоступен", fmt.Errorf("check: %w", userErrorf("доступ к прокси отозван")))
	if got := ft.lastText(); got != "Доступ к прокси недоступен: доступ к прокси отозван" {
		t.Fatalf("user error: %q", got)
	}
}

func TestValidateMtpUserAccessNotFoundIsUserError(t *testing.T) {
	c := newTestTelemtClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"ok":false,"error":{"code":"not_found","message":"user not found"},"request_id":1}`)
	})
	ctx := &Ctx{User: User{Roles: map[Role]bool{RoleMtprotoUserControl: true}, MtprotoProfiles: []string{"vasya"}}}

	_, err := validateMtpUserAccess(ctx, c, "vasya_phone")
	var ue userError
	if !errors.As(err, &ue) || ue.msg != "прокси больше не найден" {
		t.Fatalf("err = %#v", err)
	}
}
