package bot

import (
	"strconv"
	"strings"
)

func RegisterAdminSettingsHandlers(reg *CommandRegistry) {
	reg.Command(CommandSpec{Cmd: "/log", Args: "", Desc: "показать последние строки лога", Section: "Админ: настройки и логи", NeedAny: []Role{RoleAdmin}}, handleLogShow, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."))
	reg.Alias("log", "/log")

	reg.Command(CommandSpec{Cmd: "/log_tail", Args: "<N>", Desc: "настроить сколько строк показывает /log", Section: "Админ: настройки и логи", NeedAny: []Role{RoleAdmin}}, handleLogTailSet, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."))
	reg.Alias("log_tail", "/log_tail")

	reg.Command(CommandSpec{Cmd: "/log_clear", Args: "", Desc: "очистить лог", Section: "Админ: настройки и логи", NeedAny: []Role{RoleAdmin}}, handleLogClear, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."))
	reg.Alias("log_clear", "/log_clear")

	// Split logging toggles:
	reg.Command(CommandSpec{Cmd: "/log_errors_on", Args: "", Desc: "включить логирование ошибок", Section: "Админ: настройки и логи", NeedAny: []Role{RoleAdmin}}, handleLogErrorsOn, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."))
	reg.Alias("log_errors_on", "/log_errors_on")
	reg.Command(CommandSpec{Cmd: "/log_errors_off", Args: "", Desc: "выключить логирование ошибок", Section: "Админ: настройки и логи", NeedAny: []Role{RoleAdmin}}, handleLogErrorsOff, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."))
	reg.Alias("log_errors_off", "/log_errors_off")

	reg.Command(CommandSpec{Cmd: "/log_cmd_on", Args: "", Desc: "включить логирование команд", Section: "Админ: настройки и логи", NeedAny: []Role{RoleAdmin}}, handleLogCmdOn, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."))
	reg.Alias("log_cmd_on", "/log_cmd_on")
	reg.Command(CommandSpec{Cmd: "/log_cmd_off", Args: "", Desc: "выключить логирование команд", Section: "Админ: настройки и логи", NeedAny: []Role{RoleAdmin}}, handleLogCmdOff, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."))
	reg.Alias("log_cmd_off", "/log_cmd_off")

	// Backward compatible: toggles both
	reg.Command(CommandSpec{Cmd: "/log_on", Args: "", Desc: "включить логирование (оба)", Section: "Админ: настройки и логи", NeedAny: []Role{RoleAdmin}}, handleLogOn, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."))
	reg.Alias("log_on", "/log_on")
	reg.Command(CommandSpec{Cmd: "/log_off", Args: "", Desc: "выключить логирование (оба)", Section: "Админ: настройки и логи", NeedAny: []Role{RoleAdmin}}, handleLogOff, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."))
	reg.Alias("log_off", "/log_off")

	reg.Command(CommandSpec{Cmd: "/bot_on", Args: "", Desc: "включить ответы бота", Section: "Админ: настройки и логи", NeedAny: []Role{RoleAdmin}}, handleBotOn, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."))
	reg.Alias("bot_on", "/bot_on")

	reg.Command(CommandSpec{Cmd: "/bot_off", Args: "", Desc: "выключить ответы бота", Section: "Админ: настройки и логи", NeedAny: []Role{RoleAdmin}}, handleBotOff, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."))
	reg.Alias("bot_off", "/bot_off")

	reg.Command(CommandSpec{Cmd: "/settings", Args: "", Desc: "настройки бота (логирование, отключение ответов)", Section: "Админ: настройки и логи", NeedAny: []Role{RoleAdmin}}, handleSettingsShow, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."))
	reg.Alias("settings", "/settings")
}

func handleSettingsShow(ctx *Ctx, _ string) {
	s := getSettingsCached()
	lines := []string{
		"Настройки бота:",
		"• log_errors_enabled: " + boolToOnOff(s.LogErrorsEnabled),
		"• log_cmd_enabled: " + boolToOnOff(s.LogCommandsEnabled),
		"• log_tail_lines: " + strconv.Itoa(s.LogTailLines),
		"• bot_enabled_for_users: " + boolToOnOff(s.BotEnabledForUsers),
		"",
		"Команды:",
		"/log — показать последние строки лога",
		"/log_tail <N> — сколько строк показывать в /log",
		"/log_clear — очистить лог",
		"/log_errors_on, /log_errors_off",
		"/log_cmd_on, /log_cmd_off",
		"/bot_on, /bot_off",
	}
	reply(ctx.Bot, ctx.ChatID, strings.Join(lines, "\n"))
}

func handleLogShow(ctx *Ctx, _ string) {
	if gLogger == nil {
		reply(ctx.Bot, ctx.ChatID, "Логгер не настроен.")
		return
	}
	s := getSettingsCached()
	text, err := gLogger.ReadTailLines(s.LogTailLines, 120000)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка чтения лога: "+err.Error())
		return
	}
	if strings.TrimSpace(text) == "" {
		reply(ctx.Bot, ctx.ChatID, "Лог пуст.")
		return
	}
	sendTextChunks(ctx.Bot, ctx.ChatID, text)
}

func handleLogTailSet(ctx *Ctx, arg string) {
	n, err := strconv.Atoi(strings.TrimSpace(arg))
	if err != nil || n <= 0 || n > 2000 {
		reply(ctx.Bot, ctx.ChatID, "Укажи число строк 1..2000. Например: /log_tail 200")
		return
	}
	s := getSettingsCached()
	s.LogTailLines = n
	if err := gSettings.Save(s); err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка сохранения настроек: "+err.Error())
		return
	}
	setSettingsCached(s)
	reply(ctx.Bot, ctx.ChatID, "Ок. /log будет показывать последние "+strconv.Itoa(n)+" строк.")
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

func handleLogErrorsOn(ctx *Ctx, _ string) {
	s := getSettingsCached()
	s.LogErrorsEnabled = true
	s.LoggingEnabled = true // legacy mirror
	if err := gSettings.Save(s); err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка сохранения настроек: "+err.Error())
		return
	}
	setSettingsCached(s)
	reply(ctx.Bot, ctx.ChatID, "Логирование ошибок включено.")
}

func handleLogErrorsOff(ctx *Ctx, _ string) {
	s := getSettingsCached()
	s.LogErrorsEnabled = false
	// legacy: keep in sync if both disabled
	s.LoggingEnabled = s.LogErrorsEnabled || s.LogCommandsEnabled
	if err := gSettings.Save(s); err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка сохранения настроек: "+err.Error())
		return
	}
	setSettingsCached(s)
	reply(ctx.Bot, ctx.ChatID, "Логирование ошибок отключено.")
}

func handleLogCmdOn(ctx *Ctx, _ string) {
	s := getSettingsCached()
	s.LogCommandsEnabled = true
	s.LoggingEnabled = true // legacy mirror
	if err := gSettings.Save(s); err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка сохранения настроек: "+err.Error())
		return
	}
	setSettingsCached(s)
	reply(ctx.Bot, ctx.ChatID, "Логирование команд включено.")
}

func handleLogCmdOff(ctx *Ctx, _ string) {
	s := getSettingsCached()
	s.LogCommandsEnabled = false
	s.LoggingEnabled = s.LogErrorsEnabled || s.LogCommandsEnabled
	if err := gSettings.Save(s); err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка сохранения настроек: "+err.Error())
		return
	}
	setSettingsCached(s)
	reply(ctx.Bot, ctx.ChatID, "Логирование команд отключено.")
}

func handleLogOn(ctx *Ctx, _ string) {
	s := getSettingsCached()
	s.LogErrorsEnabled = true
	s.LogCommandsEnabled = true
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
	s.LogErrorsEnabled = false
	s.LogCommandsEnabled = false
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
