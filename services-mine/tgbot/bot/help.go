package main

import (
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
			Title: "Быстрый старт",
			Items: []helpCmd{
				{Cmd: "/help", Desc: "показать эту справку"},
				{Cmd: "/roles", Args: "[name]", Desc: "показать роли (свои; Admin может смотреть чужие)"},
				{Cmd: "/support", Args: "[сообщение]", Desc: "написать в поддержку (личка или группа; без аргумента откроет UI)"},
			},
		},
		{
			Title: "Сеть",
			Items: []helpCmd{
				{Cmd: "/net_dns_resolve", Args: "[domain]", Desc: "DNS resolve домена → IP (без аргумента откроет UI)", NeedAny: []Role{RoleNetUser}},
			},
		},
		{
			Title: "Домены",
			Items: []helpCmd{
				{Cmd: "/add", Args: "<domain>", Desc: "добавить домен в твою секцию", NeedAny: []Role{RoleDomainEditor}},
				{Cmd: "/del", Args: "<domain>", Desc: "удалить домен из твоей секции", NeedAny: []Role{RoleDomainEditor}},
				{Cmd: "/export", Desc: "выгрузить домены", NeedAny: []Role{RoleDomainManager}},
			},
		},
		{
			Title: "WireGuard",
			Items: []helpCmd{
				{Cmd: "/wg_stats", Desc: "статистика WG (для своего пользователя)", NeedAny: []Role{RoleWgStats}},
				{Cmd: "/wg_stats_admin", Desc: "статистика WG (как админ, без имени)", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/wg_name", Args: "<client-prefix>", Desc: "найти WG клиента по имени (prefix, case-insensitive)"},
			},
		},
		{
			Title: "Сервисы",
			Items: []helpCmd{
				{Cmd: "/sr_agh_update_lists", Desc: "обновить списки AdGuard", NeedAny: []Role{RoleServiceManager}},
			},
		},
		{
			Title: "AI / Accounts",
			Items: []helpCmd{
				{Cmd: "/account", Desc: "получить учётные данные к сервису (выбор по имени)", NeedAny: []Role{RoleAiUser}},
			},
		},
		{
			Title: "Админ: пользователи и роли",
			Items: []helpCmd{
				{Cmd: "/users", Desc: "список пользователей и ролей", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/grant", Args: "<name> <role>", Desc: "выдать роль пользователю", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/revoke", Args: "<name> <role>", Desc: "снять роль с пользователя", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/roles_catalog", Desc: "все роли с описанием", NeedAny: []Role{RoleAdmin}},
			},
		},
		{
			Title: "Админ: аккаунты",
			Items: []helpCmd{
				{Cmd: "/accounts", Desc: "управление accounts (UI)", NeedAny: []Role{RoleAdmin}},
			},
		},
		{
			Title: "Админ: сообщения",
			Items: []helpCmd{
				{Cmd: "/adminmsg", Desc: "мягкий UI для сообщений (всем/одному)", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/broadcast", Args: "<text>", Desc: "отправить всем (без аргумента откроет UI)", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/message", Args: "<name> <text>", Desc: "отправить пользователю (без аргумента откроет UI)", NeedAny: []Role{RoleAdmin}},
			},
		},
		{
			Title: "Админ: настройки и логи",
			Items: []helpCmd{
				{Cmd: "/settings", Desc: "настройки бота (логирование, отключение ответов)", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/log", Desc: "показать лог", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/log_clear", Desc: "очистить лог", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/log_on", Desc: "включить логирование", NeedAny: []Role{RoleAdmin}},
				{Cmd: "/log_off", Desc: "выключить логирование", NeedAny: []Role{RoleAdmin}},
			},
		},
	}

	var b strings.Builder
	b.WriteString("📚 <b>Справка</b>\n")

	for _, bl := range blocks {
		items := filterHelpItems(bl.Items, u)
		if len(items) == 0 {
			continue
		}
		b.WriteString("\n<b>")
		b.WriteString(bl.Title)
		b.WriteString("</b>\n")
		for _, it := range items {
			line := it.Cmd
			if it.Args != "" {
				line += " " + it.Args
			}
			if it.Desc != "" {
				line += " — " + it.Desc
			}
			b.WriteString("• ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n💡 Подсказка: команды и роли не зависят от регистра.\n")
	return b.String()
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

// filterHelpItems returns only those help items that are visible for the user by roles.
func filterHelpItems(items []helpCmd, u User) []helpCmd {
	if len(items) == 0 {
		return nil
	}
	out := make([]helpCmd, 0, len(items))
	for _, it := range items {
		if isCmdAllowed(u, it) {
			out = append(out, it)
		}
	}
	return out
}
