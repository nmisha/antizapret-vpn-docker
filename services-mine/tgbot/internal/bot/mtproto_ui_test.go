package bot

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestMtpPendingIgnoresGroupInput(t *testing.T) {
	const id int64 = 76543
	for _, kind := range []mtpPendingKind{mtpPendingAdd, mtpPendingLimit} {
		t.Run(string(kind), func(t *testing.T) {
			pending := mtpPending{Kind: kind, Name: "alice", Field: "ips"}
			setMtpPending(id, pending)
			t.Cleanup(func() { popMtpPending(id) })
			ctx := &Ctx{TgID: id, ChatID: -123, User: User{Roles: map[Role]bool{RoleAdmin: true}}}
			if handleMtpPendingIfAny(ctx, "2") {
				t.Fatal("group input was consumed")
			}
			if got, ok := popMtpPending(id); !ok || got != pending {
				t.Fatalf("pending lost: %+v, %v", got, ok)
			}
		})
	}
}

func TestMtpCancelCallback(t *testing.T) {
	const id int64 = 76544
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"test","username":"test_bot"}}`)
	}))
	defer srv.Close()
	bot, err := tgbotapi.NewBotAPIWithAPIEndpoint("test", srv.URL+"/bot%s/%s")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "users.json")
	if err := os.WriteFile(path, []byte(fmt.Sprintf(`[{"telegram_id":%d,"name":"admin","roles":["Admin"]}]`, id)), 0600); err != nil {
		t.Fatal(err)
	}
	store := NewUsersStore(path)
	for _, chatType := range []string{"group", "private"} {
		t.Run(chatType, func(t *testing.T) {
			setMtpPending(id, mtpPending{Kind: mtpPendingAdd})
			t.Cleanup(func() { popMtpPending(id) })
			handleCallback(bot, store, nil, nil, &tgbotapi.CallbackQuery{
				ID: "query", From: &tgbotapi.User{ID: id}, Data: "ui:cancel",
				Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: id, Type: chatType}},
			})
			_, pending := popMtpPending(id)
			if pending != (chatType == "group") {
				t.Fatalf("pending after %s cancel = %v", chatType, pending)
			}
		})
	}
}

func TestParseMtpSizeBounds(t *testing.T) {
	for _, base := range []float64{1000, 1024} {
		for _, input := range []string{"NaN", "Inf", "+Inf", "-Inf", "1e309", "1e308T", "18446744073709551616", "100000000T", "0.5", "0", "-1"} {
			if got, err := parseSizeWithSuffix(input, base); err == nil {
				t.Errorf("base %v: %q accepted as %d", base, input, got)
			}
		}
		for input, want := range map[string]uint64{"1": 1, "1.5M": uint64(1.5 * base * base), "0.5K": uint64(base / 2)} {
			if got, err := parseSizeWithSuffix(input, base); err != nil || got != want {
				t.Errorf("base %v: %q = %d, %v; want %d", base, input, got, err, want)
			}
		}
	}
}
