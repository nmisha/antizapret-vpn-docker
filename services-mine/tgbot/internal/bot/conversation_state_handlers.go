package bot

import (
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// handleConversationStateIfAny intercepts plain text messages when a wizard is active.
// Returns true if the message was consumed.
func handleConversationStateIfAny(ctx *Ctx, fullText, cmd, arg string) bool {
	st, ok := getConv(ctx.ChatID, ctx.TgID)
	if !ok || st.Mode == ConvNone {
		return false
	}

	if strings.EqualFold(cmd, "/cancel") {
		clearConv(ctx.ChatID, ctx.TgID)
		reply(ctx.Bot, ctx.ChatID, "Ок, отменил.")
		return true
	}

	switch st.Mode {
	case ConvNetDNSAwaitDomain:
		if strings.HasPrefix(strings.TrimSpace(fullText), "/") {
			reply(ctx.Bot, ctx.ChatID, "Введи домен обычным сообщением или /cancel.")
			return true
		}
		domain := strings.TrimSpace(fullText)
		clearConv(ctx.ChatID, ctx.TgID)
		netDNSResolveAndReply(ctx, domain)
		return true

	case ConvSupportAwaitText:
		if strings.HasPrefix(strings.TrimSpace(fullText), "/") {
			reply(ctx.Bot, ctx.ChatID, "Напиши текст обращения обычным сообщением или /cancel.")
			return true
		}
		forwardSupport(ctx, strings.TrimSpace(fullText))
		clearConv(ctx.ChatID, ctx.TgID)
		return true

	case ConvAdminBroadcastText:
		if !ctx.User.Has(RoleAdmin) {
			clearConv(ctx.ChatID, ctx.TgID)
			reply(ctx.Bot, ctx.ChatID, "Недостаточно прав. Нужна роль Admin.")
			return true
		}
		if strings.HasPrefix(strings.TrimSpace(fullText), "/") {
			reply(ctx.Bot, ctx.ChatID, "Напиши текст рассылки обычным сообщением или /cancel.")
			return true
		}
		sendBroadcast(ctx, strings.TrimSpace(fullText), ctx.MessageEntities)
		clearConv(ctx.ChatID, ctx.TgID)
		return true

	case ConvAdminMsgAwaitText:
		if !ctx.User.Has(RoleAdmin) {
			clearConv(ctx.ChatID, ctx.TgID)
			reply(ctx.Bot, ctx.ChatID, "Недостаточно прав. Нужна роль Admin.")
			return true
		}
		if strings.HasPrefix(strings.TrimSpace(fullText), "/") {
			reply(ctx.Bot, ctx.ChatID, "Напиши текст сообщения обычным сообщением или /cancel.")
			return true
		}
		sendAdminMessageToUser(ctx, st.TargetID, st.TargetName, strings.TrimSpace(fullText), ctx.MessageEntities)
		clearConv(ctx.ChatID, ctx.TgID)
		return true

	case ConvAdminMsgSearchUser:
		if !ctx.User.Has(RoleAdmin) {
			clearConv(ctx.ChatID, ctx.TgID)
			reply(ctx.Bot, ctx.ChatID, "Недостаточно прав. Нужна роль Admin.")
			return true
		}
		if strings.HasPrefix(strings.TrimSpace(fullText), "/") {
			reply(ctx.Bot, ctx.ChatID, "Напиши часть имени пользователя обычным сообщением или /cancel.")
			return true
		}
		prefix := strings.TrimSpace(fullText)
		st.SearchPrefix = prefix
		st.Mode = ConvAdminMsgSelectUser
		st.Draft = ""
		setConv(ctx.ChatID, ctx.TgID, st)
		showAdminPickUser(ctx, prefix)
		return true
	}

	clearConv(ctx.ChatID, ctx.TgID)
	reply(ctx.Bot, ctx.ChatID, "Сбросил состояние диалога. Попробуй ещё раз.")
	return true
}

func supportStartUI(ctx *Ctx) {
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Отмена", "ui:cancel"),
		),
	)
	msg := tgbotapi.NewMessage(ctx.ChatID, "Напиши обращение в поддержку обычным сообщением.\n\nМожно из группы, я передам, откуда оно пришло.\n\n/cancel - отмена")
	msg.ReplyMarkup = kb
	_, err := ctx.Bot.Send(msg)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", msg.Text)
}

// forwardSupport keeps the existing logic of sending to all Support-role users.
func forwardSupport(ctx *Ctx, text string) {
	_ = text
	sendSupportMessage(ctx, text, ctx.MessageEntities)
}

func previewBox(title, body string) string {
	if body == "" {
		body = "(пусто)"
	}
	return fmt.Sprintf("%s\n\n%s", title, body)
}
