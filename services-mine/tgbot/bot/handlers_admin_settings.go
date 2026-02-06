package main

import (
	"strings"
)

func RegisterAdminSettingsHandlers(r *Router) {
	r.Handle("/log", handleLogShow, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."))
	r.Alias("log", "/log")

	r.Handle("/log_clear", handleLogClear, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."))
	r.Alias("log_clear", "/log_clear")

	r.Handle("/log_on", handleLogOn, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."))
	r.Alias("log_on", "/log_on")

	r.Handle("/log_off", handleLogOff, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."))
	r.Alias("log_off", "/log_off")

	r.Handle("/bot_on", handleBotOn, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."))
	r.Alias("bot_on", "/bot_on")

	r.Handle("/bot_off", handleBotOff, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."))
	r.Alias("bot_off", "/bot_off")

	r.Handle("/settings", handleSettingsShow, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."))
	r.Alias("settings", "/settings")
}

func handleSettingsShow(ctx *Ctx, _ string) {
	s := getSettingsCached()
	lines := []string{
		"Настройки бота:",
		"• logging_enabled: " + boolToOnOff(s.LoggingEnabled),
		"• bot_enabled_for_users: " + boolToOnOff(s.BotEnabledForUsers),
		"",
		"Команды:",
		"/log, /log_clear, /log_on, /log_off",
		"/bot_on, /bot_off",
	}
	reply(ctx.Bot, ctx.ChatID, strings.Join(lines, "\n"))
}

func handleLogShow(ctx *Ctx, _ string) {
	if gLogger == nil {
		reply(ctx.Bot, ctx.ChatID, "Логгер не настроен.")
		return
	}
	text, err := gLogger.ReadAll(3800)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка чтения лога: "+err.Error())
		return
	}
	if strings.TrimSpace(text) == "" {
		reply(ctx.Bot, ctx.ChatID, "Лог пуст.")
		return
	}
	reply(ctx.Bot, ctx.ChatID, text)
}

func handleLogClear(ctx *Ctx, _ string) {
	if gLogger == nil {
		reply(ctx.Bot, ctx.ChatID, "Логгер не настроен.")
		return
	}
	if err := gLogger.Clear(); err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка очистки лога: "+err.Error())
		return
	}
	reply(ctx.Bot, ctx.ChatID, "Лог очищен.")
}

func handleLogOn(ctx *Ctx, _ string) {
	s := getSettingsCached()
	s.LoggingEnabled = true
	if err := gSettings.Save(s); err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка сохранения настроек: "+err.Error())
		return
	}
	setSettingsCached(s)
	reply(ctx.Bot, ctx.ChatID, "Логирование включено.")
}

func handleLogOff(ctx *Ctx, _ string) {
	s := getSettingsCached()
	s.LoggingEnabled = false
	if err := gSettings.Save(s); err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка сохранения настроек: "+err.Error())
		return
	}
	setSettingsCached(s)
	reply(ctx.Bot, ctx.ChatID, "Логирование отключено.")
}

func handleBotOn(ctx *Ctx, _ string) {
	s := getSettingsCached()
	s.BotEnabledForUsers = true
	if err := gSettings.Save(s); err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка сохранения настроек: "+err.Error())
		return
	}
	setSettingsCached(s)
	reply(ctx.Bot, ctx.ChatID, "Бот включён для пользователей.")
}

func handleBotOff(ctx *Ctx, _ string) {
	s := getSettingsCached()
	s.BotEnabledForUsers = false
	if err := gSettings.Save(s); err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка сохранения настроек: "+err.Error())
		return
	}
	setSettingsCached(s)
	reply(ctx.Bot, ctx.ChatID, "Бот отключён для пользователей (кроме Admin).")
}

func boolToOnOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}
