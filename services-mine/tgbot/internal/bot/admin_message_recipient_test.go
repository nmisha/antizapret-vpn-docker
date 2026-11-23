package bot

import (
	"os"
	"strings"
	"testing"
)

func TestAdminMessageRefusesUnreadableRecipientStore(t *testing.T) {
	for _, kind := range []string{"missing", "invalid_json"} {
		t.Run(kind, func(t *testing.T) {
			bot, ft := newFakeTelegram(t)
			us := newTestUsersStore(t, disabledFixture)
			ctx := adminCtx(t, bot, us)
			var err error
			if kind == "missing" {
				err = os.Remove(us.Path)
			} else {
				err = os.WriteFile(us.Path, []byte("{"), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			sendAdminMessageToUser(ctx, 400, "petya", "private message", nil)
			if ft.sentTo("400") {
				t.Fatalf("message sent without recipient validation: %v", ft.sent())
			}
			if !ft.sentTo("100") || !strings.Contains(ft.lastText(), "сообщение не отправлено") {
				t.Fatalf("admin did not receive a failure notice: %q", ft.lastText())
			}
		})
	}
}
