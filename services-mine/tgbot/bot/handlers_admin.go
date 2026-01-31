package main

import (
	"fmt"
	"strings"
)

func RegisterAdminHandlers(r *Router) {
	// help
	r.Handle("/start", handleHelp)
	r.Handle("/help", handleHelp)
	r.Alias("help", "/help")

	// users/admin
	r.Handle("/users", handleUsers, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."))
	r.Alias("users", "/users")

	// roles
	r.Handle("/roles", handleRoles)
	r.Alias("roles", "/roles")

	// grant/revoke/rename — admin only + обязательные аргументы (где нужно)
	r.Handle("/grant", handleGrant,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequireNonEmptyArg("Формат: /grant <name> <role>"),
	)
	r.Alias("grant", "/grant")

	r.Handle("/revoke", handleRevoke,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequireNonEmptyArg("Формат: /revoke <name> <role>"),
	)
	r.Alias("revoke", "/revoke")

	r.Handle("/rename", handleRename,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequireNonEmptyArg("Формат: /rename <old_name> <new_name>"),
	)
	r.Alias("rename", "/rename")
}

func handleHelp(ctx *Ctx, _ string) {
	reply(ctx.Bot, ctx.ChatID, helpForUser(ctx.User))
}

func handleUsers(ctx *Ctx, _ string) {
	list, err := ctx.UsersStore.ListUsers()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка чтения users.json: "+err.Error())
		return
	}
	reply(ctx.Bot, ctx.ChatID, formatUsers(list))
}

func handleRoles(ctx *Ctx, arg string) {
	targetName := strings.TrimSpace(arg)
	if targetName == "" {
		reply(ctx.Bot, ctx.ChatID, fmt.Sprintf("Пользователь %s (tg_id=%d)\nРоли: %s",
			ctx.User.Name, ctx.User.TelegramID, strings.Join(uniqueStringsCaseInsensitive(ctx.User.RolesRaw), ", "),
		))
		return
	}
	if !ctx.User.Has(RoleAdmin) {
		reply(ctx.Bot, ctx.ChatID, "Недостаточно прав. Смотреть роли других может только Admin.")
		return
	}
	tu, ok, err := ctx.UsersStore.GetByName(targetName)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка чтения users.json: "+err.Error())
		return
	}
	if !ok {
		reply(ctx.Bot, ctx.ChatID, "Пользователь не найден: "+targetName)
		return
	}
	reply(ctx.Bot, ctx.ChatID, fmt.Sprintf("Пользователь %s (tg_id=%d)\nРоли: %s",
		tu.Name, tu.TelegramID, strings.Join(uniqueStringsCaseInsensitive(tu.RolesRaw), ", "),
	))
}

func handleGrant(ctx *Ctx, arg string) {
	fields := strings.Fields(arg)
	if len(fields) != 2 {
		reply(ctx.Bot, ctx.ChatID, "Формат: /grant <name> <role>")
		return
	}
	name := fields[0]
	role := Role(fields[1])

	msg, err := ctx.UsersStore.GrantByName(name, role)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
		return
	}
	reply(ctx.Bot, ctx.ChatID, msg)
}

func handleRevoke(ctx *Ctx, arg string) {
	fields := strings.Fields(arg)
	if len(fields) != 2 {
		reply(ctx.Bot, ctx.ChatID, "Формат: /revoke <name> <role>")
		return
	}
	name := fields[0]
	role := Role(fields[1])

	msg, err := ctx.UsersStore.RevokeByName(name, role)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
		return
	}
	reply(ctx.Bot, ctx.ChatID, msg)
}

func handleRename(ctx *Ctx, arg string) {
	fields := strings.Fields(arg)
	if len(fields) != 2 {
		reply(ctx.Bot, ctx.ChatID, "Формат: /rename <old_name> <new_name>")
		return
	}
	oldName := fields[0]
	newName := fields[1]

	msg1, err := ctx.UsersStore.Rename(oldName, newName)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
		return
	}

	msg2, err := ctx.Domains.RenameSection(oldName, newName)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, msg1+"\n\nОшибка при переименовании секции доменов: "+err.Error())
		return
	}

	reply(ctx.Bot, ctx.ChatID, msg1+"\n"+msg2)
}
