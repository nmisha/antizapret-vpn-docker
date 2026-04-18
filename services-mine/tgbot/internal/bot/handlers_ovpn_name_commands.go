package bot

import (
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const ovpnNameCbPrefix = "ovpnn:"

func RegisterOvpnNameCommands(reg *CommandRegistry) {
	reg.Command(
		CommandSpec{
			Cmd:     "/ovpn_create",
			Args:    "<certificate_name>",
			Desc:    "создать новый сертификат",
			Section: "Админ: OpenVPN",
			NeedAny: []Role{RoleAdmin},
		},
		handleOvpnNameCreate,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequireNonEmptyArg("Формат: /ovpn_create <certificate_name>"),
	)
	reg.Command(
		CommandSpec{
			Cmd:     "/ovpn_conf",
			Args:    "<profile_name_or_prefix>",
			Desc:    "получить конфиг профиля",
			Section: "Админ: OpenVPN",
			NeedAny: []Role{RoleAdmin},
		},
		handleOvpnNameConf,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequireNonEmptyArg("Формат: /ovpn_conf <profile_name_or_prefix>"),
	)
	reg.Command(
		CommandSpec{
			Cmd:     "/ovpn_stats_profile",
			Args:    "<profile_name_or_prefix>",
			Desc:    "статистика по профилю",
			Section: "Админ: OpenVPN",
			NeedAny: []Role{RoleAdmin},
		},
		handleOvpnNameStatsProfile,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequireNonEmptyArg("Формат: /ovpn_stats_profile <profile_name_or_prefix>"),
	)
	reg.Command(
		CommandSpec{
			Cmd:     "/ovpn_score",
			Args:    "<profile_name_or_prefix>",
			Desc:    "получить risk score профиля",
			Section: "Админ: OpenVPN",
			NeedAny: []Role{RoleAdmin},
		},
		handleOvpnNameScore,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequireNonEmptyArg("Формат: /ovpn_score <profile_name_or_prefix>"),
	)
	reg.Command(
		CommandSpec{
			Cmd:     "/ovpn_revoke",
			Args:    "<profile_name_or_prefix>",
			Desc:    "отозвать сертификат",
			Section: "Админ: OpenVPN",
			NeedAny: []Role{RoleAdmin},
		},
		handleOvpnNameRevoke,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequireNonEmptyArg("Формат: /ovpn_revoke <profile_name_or_prefix>"),
	)
	reg.Command(
		CommandSpec{
			Cmd:     "/ovpn_burn",
			Args:    "<profile_name_or_prefix>",
			Desc:    "удалить уже отозванный сертификат",
			Section: "Админ: OpenVPN",
			NeedAny: []Role{RoleAdmin},
		},
		handleOvpnNameBurn,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequireNonEmptyArg("Формат: /ovpn_burn <profile_name_or_prefix>"),
	)
	reg.Command(
		CommandSpec{
			Cmd:     "/ovpn_restart",
			Args:    "<profile_name_or_prefix>",
			Desc:    "перезапустить OpenVPN server",
			Section: "Админ: OpenVPN",
			NeedAny: []Role{RoleAdmin},
		},
		handleOvpnNameRestart,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequireNonEmptyArg("Формат: /ovpn_restart <profile_name_or_prefix>"),
	)
	reg.Command(
		CommandSpec{
			Cmd:     "/ovpn_restart_container",
			Args:    "<profile_name_or_prefix>",
			Desc:    "перезапустить OpenVPN container",
			Section: "Админ: OpenVPN",
			NeedAny: []Role{RoleAdmin},
		},
		handleOvpnNameRestartContainer,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequireNonEmptyArg("Формат: /ovpn_restart_container <profile_name_or_prefix>"),
	)
}

func handleOvpnNameCreate(ctx *Ctx, arg string) {
	if !ctx.IsPrivate {
		reply(ctx.Bot, ctx.ChatID, "Эта команда доступна только в личных сообщениях боту.")
		return
	}
	name := strings.TrimSpace(arg)
	if name == "" {
		reply(ctx.Bot, ctx.ChatID, "Формат: /ovpn_create <certificate_name>")
		return
	}
	if strings.ContainsAny(name, " \t\r\n") {
		reply(ctx.Bot, ctx.ChatID, "Имя сертификата должно быть без пробелов.")
		return
	}
	client, err := newOvpnUIClientFromEnv()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, err.Error())
		return
	}
	if err := client.createProfile(name); err != nil {
		reply(ctx.Bot, ctx.ChatID, "Не удалось создать OpenVPN сертификат:\n"+truncate(err.Error(), 3500))
		return
	}
	reply(ctx.Bot, ctx.ChatID, "OK: OpenVPN certificate created")
}

func handleOvpnNameConf(ctx *Ctx, arg string) {
	ovpnNameAction(ctx, "conf", arg)
}

func handleOvpnNameStatsProfile(ctx *Ctx, arg string) {
	ovpnNameAction(ctx, "stats", arg)
}

func handleOvpnNameScore(ctx *Ctx, arg string) {
	ovpnNameAction(ctx, "score", arg)
}

func handleOvpnNameRevoke(ctx *Ctx, arg string) {
	ovpnNameAction(ctx, "revoke", arg)
}

func handleOvpnNameBurn(ctx *Ctx, arg string) {
	ovpnNameAction(ctx, "burn", arg)
}

func handleOvpnNameRestart(ctx *Ctx, arg string) {
	ovpnNameAction(ctx, "restart", arg)
}

func handleOvpnNameRestartContainer(ctx *Ctx, arg string) {
	ovpnNameAction(ctx, "restart_container", arg)
}

func ovpnNameAction(ctx *Ctx, action string, query string) {
	if !ctx.IsPrivate {
		reply(ctx.Bot, ctx.ChatID, "Эта команда доступна только в личных сообщениях боту.")
		return
	}
	client, err := newOvpnUIClientFromEnv()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, err.Error())
		return
	}
	profiles, err := client.listProfiles()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Не удалось получить список OpenVPN профилей:\n"+truncate(err.Error(), 3500))
		return
	}
	matches := matchOvpnProfilesByNamePrefix(profiles, query)
	if len(matches) == 0 {
		reply(ctx.Bot, ctx.ChatID, "Профиль не найден: "+query)
		return
	}
	if len(matches) == 1 {
		performOvpnProfileAction(ctx, client, action, matches[0].Name)
		return
	}

	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(matches)+1)
	for _, p := range matches {
		cb := ovpnNameCbPrefix + action + ":" + p.Name
		rows = append(rows, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(p.Name, cb),
		})
	}
	rows = append(rows, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("Cancel", "ui:cancel"),
	})
	kb := tgbotapi.NewInlineKeyboardMarkup(rows...)
	m := tgbotapi.NewMessage(ctx.ChatID, "Найдено несколько профилей. Выбери точный:")
	m.ReplyMarkup = kb
	_, err = ctx.Bot.Send(m)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", "inline send")
}

func matchOvpnProfilesByNamePrefix(profiles []ovpnProfile, query string) []ovpnProfile {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	out := make([]ovpnProfile, 0, 8)
	for _, p := range profiles {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(p.Name)), q) {
			out = append(out, p)
		}
	}
	return out
}

func performOvpnProfileAction(ctx *Ctx, client *ovpnUIClient, action string, profileName string) {
	profile, ok := findOvpnProfileByExactName(client, profileName)
	if !ok && action != "restart" {
		reply(ctx.Bot, ctx.ChatID, "Профиль больше не найден: "+profileName)
		return
	}

	switch action {
	case "conf":
		sendOvpnConfigAsFile(ctx, client, profileName)
	case "stats":
		sendOvpnStatsForProfile(ctx, client, profileName)
	case "score":
		sendDNSGuardRiskScore(ctx, "ovpn", profileName, true)
	case "revoke":
		if strings.TrimSpace(profile.RevokeURL) == "" {
			reply(ctx.Bot, ctx.ChatID, "Для выбранного профиля это действие сейчас недоступно.")
			return
		}
		if err := client.executeProfileAction(profile.RevokeURL); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Не удалось выполнить действие OpenVPN:\n"+truncate(err.Error(), 3500))
			return
		}
		reply(ctx.Bot, ctx.ChatID, "Сертификат OpenVPN отозван. Для немедленного разрыва активной сессии может потребоваться restart OpenVPN.")
	case "burn":
		if strings.TrimSpace(profile.BurnURL) == "" {
			reply(ctx.Bot, ctx.ChatID, "Для выбранного профиля это действие сейчас недоступно.")
			return
		}
		if err := client.executeProfileAction(profile.BurnURL); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Не удалось выполнить действие OpenVPN:\n"+truncate(err.Error(), 3500))
			return
		}
		reply(ctx.Bot, ctx.ChatID, "Отозванный сертификат OpenVPN удален.")
	case "restart":
		if err := client.restartServer("SIGUSR1"); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Не удалось перезапустить OpenVPN server:\n"+truncate(err.Error(), 3500))
			return
		}
		reply(ctx.Bot, ctx.ChatID, "OpenVPN server restarted (SIGUSR1). Активные клиентские сессии должны быть переинициализированы.")
	case "restart_container":
		if strings.TrimSpace(profile.RestartContainerURL) == "" {
			reply(ctx.Bot, ctx.ChatID, "Restart container недоступен в текущем OpenVPN UI.")
			return
		}
		if err := client.executeProfileAction(profile.RestartContainerURL); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Не удалось перезапустить OpenVPN container:\n"+truncate(err.Error(), 3500))
			return
		}
		reply(ctx.Bot, ctx.ChatID, "OpenVPN container restart triggered.")
	default:
		reply(ctx.Bot, ctx.ChatID, "Неизвестное действие.")
	}
}

func findOvpnProfileByExactName(client *ovpnUIClient, profileName string) (ovpnProfile, bool) {
	profiles, err := client.listProfiles()
	if err != nil {
		return ovpnProfile{}, false
	}
	want := strings.ToLower(strings.TrimSpace(profileName))
	for _, profile := range profiles {
		if strings.ToLower(strings.TrimSpace(profile.Name)) == want {
			return profile, true
		}
	}
	return ovpnProfile{}, false
}
