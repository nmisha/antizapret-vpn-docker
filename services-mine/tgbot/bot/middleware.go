package main

import (
	"strings"
)

// RequireRole — требует конкретную роль. Admin проходит всегда через u.Has().
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

// RequireAnyRole — требует любую из ролей. Admin проходит всегда через u.Has().
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

// RequireNonEmptyArg — требует непустой аргумент.
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
