package bot

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Join requests: when someone not in users.json writes to the bot in private, the person still gets
// only a joke (the bot does not reveal itself), and admins silently get a card with Add / Ignore.

const (
	joinPerUserCooldown = 24 * time.Hour
	joinGlobalMax       = 20 // notifications per joinGlobalWindow, against floods from many accounts
	joinGlobalWindow    = time.Hour
)

var joinPerUser = newCooldownLimiter(joinPerUserCooldown)

var (
	joinGlobalMu   sync.Mutex
	joinGlobalSent []time.Time
)

var tgUsernameRe = regexp.MustCompile(`^[A-Za-z0-9_]{1,32}$`)

func joinGlobalAllow(now time.Time) bool {
	joinGlobalMu.Lock()
	defer joinGlobalMu.Unlock()
	kept := joinGlobalSent[:0]
	for _, t := range joinGlobalSent {
		if now.Sub(t) < joinGlobalWindow {
			kept = append(kept, t)
		}
	}
	joinGlobalSent = kept
	if len(joinGlobalSent) >= joinGlobalMax {
		return false
	}
	joinGlobalSent = append(joinGlobalSent, now)
	return true
}

// joinGlobalRelease returns a slot taken by joinGlobalAllow(at) when nothing was delivered.
func joinGlobalRelease(at time.Time) {
	joinGlobalMu.Lock()
	defer joinGlobalMu.Unlock()
	if i := slices.IndexFunc(joinGlobalSent, at.Equal); i >= 0 {
		joinGlobalSent = slices.Delete(joinGlobalSent, i, i+1)
	}
}

func isJoinIgnored(id int64) bool {
	return slices.Contains(getSettingsCached().IgnoredJoinIDs, id)
}

// setJoinIgnored persists the ignore list in bot_settings.json.
func setJoinIgnored(id int64, ignored bool) error {
	_, err := updateSettings(func(s *BotSettings) {
		has := slices.Contains(s.IgnoredJoinIDs, id)
		switch {
		case ignored && !has:
			s.IgnoredJoinIDs = append(s.IgnoredJoinIDs, id)
		case !ignored && has:
			s.IgnoredJoinIDs = slices.DeleteFunc(s.IgnoredJoinIDs, func(v int64) bool { return v == id })
		}
	})
	return err
}

func formatJoinRequest(from *tgbotapi.User, text string) string {
	name := strings.TrimSpace(from.FirstName + " " + from.LastName)
	if name == "" {
		name = "(без имени)"
	}
	var b strings.Builder
	b.WriteString("🔔 <b>Новый человек пишет боту</b>\n")
	fmt.Fprintf(&b, "Имя: %s\n", htmlEscape(name))
	if from.UserName != "" {
		fmt.Fprintf(&b, "Username: @%s\n", htmlEscape(from.UserName))
	}
	fmt.Fprintf(&b, "tg_id: <code>%d</code>\n", from.ID)
	if from.LanguageCode != "" {
		fmt.Fprintf(&b, "Язык: %s\n", htmlEscape(from.LanguageCode))
	}
	fmt.Fprintf(&b, "Сообщение: %s\n\n", htmlEscape(truncate(text, 300)))
	b.WriteString("Человек получил шутку и не знает о заявке. Следующее уведомление о нём — не раньше чем через сутки.")
	return b.String()
}

// notifyAdminsAboutStranger is called for private messages from people not in users.json.
func notifyAdminsAboutStranger(bot *tgbotapi.BotAPI, usersStore *UsersStore, msg *tgbotapi.Message) {
	if msg == nil || msg.From == nil || msg.Chat == nil || !msg.Chat.IsPrivate() || msg.From.IsBot {
		return
	}
	id := msg.From.ID
	if isJoinIgnored(id) {
		return
	}
	// Updates are handled one at a time, so check-then-mark needs no extra locking.
	// Only a successful delivery starts the per-user cooldown and keeps the global slot.
	if joinPerUser.remaining(id, time.Now()) > 0 {
		return
	}
	admins, err := usersStore.ListAdmins()
	if err != nil || len(admins) == 0 {
		return
	}
	admitted := time.Now()
	if !joinGlobalAllow(admitted) {
		return
	}

	sid := idStr(id)
	rows := [][]tgbotapi.InlineKeyboardButton{}
	if tgUsernameRe.MatchString(msg.From.UserName) {
		if name, err := validateBotUserName(msg.From.UserName); err == nil {
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("➕ Добавить как "+name, usrCb("j", "as", sid, name))))
		}
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("➕ Добавить с другим именем", usrCb("j", "add", sid)),
		tgbotapi.NewInlineKeyboardButtonData("🙈 Игнорировать", usrCb("j", "ign", sid)),
	))
	kb := tgbotapi.NewInlineKeyboardMarkup(rows...)
	text := formatJoinRequest(msg.From, msg.Text)

	delivered := false
	for _, a := range admins {
		m := tgbotapi.NewMessage(a.TelegramID, text)
		m.ParseMode = "HTML"
		m.DisableWebPagePreview = true
		m.ReplyMarkup = kb
		_, err := bot.Send(m)
		if err == nil {
			delivered = true
		}
		logSendErrorIfEnabled(a.TelegramID, err, "send message", "join request")
	}
	if !delivered {
		joinGlobalRelease(admitted)
		return
	}
	joinPerUser.mark(id, time.Now())
}

// handleJoinRequestCallback handles "usr:j:<act>:<tg_id>[:<name>]" from a join request card.
func handleJoinRequestCallback(ctx *Ctx, act, rawID, name string) {
	var id int64
	if _, err := fmt.Sscan(rawID, &id); err != nil || id <= 0 {
		reply(ctx.Bot, ctx.ChatID, "Неверный формат.")
		return
	}
	if _, exists, err := ctx.UsersStore.GetByID(id); err == nil && exists {
		reply(ctx.Bot, ctx.ChatID, "Этот человек уже добавлен.")
		ctx.MessageID = 0
		showUserCard(ctx, id)
		return
	}

	switch act {
	case "as":
		createBotUser(ctx, name, id, true)
	case "add":
		setUsrPending(ctx.TgID, usrPending{Kind: usrPendingAdd, TargetID: id})
		reply(ctx.Bot, ctx.ChatID, fmt.Sprintf("Отправь имя для tg_id %d одним сообщением (буквы, цифры, _ . -, до 32 символов).\n/cancel — отмена.", id))
	case "ign":
		if err := setJoinIgnored(id, true); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
			return
		}
		reply(ctx.Bot, ctx.ChatID, fmt.Sprintf("Ок, о %d больше не сообщаю. Снять: /users_admin → Игнор.", id))
	default:
		reply(ctx.Bot, ctx.ChatID, "Неизвестное действие.")
	}
}
