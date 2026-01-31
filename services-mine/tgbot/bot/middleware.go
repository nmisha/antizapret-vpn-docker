package main

import (
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

//type Middleware func(next HandlerFunc) HandlerFunc

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
