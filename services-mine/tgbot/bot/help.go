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
	blocks := []struct {
		Title string
		Items []helpCmd
	}{
		{
			Title: "Общее",
			Items: []helpCmd{
				{Cmd: "/help", Desc: "помощь"},
				{Cmd: "/roles", Args: "[name]", Desc: "показать роли (свои; Admin может смотреть чужие)"},
				{Cmd: "/support", Args: "[сообщение]", Desc: "написать в поддержку (можно из группы; без аргумента откроет UI)"},
			},
		},
		{
			Title: "Домены",
			Items: []helpCmd{
				{Cmd: "/add", Args: "<domain>", Desc: "добавить домен в твою секцию", NeedAny: []Role{RoleDomainEditor}},
				{Cmd: "/del", Args: "<domain>", Desc: "удалить домен из твоей секции", NeedAny: []Role{RoleDomainEditor}},
				{Cmd: "/list", Desc: "показать домены твоей секции", NeedAny: []Role{RoleDomainEditor}},
				{Cmd: "/export", Desc: "экспорт всего списка", NeedAny: []Role{RoleDomainManager}},
			},
		},
		{
			Title: "WireGuard",
			Items: []helpCmd{
				{Cmd: "/wgstats", Args: "[user]", Desc: "статистика WireGuard (Admin может указать пользователя)", NeedAny: []Role{RoleWgStats}},
				{Cmd: "/wgstats_admin", Desc: "статистика WireGuard по всем профилям", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/wgprofiles", Desc: "меню WireGuard профилей (в личке)", NeedAny: []Role{RoleWgUserControl}},
				{Cmd: "/wgprofiles_admin", Desc: "админ-меню WireGuard профилей (в личке)", NeedAny: []Role{RoleAdmin}},
			},
		},
		{
			Title: "Сервисы",
			Items: []helpCmd{
				{Cmd: "/agh_update_lists", Desc: "обновить списки AdGuard", NeedAny: []Role{RoleServiceManager}},
			},
		},
		{
			Title: "AI",
			Items: []helpCmd{
				{Cmd: "/accounts", Desc: "получить учётную запись к сервису (только в личке)", NeedAny: []Role{RoleAiUser}},
			},
		},
		{
			Title: "Админка",
			Items: []helpCmd{
				{Cmd: "/adminmsg", Desc: "мягкий UI для сообщений (всем или конкретному)", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/broadcast", Args: "[сообщение]", Desc: "рассылка всем (без аргумента откроет UI)", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/message", Args: "[name] [сообщение]", Desc: "сообщение пользователю (без аргумента откроет UI)", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/users", Desc: "список пользователей", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/grant", Args: "<name> <role>", Desc: "выдать роль", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/revoke", Args: "<name> <role>", Desc: "снять роль", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/rename", Args: "<old> <new>", Desc: "переименовать пользователя и секцию доменов", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/roles_catalog", Desc: "список возможных ролей с описанием", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/settings", Desc: "показать настройки бота", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/log", Desc: "показать лог (хвост)", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/log_on", Desc: "включить логирование", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/log_off", Desc: "выключить логирование", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/log_clear", Desc: "очистить лог", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/bot_on", Desc: "включить бота для пользователей", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/bot_off", Desc: "отключить бота для пользователей", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/acc_list", Desc: "список accounts", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/acc_add", Args: "<name> <login> <password>", Desc: "добавить/обновить account", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/acc_set", Args: "<name> <login> <password>", Desc: "обновить account", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/acc_del", Args: "<name>", Desc: "удалить account", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/acc_rename", Args: "<old> <new>", Desc: "переименовать account", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/wgenable", Args: "<profile>", Desc: "включить WG профиль по имени (с выбором при неоднозначности)", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/wgdisable", Args: "<profile>", Desc: "выключить WG профиль по имени", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/wgrename", Args: "<old> <new>", Desc: "переименовать WG профиль по имени", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/wgdel", Args: "<profile>", Desc: "удалить WG профиль по имени", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/wgconf", Args: "<profile>", Desc: "получить конфиг WG профиля по имени", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/wgqr", Args: "<profile>", Desc: "получить QR WG профиля по имени", NeedAny: []Role{RoleAdmin}},
			},
		},
	}

	var b strings.Builder
	b.WriteString("Доступные команды:\n\n")

	printedAnyBlock := false
	for _, bl := range blocks {
		lines := make([]string, 0, len(bl.Items))
		for _, c := range bl.Items {
			if isCmdAllowed(u, c) {
				cmdline := c.Cmd
				if strings.TrimSpace(c.Args) != "" {
					cmdline += " " + c.Args
				}
				lines = append(lines, fmt.Sprintf("• %-32s — %s", cmdline, c.Desc))
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
