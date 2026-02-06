package main

import (
	"fmt"
	"html"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func RegisterSupportHandlers(r *Router) {
	r.Handle("/support", handleSupport)
	r.Alias("support", "/support")
}

func handleSupport(ctx *Ctx, arg string) {
	if !ctx.IsPrivate {
		reply(ctx.Bot, ctx.ChatID, "Пожалуйста, напиши в поддержку в личных сообщениях боту: /support <сообщение>")
		return
	}
	msg := strings.TrimSpace(arg)
	if msg == "" {
		reply(ctx.Bot, ctx.ChatID, "Напиши так: /support <сообщение>")
		return
	}

	users, err := ctx.UsersStore.ListUsers()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка чтения users.json: "+err.Error())
		return
	}

	sent := 0
	for _, u := range users {
		// Only explicit Support role (Admin should not auto-receive unless explicitly given Support).
		if !u.HasExact(RoleSupport) {
			continue
		}

		body := fmt.Sprintf(
			"🆘 <b>Support request</b>\nFrom: <b>%s</b> (tg_id=%d)\n\n%s",
			html.EscapeString(ctx.User.Name),
			ctx.TgID,
			html.EscapeString(msg),
		)
		m := tgbotapi.NewMessage(u.TelegramID, body)
		m.ParseMode = "HTML"
		m.DisableWebPagePreview = true
		if _, e := ctx.Bot.Send(m); e == nil {
			sent++
		}
	}

	if sent == 0 {
		reply(ctx.Bot, ctx.ChatID, "Сообщение принято, но сейчас нет пользователей с ролью Support.")
		return
	}
	reply(ctx.Bot, ctx.ChatID, fmt.Sprintf("Сообщение отправлено в поддержку (%d получ.).", sent))
}
