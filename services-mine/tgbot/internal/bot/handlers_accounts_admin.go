package bot

import (
	"strings"
)

func RegisterAdminAccountsHandlers(reg *CommandRegistry) {
	reg.Command(CommandSpec{Cmd: "/acc_list", Args: "", Desc: "список учётных записей", Section: "Админ: аккаунты", NeedAny: []Role{RoleAdmin}}, handleAccList, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."))
	reg.Alias("acc_list", "/acc_list")

	reg.Command(CommandSpec{Cmd: "/acc_add", Args: "<name> <login> <password>", Desc: "добавить/обновить учётную запись", Section: "Админ: аккаунты", NeedAny: []Role{RoleAdmin}}, handleAccAdd,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequireNonEmptyArg("Формат: /acc_add <name> <login> <password>"),
	)
	reg.Alias("acc_add", "/acc_add")

	reg.Command(CommandSpec{Cmd: "/acc_del", Args: "<name>", Desc: "удалить учётную запись", Section: "Админ: аккаунты", NeedAny: []Role{RoleAdmin}}, handleAccDel,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequireNonEmptyArg("Формат: /acc_del <name>"),
	)
	reg.Alias("acc_del", "/acc_del")

	reg.Command(CommandSpec{Cmd: "/acc_rename", Args: "<old_name> <new_name>", Desc: "переименовать учётную запись", Section: "Админ: аккаунты", NeedAny: []Role{RoleAdmin}}, handleAccRename,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequireNonEmptyArg("Формат: /acc_rename <old_name> <new_name>"),
	)
	reg.Alias("acc_rename", "/acc_rename")

	reg.Command(CommandSpec{Cmd: "/acc_set", Args: "<name> <login> <password>", Desc: "обновить логин/пароль учётной записи", Section: "Админ: аккаунты", NeedAny: []Role{RoleAdmin}}, handleAccSet,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequireNonEmptyArg("Формат: /acc_set <name> <login> <password>"),
	)
	reg.Alias("acc_set", "/acc_set")
}

func handleAccList(ctx *Ctx, _ string) {
	list, err := ctx.Accounts.List()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка чтения accounts.json: "+err.Error())
		return
	}
	if len(list) == 0 {
		reply(ctx.Bot, ctx.ChatID, "accounts.json пуст.")
		return
	}
	var b strings.Builder
	b.WriteString("Учётные записи:\n")
	for _, a := range list {
		b.WriteString("• ")
		b.WriteString(a.Name)
		if a.Login != "" {
			b.WriteString(" (")
			b.WriteString(a.Login)
			b.WriteString(")")
		}
		b.WriteString("\n")
	}
	reply(ctx.Bot, ctx.ChatID, b.String())
}

func handleAccAdd(ctx *Ctx, arg string) {
	fields := strings.Fields(arg)
	if len(fields) < 3 {
		reply(ctx.Bot, ctx.ChatID, "Формат: /acc_add <name> <login> <password>")
		return
	}
	name := fields[0]
	login := fields[1]
	pass := strings.Join(fields[2:], " ")
	msg, err := ctx.Accounts.Upsert(Account{Name: name, Login: login, Password: pass})
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
		return
	}
	reply(ctx.Bot, ctx.ChatID, msg)
}

func handleAccSet(ctx *Ctx, arg string) {
	// same as add, but message differs in store
	handleAccAdd(ctx, arg)
}

func handleAccDel(ctx *Ctx, arg string) {
	name := strings.TrimSpace(arg)
	if name == "" {
		reply(ctx.Bot, ctx.ChatID, "Формат: /acc_del <name>")
		return
	}
	// Confirmation for any destructive action.
	// Actual deletion is performed in handleConfirmCallback.
	sendConfirm(ctx, "Удалить учётную запись \""+name+"\"?", "acc:del", name)
}

func handleAccRename(ctx *Ctx, arg string) {
	fields := strings.Fields(arg)
	if len(fields) != 2 {
		reply(ctx.Bot, ctx.ChatID, "Формат: /acc_rename <old_name> <new_name>")
		return
	}
	msg, err := ctx.Accounts.Rename(fields[0], fields[1])
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
		return
	}
	reply(ctx.Bot, ctx.ChatID, msg)
}
