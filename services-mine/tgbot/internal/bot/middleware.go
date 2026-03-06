package bot

import (
	"fmt"
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func RequireRole(role Role, msg string) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx *Ctx, arg string) {
			if ctx.User.Has(role) {
				next(ctx, arg)
				return
			}
			if msg == "" {
				msg = "Недостаточно прав."
			}
			reply(ctx.Bot, ctx.ChatID, msg)
		}
	}
}

func RequireAnyRole(msg string, roles ...Role) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx *Ctx, arg string) {
			for _, r := range roles {
				if ctx.User.Has(r) {
					next(ctx, arg)
					return
				}
			}
			if msg == "" {
				msg = "Недостаточно прав."
			}
			reply(ctx.Bot, ctx.ChatID, msg)
		}
	}
}

func RequireNonEmptyArg(msg string) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx *Ctx, arg string) {
			if strings.TrimSpace(arg) == "" {
				if msg == "" {
					msg = "Не задан аргумент."
				}
				reply(ctx.Bot, ctx.ChatID, msg)
				return
			}
			next(ctx, arg)
		}
	}
}

func RequireFields(n int, msg string) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx *Ctx, arg string) {
			fields := strings.Fields(arg)
			if len(fields) != n {
				if msg == "" {
					msg = "Неверный формат команды."
				}
				reply(ctx.Bot, ctx.ChatID, msg)
				return
			}
			ctx.Fields = fields
			next(ctx, arg)
		}
	}
}

// RequirePrivate ограничивает выполнение команды личным чатом с ботом.
// Полезно для UI/операций с чувствительными данными (например, пароли).
func RequirePrivate(msg string) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx *Ctx, arg string) {
			if ctx.IsPrivate {
				next(ctx, arg)
				return
			}
			if msg == "" {
				msg = "Эта команда доступна только в личных сообщениях с ботом."
			}
			reply(ctx.Bot, ctx.ChatID, msg)
		}
	}
}

// RequirePrivateWithOpenDM limits command usage to private chat and suggests opening DM.
func RequirePrivateWithOpenDM(msg string) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx *Ctx, arg string) {
			if ctx.IsPrivate {
				next(ctx, arg)
				return
			}
			if msg == "" {
				msg = "Эта команда доступна только в личных сообщениях с ботом."
			}
			replyWithOpenDMButton(ctx, msg)
		}
	}
}

func replyWithOpenDMButton(ctx *Ctx, intro string) {
	username := ctx.Bot.Self.UserName
	if username == "" {
		reply(ctx.Bot, ctx.ChatID, intro)
		return
	}

	url := fmt.Sprintf("https://t.me/%s", username)
	kb := tgbotapi.NewInlineKeyboardMarkup(
		[]tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonURL("Открыть личку", url),
		},
		[]tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("Отмена", "ui:cancel"),
		},
	)
	m := tgbotapi.NewMessage(ctx.ChatID, intro+"\n\nНажми кнопку ниже:")
	m.ReplyMarkup = kb
	_, err := ctx.Bot.Send(m)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", m.Text)
}

func WithTyping() Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx *Ctx, arg string) {
			_, _ = ctx.Bot.Request(tgbotapi.NewChatAction(ctx.ChatID, tgbotapi.ChatTyping))
			next(ctx, arg)
		}
	}
}

func LogCommand() Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx *Ctx, arg string) {
			start := time.Now()
			next(ctx, arg)
			d := time.Since(start)
			log.Printf("cmd=%s user=%s tg_id=%d dur=%s arg=%q",
				ctx.Cmd, ctx.User.Name, ctx.User.TelegramID, d, strings.TrimSpace(arg))
		}
	}
}
