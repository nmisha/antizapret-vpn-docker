package main

import (
	"fmt"
	"strings"
)

type helpCmd struct {
	Cmd     string
	Args    string
	Desc    string
	NeedAny []Role
}

func helpForUser(u User) string {
	cmds := []helpCmd{
		{Cmd: "/help", Desc: "помощь"},
		{Cmd: "/roles", Args: "[name]", Desc: "показать роли (свои; Admin может смотреть чужие)"},
		{Cmd: "/support", Args: "<сообщение>", Desc: "написать в поддержку (в личке)"},

		{Cmd: "/add", Args: "<domain>", Desc: "добавить домен в твою секцию", NeedAny: []Role{RoleDomainEditor}},
		{Cmd: "/del", Args: "<domain>", Desc: "удалить домен из твоей секции", NeedAny: []Role{RoleDomainEditor}},
		{Cmd: "/list", Desc: "показать домены твоей секции", NeedAny: []Role{RoleDomainEditor}},
		{Cmd: "/export", Desc: "экспорт всего списка", NeedAny: []Role{RoleDomainManager}},

		{Cmd: "/wgstats", Args: "[user]", Desc: "статистика WireGuard (Admin может указать пользователя)", NeedAny: []Role{RoleWgStats}},
		{Cmd: "/wgstats_admin", Desc: "статистика WireGuard без имени пользователя", NeedAny: []Role{RoleAdmin}},
		{Cmd: "/agh_update_lists", Desc: "обновить списки AdGuard", NeedAny: []Role{RoleServiceManager}},

		{Cmd: "/users", Desc: "список пользователей", NeedAny: []Role{RoleAdmin}},
		{Cmd: "/grant", Args: "<name> <role>", Desc: "выдать роль", NeedAny: []Role{RoleAdmin}},
		{Cmd: "/revoke", Args: "<name> <role>", Desc: "снять роль", NeedAny: []Role{RoleAdmin}},
		{Cmd: "/rename", Args: "<old> <new>", Desc: "переименовать пользователя и секцию доменов", NeedAny: []Role{RoleAdmin}},

		{Cmd: "/accounts", Desc: "получить учётную запись к сервису (только в личке)", NeedAny: []Role{RoleAiUser}},
	}

	type block struct {
		Title string
		Items []helpCmd
	}
	blocks := []block{
		{Title: "Общее", Items: []helpCmd{cmds[0], cmds[1], cmds[2]}},
		{Title: "Домены", Items: []helpCmd{cmds[3], cmds[4], cmds[5], cmds[6]}},
		{Title: "AI", Items: []helpCmd{cmds[14]}},
		{Title: "Сервисы", Items: []helpCmd{cmds[7], cmds[8], cmds[9]}},
		{Title: "Админка", Items: []helpCmd{cmds[10], cmds[11], cmds[12], cmds[13]}},
	}

	var b strings.Builder
	b.WriteString("Доступные команды:\n")

	printedAnyBlock := false
	for _, bl := range blocks {
		lines := make([]string, 0, len(bl.Items))
		for _, c := range bl.Items {
			if isCmdAllowed(u, c) {
				cmdline := c.Cmd
				if strings.TrimSpace(c.Args) != "" {
					cmdline += " " + c.Args
				}
				lines = append(lines, fmt.Sprintf("• %-22s — %s", cmdline, c.Desc))
			}
		}
		if len(lines) == 0 {
			continue
		}
		if printedAnyBlock {
			b.WriteString("\n")
		}
		printedAnyBlock = true
		b.WriteString(bl.Title + ":\n")
		for _, ln := range lines {
			b.WriteString(ln)
			b.WriteByte('\n')
		}
	}

	return strings.TrimSpace(b.String())
}

func isCmdAllowed(u User, c helpCmd) bool {
	if len(c.NeedAny) == 0 {
		return true
	}
	for _, r := range c.NeedAny {
		if u.Has(r) {
			return true
		}
	}
	return false
}
