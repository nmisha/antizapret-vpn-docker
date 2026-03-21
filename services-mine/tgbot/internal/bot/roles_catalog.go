package bot

import "strings"

func handleRolesCatalog(ctx *Ctx, _ string) {
	desc := []struct {
		Role Role
		Desc string
	}{
		{RoleAdmin, "Полный доступ ко всем командам и разделам."},
		{RoleDomainManager, "Управление доменами: добавление/удаление, в том числе вне своей секции."},
		{RoleDomainEditor, "Управление доменами в своей секции в рамках разрешений."},
		{RoleServiceManager, "Сервисные команды для управления и обслуживания сервисов."},
		{RoleInfo, "Информационные команды."},
		{RoleNetUser, "Сетевые утилиты: DNS resolve и другие."},
		{RoleWgStats, "Доступ к /wg_stats для просмотра статистики WireGuard по своим профилям."},
		{RoleOvpnStats, "Доступ к /ovpn_stats для просмотра статистики OpenVPN по своим профилям."},
		{RoleWgUserControl, "Доступ к /wg_profiles: статистика, конфиг и QR по своим WireGuard-профилям."},
		{RoleOvpnUserControl, "Доступ к /ovpn_profiles: выдача OpenVPN-профилей."},
		{RoleSupport, "Получает сообщения из /support."},
		{RoleAiUser, "Доступ к командам выдачи учётных записей других сервисов, если это настроено."},
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
