package bot

import (
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func RegisterAdminHandlers(reg *CommandRegistry) {
	// help
	reg.Command(CommandSpec{Cmd: "/start", Desc: "показать справку", Section: "Быстрый старт"}, handleHelp)
	reg.Command(CommandSpec{Cmd: "/help", Desc: "показать эту справку", Section: "Быстрый старт"}, handleHelp)
	reg.Alias("help", "/help")

	// users/admin
	reg.Command(CommandSpec{Cmd: "/users", Desc: "список пользователей и ролей", Section: "Админ: пользователи и роли", NeedAny: []Role{RoleAdmin}}, handleUsers, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."))
	reg.Alias("users", "/users")

	// roles
	reg.Command(CommandSpec{Cmd: "/roles", Args: "[name]", Desc: "показать роли (свои; Admin может смотреть чужие)", Section: "Быстрый старт"}, handleRoles)
	reg.Alias("roles", "/roles")

	// roles catalog (admin)
	reg.Command(CommandSpec{Cmd: "/roles_catalog", Desc: "все роли с описанием", Section: "Админ: пользователи и роли", NeedAny: []Role{RoleAdmin}}, handleRolesCatalog, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."))
	reg.Alias("roles_catalog", "/roles_catalog")

	// grant/revoke/rename — admin only + обязательные аргументы (где нужно)
	reg.Command(CommandSpec{Cmd: "/grant", Args: "<name> <role>", Desc: "выдать роль пользователю", Section: "Админ: пользователи и роли", NeedAny: []Role{RoleAdmin}}, handleGrant,

		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequireNonEmptyArg("Формат: /grant <name> <role>"),
	)
	reg.Alias("grant", "/grant")

	reg.Command(CommandSpec{Cmd: "/revoke", Args: "<name> <role>", Desc: "снять роль с пользователя", Section: "Админ: пользователи и роли", NeedAny: []Role{RoleAdmin}}, handleRevoke,

		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequireNonEmptyArg("Формат: /revoke <name> <role>"),
	)
	reg.Alias("revoke", "/revoke")

	reg.Handle("/rename", handleRename,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequireNonEmptyArg("Формат: /rename <old_name> <new_name>"),
	)
	reg.Alias("rename", "/rename")

	// admin messaging
	reg.Handle("/adminmsg", handleAdminMsgMenu,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
	)
	reg.Alias("adminmsg", "/adminmsg")

	reg.Command(CommandSpec{Cmd: "/broadcast", Args: "<text>", Desc: "отправить всем (без аргумента откроет UI)", Section: "Админ: сообщения", NeedAny: []Role{RoleAdmin}}, handleBroadcast,

		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
	)
	reg.Alias("broadcast", "/broadcast")

	reg.Handle("/message", handleMessageUser,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
	)
	reg.Alias("msg", "/message")
}

func handleAdminMsgMenu(ctx *Ctx, _ string) {
	showAdminMessagingMenu(ctx)
}

func handleHelp(ctx *Ctx, _ string) {
	replyHTML(ctx.Bot, ctx.ChatID, helpForUser(ctx.User))
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

func handleBroadcast(ctx *Ctx, arg string) {
	text := strings.TrimSpace(arg)
	if text == "" {
		// Soft UI
		startAdminBroadcastUI(ctx)
		return
	}

	sendBroadcast(ctx, text)
}

func handleMessageUser(ctx *Ctx, arg string) {
	fields := strings.Fields(arg)
	if len(fields) == 0 {
		// Soft UI
		startAdminPickUserUI(ctx)
		return
	}
	if len(fields) < 2 {
		reply(ctx.Bot, ctx.ChatID, "Формат: /message <name> <сообщение> (или просто /message для UI)")
		return
	}
	target := fields[0]
	text := strings.TrimSpace(arg[len(target):])
	text = strings.TrimSpace(text)
	if text == "" {
		reply(ctx.Bot, ctx.ChatID, "Формат: /message <name> <сообщение>")
		return
	}

	u, ok, err := ctx.UsersStore.GetByName(target)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка чтения users.json: "+err.Error())
		return
	}
	if !ok {
		reply(ctx.Bot, ctx.ChatID, "Пользователь не найден: "+target)
		return
	}

	sendAdminMessageToUser(ctx, u.TelegramID, u.Name, text)
}

func sendBroadcast(ctx *Ctx, text string) {
	users, err := ctx.UsersStore.ListUsers()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка чтения users.json: "+err.Error())
		return
	}

	sent, failed := 0, 0
	failLines := make([]string, 0, 8)
	for _, u := range users {
		m := tgbotapi.NewMessage(u.TelegramID, "📢 Сообщение от администратора:\n\n"+text)
		m.DisableWebPagePreview = true
		if _, e := ctx.Bot.Send(m); e == nil {
			sent++
		} else {
			failed++
			// collect details (cap to keep message size reasonable)
			if len(failLines) < 30 {
				failLines = append(failLines, fmt.Sprintf("• %s (%d): %v", u.Name, u.TelegramID, e))
			}
		}
	}

	msg := fmt.Sprintf("Готово. Отправлено: %d. Ошибок: %d.", sent, failed)
	reply(ctx.Bot, ctx.ChatID, msg)
	if failed > 0 {
		details := "Не доставлено:\n" + strings.Join(failLines, "\n")
		if failed > len(failLines) {
			details += fmt.Sprintf("\n… и ещё %d", failed-len(failLines))
		}
		sendTextChunks(ctx.Bot, ctx.ChatID, details)
	}
}

func sendAdminMessageToUser(ctx *Ctx, targetID int64, targetName, text string) {
	m := tgbotapi.NewMessage(targetID, "✉️ Сообщение от администратора:\n\n"+text)
	m.DisableWebPagePreview = true
	if _, e := ctx.Bot.Send(m); e != nil {
		reply(ctx.Bot, ctx.ChatID, fmt.Sprintf("Не удалось отправить %s (%d): %v", targetName, targetID, e))
		return
	}
	reply(ctx.Bot, ctx.ChatID, "Отправлено пользователю: "+targetName)
}
