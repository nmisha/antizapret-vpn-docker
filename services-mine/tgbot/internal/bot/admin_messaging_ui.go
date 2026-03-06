package bot

import (
	"fmt"
	"sort"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const adminMsgCbPrefix = "admmsg:" // admin messaging UI callbacks

func admmsg(parts ...string) string {
	// admmsg:<p1>:<p2>:...
	b := strings.Builder{}
	b.WriteString(adminMsgCbPrefix)
	for i, p := range parts {
		if i > 0 {
			b.WriteString(":")
		}
		// keep callback data simple (no ':')
		b.WriteString(strings.ReplaceAll(p, ":", ""))
	}
	return b.String()
}

func showAdminMessagingMenu(ctx *Ctx) {
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📢 Всем", admmsg("broadcast")),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👤 Пользователю", admmsg("pick")),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✖ Отмена", "ui:cancel"),
		),
	)
	m := tgbotapi.NewMessage(ctx.ChatID, "✉️ Админ-сообщения\n\nВыбери, что отправить:")
	m.ReplyMarkup = kb
	ctx.Bot.Send(m)
}

func startAdminBroadcastUI(ctx *Ctx) {
	setConv(ctx.ChatID, ctx.TgID, ConvState{Mode: ConvAdminBroadcastText})
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✖ Отмена", "ui:cancel"),
		),
	)
	m := tgbotapi.NewMessage(ctx.ChatID, "📢 Рассылка всем пользователям\n\nНапиши текст рассылки обычным сообщением.\n\n/cancel — отмена")
	m.ReplyMarkup = kb
	ctx.Bot.Send(m)
}

func showAdminBroadcastConfirm(ctx *Ctx, draft string) {
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Отправить", admmsg("broadcast_send")),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✏️ Изменить", admmsg("broadcast_edit")),
			tgbotapi.NewInlineKeyboardButtonData("✖ Отмена", "ui:cancel"),
		),
	)
	m := tgbotapi.NewMessage(ctx.ChatID, previewBox("📢 Предпросмотр рассылки:", draft))
	m.ReplyMarkup = kb
	ctx.Bot.Send(m)
}

func showAdminPickUser(ctx *Ctx, prefix string) {
	users, err := ctx.UsersStore.ListUsers()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка чтения users.json: "+err.Error())
		return
	}

	// sort by name
	sort.Slice(users, func(i, j int) bool { return strings.ToLower(users[i].Name) < strings.ToLower(users[j].Name) })

	filtered := make([]User, 0, len(users))
	pfx := strings.ToLower(strings.TrimSpace(prefix))
	for _, u := range users {
		if pfx == "" || strings.HasPrefix(strings.ToLower(u.Name), pfx) {
			filtered = append(filtered, u)
		}
	}

	rows := make([][]tgbotapi.InlineKeyboardButton, 0, 12)
	max := 10
	if len(filtered) < max {
		max = len(filtered)
	}
	for i := 0; i < max; i++ {
		u := filtered[i]
		title := u.Name
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👤 "+title, admmsg("to", fmt.Sprintf("%d", u.TelegramID))),
		))
	}
	rows = append(rows,
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔎 Поиск", admmsg("search")),
			tgbotapi.NewInlineKeyboardButtonData("✖ Отмена", "ui:cancel"),
		),
	)

	header := "Выбери пользователя, кому отправить сообщение:"
	if pfx != "" {
		header = fmt.Sprintf("Выбери пользователя (поиск: %s):", prefix)
	}

	m := tgbotapi.NewMessage(ctx.ChatID, header)
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	ctx.Bot.Send(m)
}

func startAdminPickUserUI(ctx *Ctx) {
	setConv(ctx.ChatID, ctx.TgID, ConvState{Mode: ConvAdminMsgSelectUser})
	showAdminPickUser(ctx, "")
}

func startAdminSearchUserUI(ctx *Ctx) {
	setConv(ctx.ChatID, ctx.TgID, ConvState{Mode: ConvAdminMsgSearchUser})
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✖ Отмена", "ui:cancel"),
		),
	)
	m := tgbotapi.NewMessage(ctx.ChatID, "🔎 Напиши часть имени пользователя (prefix), затем я покажу список.")
	m.ReplyMarkup = kb
	ctx.Bot.Send(m)
}

func startAdminUserMessageText(ctx *Ctx, targetID int64, targetName string) {
	setConv(ctx.ChatID, ctx.TgID, ConvState{Mode: ConvAdminMsgAwaitText, TargetID: targetID, TargetName: targetName})
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✖ Отмена", "ui:cancel"),
		),
	)
	m := tgbotapi.NewMessage(ctx.ChatID, fmt.Sprintf("✉️ Сообщение пользователю: %s\n\nНапиши текст обычным сообщением.", targetName))
	m.ReplyMarkup = kb
	ctx.Bot.Send(m)
}

func showAdminUserMessageConfirm(ctx *Ctx, targetName, draft string) {
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Отправить", admmsg("user_send")),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✏️ Изменить", admmsg("user_edit")),
			tgbotapi.NewInlineKeyboardButtonData("🔁 Сменить получателя", admmsg("pick")),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✖ Отмена", "ui:cancel"),
		),
	)
	m := tgbotapi.NewMessage(ctx.ChatID, previewBox(fmt.Sprintf("✉️ Предпросмотр (получатель: %s):", targetName), draft))
	m.ReplyMarkup = kb
	ctx.Bot.Send(m)
}

// handleAdminMessagingCallback processes inline callbacks for the admin messaging wizard.
func handleAdminMessagingCallback(ctx *Ctx, data string) {
	if !strings.HasPrefix(data, adminMsgCbPrefix) {
		return
	}
	payload := strings.TrimPrefix(data, adminMsgCbPrefix)
	parts := strings.Split(payload, ":")
	if len(parts) == 0 {
		return
	}
	action := parts[0]

	s, _ := getConv(ctx.ChatID, ctx.TgID)

	switch action {
	case "broadcast":
		startAdminBroadcastUI(ctx)
		return
	case "broadcast_edit":
		startAdminBroadcastUI(ctx)
		return
	case "broadcast_send":
		if s.Mode != ConvAdminBroadcastConfirm || strings.TrimSpace(s.Draft) == "" {
			reply(ctx.Bot, ctx.ChatID, "Нечего отправлять. Начни заново: /broadcast")
			clearConv(ctx.ChatID, ctx.TgID)
			return
		}
		sendBroadcast(ctx, s.Draft, nil)
		clearConv(ctx.ChatID, ctx.TgID)
		return

	case "pick":
		startAdminPickUserUI(ctx)
		return
	case "search":
		startAdminSearchUserUI(ctx)
		return
	case "to":
		if len(parts) < 2 {
			reply(ctx.Bot, ctx.ChatID, "Некорректный выбор.")
			return
		}
		idStr := parts[1]
		id := parseInt64(idStr)
		if id == 0 {
			reply(ctx.Bot, ctx.ChatID, "Некорректный выбор.")
			return
		}
		// Resolve name (best-effort)
		users, err := ctx.UsersStore.ListUsers()
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Ошибка чтения users.json: "+err.Error())
			return
		}
		name := fmt.Sprintf("%d", id)
		for _, u := range users {
			if u.TelegramID == id {
				name = u.Name
				break
			}
		}
		startAdminUserMessageText(ctx, id, name)
		return

	case "user_edit":
		// keep recipient; go back to text input
		if s.TargetID == 0 {
			reply(ctx.Bot, ctx.ChatID, "Сначала выбери получателя.")
			startAdminPickUserUI(ctx)
			return
		}
		startAdminUserMessageText(ctx, s.TargetID, s.TargetName)
		return
	case "user_send":
		if s.Mode != ConvAdminMsgConfirm || s.TargetID == 0 || strings.TrimSpace(s.Draft) == "" {
			reply(ctx.Bot, ctx.ChatID, "Нечего отправлять. Начни заново: /message")
			clearConv(ctx.ChatID, ctx.TgID)
			return
		}
		sendAdminMessageToUser(ctx, s.TargetID, s.TargetName, s.Draft, nil)
		clearConv(ctx.ChatID, ctx.TgID)
		return
	}
}

func parseInt64(s string) int64 {
	var n int64
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0
		}
		n = n*10 + int64(ch-'0')
	}
	return n
}
