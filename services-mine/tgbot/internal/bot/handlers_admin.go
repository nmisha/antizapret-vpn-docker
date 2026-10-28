package bot

import (
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func RegisterAdminHandlers(reg *CommandRegistry) {
	// help
	reg.Command(CommandSpec{Cmd: "/start", Desc: "показать справку", Section: secQuickStart}, handleStart)
	reg.Command(CommandSpec{Cmd: "/help", Args: "[раздел|all]", Desc: "показать эту справку", Section: secQuickStart}, handleHelp)
	reg.Alias("help", "/help")

	// users/admin
	reg.Command(CommandSpec{Cmd: "/users", Desc: "список пользователей и ролей", Section: secAdminUsers, NeedAny: []Role{RoleAdmin}}, handleUsers,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequirePrivateWithOpenDM("Команда /users доступна только в личных сообщениях с ботом."),
	)
	reg.Alias("users", "/users")

	// user management
	reg.Command(CommandSpec{Cmd: "/user_add", Args: "<name> <tg_id>", Desc: "добавить пользователя", Section: secAdminUsers, NeedAny: []Role{RoleAdmin}}, handleUserAdd,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequirePrivateWithOpenDM("Команда /user_add доступна только в личных сообщениях с ботом."),
		RequireNonEmptyArg("Формат: /user_add <name> <tg_id>"),
	)
	reg.Alias("user_add", "/user_add")
	reg.Command(CommandSpec{Cmd: "/user_del", Args: "<name>", Desc: "удалить пользователя", Section: secAdminUsers, NeedAny: []Role{RoleAdmin}}, handleUserDel,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequirePrivateWithOpenDM("Команда /user_del доступна только в личных сообщениях с ботом."),
		RequireNonEmptyArg("Формат: /user_del <name>"),
	)
	reg.Alias("user_del", "/user_del")

	// roles
	reg.Command(CommandSpec{Cmd: "/roles", Args: "[name]", Desc: "показать роли (свои; Admin может смотреть чужие)", Section: secQuickStart}, handleRoles)
	reg.Alias("roles", "/roles")

	// roles catalog (admin)
	reg.Command(CommandSpec{Cmd: "/roles_catalog", Desc: "все роли с описанием", Section: secAdminUsers, NeedAny: []Role{RoleAdmin}}, handleRolesCatalog,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequirePrivateWithOpenDM("Команда /roles_catalog доступна только в личных сообщениях с ботом."),
	)
	reg.Alias("roles_catalog", "/roles_catalog")

	// grant/revoke/rename — admin only + обязательные аргументы (где нужно)
	reg.Command(CommandSpec{Cmd: "/grant", Args: "<name> <role>", Desc: "выдать роль пользователю", Section: secAdminUsers, NeedAny: []Role{RoleAdmin}}, handleGrant,

		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequirePrivateWithOpenDM("Команда /grant доступна только в личных сообщениях с ботом."),
		RequireNonEmptyArg("Формат: /grant <name> <role>"),
	)
	reg.Alias("grant", "/grant")

	reg.Command(CommandSpec{Cmd: "/revoke", Args: "<name> <role>", Desc: "снять роль с пользователя", Section: secAdminUsers, NeedAny: []Role{RoleAdmin}}, handleRevoke,

		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequirePrivateWithOpenDM("Команда /revoke доступна только в личных сообщениях с ботом."),
		RequireNonEmptyArg("Формат: /revoke <name> <role>"),
	)
	reg.Alias("revoke", "/revoke")

	reg.Command(CommandSpec{Cmd: "/rename", Args: "<old_name> <new_name>", Desc: "переименовать пользователя", Section: secAdminUsers, NeedAny: []Role{RoleAdmin}}, handleRename,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequirePrivateWithOpenDM("Команда /rename доступна только в личных сообщениях с ботом."),
		RequireNonEmptyArg("Формат: /rename <old_name> <new_name>"),
	)
	reg.Alias("rename", "/rename")

	// admin messaging
	reg.Command(CommandSpec{Cmd: "/adminmsg", Desc: "UI: написать пользователю", Section: secAdminMessages, NeedAny: []Role{RoleAdmin}}, handleAdminMsgMenu,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
	)
	reg.Alias("adminmsg", "/adminmsg")

	reg.Command(CommandSpec{Cmd: "/broadcast", Args: "<text>", Desc: "отправить всем (без аргумента откроет UI)", Section: secAdminMessages, NeedAny: []Role{RoleAdmin}}, handleBroadcast,

		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
	)
	reg.Alias("broadcast", "/broadcast")

	reg.Command(CommandSpec{Cmd: "/message", Args: "<name> <text>", Desc: "написать пользователю (без UI)", Section: secAdminMessages, NeedAny: []Role{RoleAdmin}}, handleMessageUser,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
	)
	reg.Alias("msg", "/message")
}

func handleAdminMsgMenu(ctx *Ctx, _ string) {
	showAdminMessagingMenu(ctx)
}

func handleHelp(ctx *Ctx, arg string) {
	showHelp(ctx, arg)
}

// handleStart ignores the /start payload (Telegram deep links) and shows the help index.
func handleStart(ctx *Ctx, _ string) {
	showHelp(ctx, "")
}

func handleUsers(ctx *Ctx, _ string) {
	list, err := ctx.UsersStore.ListUsers()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка чтения users.json: "+err.Error())
		return
	}
	reply(ctx.Bot, ctx.ChatID, formatUsers(list))
}

func handleUserAdd(ctx *Ctx, arg string) {
	fields := strings.Fields(arg)
	if len(fields) != 2 {
		reply(ctx.Bot, ctx.ChatID, "Формат: /user_add <name> <tg_id>")
		return
	}
	name := fields[0]
	tgID, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || tgID <= 0 {
		reply(ctx.Bot, ctx.ChatID, "tg_id должен быть числом. Формат: /user_add <name> <tg_id>")
		return
	}
	msg, err := ctx.UsersStore.Add(name, tgID)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
		return
	}
	reply(ctx.Bot, ctx.ChatID, msg)
}

func handleUserDel(ctx *Ctx, arg string) {
	name := strings.TrimSpace(arg)
	if name == "" {
		reply(ctx.Bot, ctx.ChatID, "Формат: /user_del <name>")
		return
	}
	u, found, _ := ctx.UsersStore.GetByName(name)
	msg, err := ctx.UsersStore.DeleteByName(name)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
		return
	}
	if found {
		clearUserUIState(u.TelegramID)
	}
	reply(ctx.Bot, ctx.ChatID, msg)
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

	msg, err := renameUserWithSection(ctx, oldName, newName)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
		return
	}
	reply(ctx.Bot, ctx.ChatID, msg)
}

func handleBroadcast(ctx *Ctx, arg string) {
	text := arg
	if text == "" {
		// Soft UI
		startAdminBroadcastUI(ctx)
		return
	}

	sendBroadcast(ctx, text, ctx.ArgEntities)
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
	text, textEntities := parseRemainderAfterFirstToken(arg, ctx.ArgEntities)
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

	sendAdminMessageToUser(ctx, u.TelegramID, u.Name, text, textEntities)
}

func sendBroadcast(ctx *Ctx, text string, entities []tgbotapi.MessageEntity) {
	users, err := ctx.UsersStore.ListUsers()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка чтения users.json: "+err.Error())
		return
	}

	sent, failed, skipped := 0, 0, 0
	failLines := make([]string, 0, 8)
	for _, u := range users {
		if u.Disabled {
			skipped++
			continue
		}
		body, bodyEntities := prependAndShiftEntities("📢 Сообщение от администратора:\n\n", text, entities)
		m := tgbotapi.NewMessage(u.TelegramID, body)
		m.Entities = bodyEntities
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
	if skipped > 0 {
		msg += fmt.Sprintf(" Пропущено (отключены): %d.", skipped)
	}
	reply(ctx.Bot, ctx.ChatID, msg)
	if failed > 0 {
		details := "Не доставлено:\n" + strings.Join(failLines, "\n")
		if failed > len(failLines) {
			details += fmt.Sprintf("\n… и ещё %d", failed-len(failLines))
		}
		sendTextChunks(ctx.Bot, ctx.ChatID, details)
	}
}

func sendAdminMessageToUser(ctx *Ctx, targetID int64, targetName, text string, entities []tgbotapi.MessageEntity) {
	// The target may have been deleted or disabled while the admin was typing.
	u, ok, err := ctx.UsersStore.GetByID(targetID)
	if err != nil {
		replyServiceError(ctx, "бот", "Не удалось проверить получателя, сообщение не отправлено", err)
		return
	}
	if !ok {
		reply(ctx.Bot, ctx.ChatID, "Пользователь "+targetName+" удалён, сообщение не отправлено.")
		return
	}
	if u.Disabled {
		reply(ctx.Bot, ctx.ChatID, "Пользователь "+targetName+" отключён, сообщение не отправлено. Включить: /users_admin")
		return
	}
	body, bodyEntities := prependAndShiftEntities("✉️ Сообщение от администратора:\n\n", text, entities)
	m := tgbotapi.NewMessage(targetID, body)
	m.Entities = bodyEntities
	m.DisableWebPagePreview = true
	if _, e := ctx.Bot.Send(m); e != nil {
		reply(ctx.Bot, ctx.ChatID, fmt.Sprintf("Не удалось отправить %s (%d): %v", targetName, targetID, e))
		return
	}
	reply(ctx.Bot, ctx.ChatID, "Отправлено пользователю: "+targetName)
}

func parseRemainderAfterFirstToken(arg string, entities []tgbotapi.MessageEntity) (string, []tgbotapi.MessageEntity) {
	if arg == "" {
		return "", nil
	}
	i := strings.IndexAny(arg, " \t\r\n")
	if i < 0 {
		return "", nil
	}
	j := i
	for j < len(arg) {
		switch arg[j] {
		case ' ', '\t', '\r', '\n':
			j++
		default:
			text := arg[j:]
			startUTF16 := utf16Len(arg[:j])
			return text, sliceEntitiesForSuffix(entities, startUTF16)
		}
	}
	return "", nil
}
