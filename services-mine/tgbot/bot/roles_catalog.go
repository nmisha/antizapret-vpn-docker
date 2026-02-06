package main

import (
	"strings"
)

func handleRolesCatalog(ctx *Ctx, _ string) {
	// Admin-only (middleware/RequireRole already checks)
	desc := []struct {
		Role Role
		Desc string
	}{
		{RoleAdmin, "Полный доступ ко всем командам и разделам."},
		{RoleDomainManager, "Управление доменами: добавление/удаление (может удалять из любой секции)."},
		{RoleDomainEditor, "Управление доменами в своей секции: добавление/удаление в рамках разрешений."},
		{RoleServiceManager, "Сервисные команды (управление/обслуживание сервисов)."},
		{RoleInfo, "Информационные команды."},
		{RoleWgStats, "Доступ к /wgstats (просмотр статистики WireGuard по своим профилям)."},
		{RoleWgUserControl, "Доступ к /wgprofiles (UI: статистика/конфиг/QR по своим WireGuard-профилям)."},
		{RoleSupport, "Получает сообщения из /support (обращения пользователей)."},
		{RoleAiUser, "Доступ к командам для выдачи учётных записей других сервисов (если настроено)."},
	}

	var b strings.Builder
	b.WriteString("Доступные роли:\n\n")
	for _, d := range desc {
		b.WriteString("• ")
		b.WriteString(string(d.Role))
		b.WriteString(" — ")
		b.WriteString(d.Desc)
		b.WriteString("\n")
	}
	reply(ctx.Bot, ctx.ChatID, b.String())
}
