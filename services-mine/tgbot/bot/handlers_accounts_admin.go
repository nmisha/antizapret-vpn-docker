package main

import (
	"strings"
)

func RegisterAdminAccountsHandlers(r *Router) {
	r.Handle("/acc_list", handleAccList, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."))
	r.Alias("acc_list", "/acc_list")

	r.Handle("/acc_add", handleAccAdd,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequireNonEmptyArg("Формат: /acc_add <name> <login> <password>"),
	)
	r.Alias("acc_add", "/acc_add")

	r.Handle("/acc_del", handleAccDel,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequireNonEmptyArg("Формат: /acc_del <name>"),
	)
	r.Alias("acc_del", "/acc_del")

	r.Handle("/acc_rename", handleAccRename,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequireNonEmptyArg("Формат: /acc_rename <old_name> <new_name>"),
	)
	r.Alias("acc_rename", "/acc_rename")

	r.Handle("/acc_set", handleAccSet,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequireNonEmptyArg("Формат: /acc_set <name> <login> <password>"),
	)
	r.Alias("acc_set", "/acc_set")
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
	msg, err := ctx.Accounts.DeleteByName(name)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
		return
	}
	reply(ctx.Bot, ctx.ChatID, msg)
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
