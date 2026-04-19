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
			Desc:    "СЃРѕР·РґР°С‚СЊ РЅРѕРІС‹Р№ СЃРµСЂС‚РёС„РёРєР°С‚",
			Section: "РђРґРјРёРЅ: OpenVPN",
			NeedAny: []Role{RoleAdmin},
		},
		handleOvpnNameCreate,
		RequireRole(RoleAdmin, "РќРµРґРѕСЃС‚Р°С‚РѕС‡РЅРѕ РїСЂР°РІ. РќСѓР¶РЅР° СЂРѕР»СЊ Admin."),
		RequireNonEmptyArg("Р¤РѕСЂРјР°С‚: /ovpn_create <certificate_name>"),
	)
	reg.Command(
		CommandSpec{
			Cmd:     "/ovpn_conf",
			Args:    "<profile_name_or_prefix>",
			Desc:    "РїРѕР»СѓС‡РёС‚СЊ РєРѕРЅС„РёРі РїСЂРѕС„РёР»СЏ",
			Section: "РђРґРјРёРЅ: OpenVPN",
			NeedAny: []Role{RoleAdmin},
		},
		handleOvpnNameConf,
		RequireRole(RoleAdmin, "РќРµРґРѕСЃС‚Р°С‚РѕС‡РЅРѕ РїСЂР°РІ. РќСѓР¶РЅР° СЂРѕР»СЊ Admin."),
		RequireNonEmptyArg("Р¤РѕСЂРјР°С‚: /ovpn_conf <profile_name_or_prefix>"),
	)
	reg.Command(
		CommandSpec{
			Cmd:     "/ovpn_stats_profile",
			Args:    "<profile_name_or_prefix>",
			Desc:    "СЃС‚Р°С‚РёСЃС‚РёРєР° РїРѕ РїСЂРѕС„РёР»СЋ",
			Section: "РђРґРјРёРЅ: OpenVPN",
			NeedAny: []Role{RoleAdmin},
		},
		handleOvpnNameStatsProfile,
		RequireRole(RoleAdmin, "РќРµРґРѕСЃС‚Р°С‚РѕС‡РЅРѕ РїСЂР°РІ. РќСѓР¶РЅР° СЂРѕР»СЊ Admin."),
		RequireNonEmptyArg("Р¤РѕСЂРјР°С‚: /ovpn_stats_profile <profile_name_or_prefix>"),
	)
	reg.Command(
		CommandSpec{
			Cmd:     "/ovpn_score",
			Args:    "<profile_name_or_prefix>",
			Desc:    "РїРѕР»СѓС‡РёС‚СЊ risk score РїСЂРѕС„РёР»СЏ",
			Section: "РђРґРјРёРЅ: OpenVPN",
			NeedAny: []Role{RoleAdmin},
		},
		handleOvpnNameScore,
		RequireRole(RoleAdmin, "РќРµРґРѕСЃС‚Р°С‚РѕС‡РЅРѕ РїСЂР°РІ. РќСѓР¶РЅР° СЂРѕР»СЊ Admin."),
		RequireNonEmptyArg("Р¤РѕСЂРјР°С‚: /ovpn_score <profile_name_or_prefix>"),
	)
	reg.Command(
		CommandSpec{
			Cmd:     "/ovpn_reset_score",
			Args:    "<profile_name_or_prefix>",
			Desc:    "reset dns-guard risk score",
			Section: "Admin: OpenVPN",
			NeedAny: []Role{RoleAdmin},
		},
		handleOvpnNameResetScore,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequireNonEmptyArg("Формат: /ovpn_reset_score <profile_name_or_prefix>"),
	)
	reg.Command(
		CommandSpec{
			Cmd:     "/ovpn_revoke",
			Args:    "<profile_name_or_prefix>",
			Desc:    "РѕС‚РѕР·РІР°С‚СЊ СЃРµСЂС‚РёС„РёРєР°С‚",
			Section: "РђРґРјРёРЅ: OpenVPN",
			NeedAny: []Role{RoleAdmin},
		},
		handleOvpnNameRevoke,
		RequireRole(RoleAdmin, "РќРµРґРѕСЃС‚Р°С‚РѕС‡РЅРѕ РїСЂР°РІ. РќСѓР¶РЅР° СЂРѕР»СЊ Admin."),
		RequireNonEmptyArg("Р¤РѕСЂРјР°С‚: /ovpn_revoke <profile_name_or_prefix>"),
	)
	reg.Command(
		CommandSpec{
			Cmd:     "/ovpn_burn",
			Args:    "<profile_name_or_prefix>",
			Desc:    "СѓРґР°Р»РёС‚СЊ СѓР¶Рµ РѕС‚РѕР·РІР°РЅРЅС‹Р№ СЃРµСЂС‚РёС„РёРєР°С‚",
			Section: "РђРґРјРёРЅ: OpenVPN",
			NeedAny: []Role{RoleAdmin},
		},
		handleOvpnNameBurn,
		RequireRole(RoleAdmin, "РќРµРґРѕСЃС‚Р°С‚РѕС‡РЅРѕ РїСЂР°РІ. РќСѓР¶РЅР° СЂРѕР»СЊ Admin."),
		RequireNonEmptyArg("Р¤РѕСЂРјР°С‚: /ovpn_burn <profile_name_or_prefix>"),
	)
	reg.Command(
		CommandSpec{
			Cmd:     "/ovpn_restart",
			Args:    "<profile_name_or_prefix>",
			Desc:    "РїРµСЂРµР·Р°РїСѓСЃС‚РёС‚СЊ OpenVPN server",
			Section: "РђРґРјРёРЅ: OpenVPN",
			NeedAny: []Role{RoleAdmin},
		},
		handleOvpnNameRestart,
		RequireRole(RoleAdmin, "РќРµРґРѕСЃС‚Р°С‚РѕС‡РЅРѕ РїСЂР°РІ. РќСѓР¶РЅР° СЂРѕР»СЊ Admin."),
		RequireNonEmptyArg("Р¤РѕСЂРјР°С‚: /ovpn_restart <profile_name_or_prefix>"),
	)
	reg.Command(
		CommandSpec{
			Cmd:     "/ovpn_restart_container",
			Args:    "<profile_name_or_prefix>",
			Desc:    "РїРµСЂРµР·Р°РїСѓСЃС‚РёС‚СЊ OpenVPN container",
			Section: "РђРґРјРёРЅ: OpenVPN",
			NeedAny: []Role{RoleAdmin},
		},
		handleOvpnNameRestartContainer,
		RequireRole(RoleAdmin, "РќРµРґРѕСЃС‚Р°С‚РѕС‡РЅРѕ РїСЂР°РІ. РќСѓР¶РЅР° СЂРѕР»СЊ Admin."),
		RequireNonEmptyArg("Р¤РѕСЂРјР°С‚: /ovpn_restart_container <profile_name_or_prefix>"),
	)
}

func handleOvpnNameCreate(ctx *Ctx, arg string) {
	if !ctx.IsPrivate {
		reply(ctx.Bot, ctx.ChatID, "Р­С‚Р° РєРѕРјР°РЅРґР° РґРѕСЃС‚СѓРїРЅР° С‚РѕР»СЊРєРѕ РІ Р»РёС‡РЅС‹С… СЃРѕРѕР±С‰РµРЅРёСЏС… Р±РѕС‚Сѓ.")
		return
	}
	name := strings.TrimSpace(arg)
	if name == "" {
		reply(ctx.Bot, ctx.ChatID, "Р¤РѕСЂРјР°С‚: /ovpn_create <certificate_name>")
		return
	}
	if strings.ContainsAny(name, " \t\r\n") {
		reply(ctx.Bot, ctx.ChatID, "РРјСЏ СЃРµСЂС‚РёС„РёРєР°С‚Р° РґРѕР»Р¶РЅРѕ Р±С‹С‚СЊ Р±РµР· РїСЂРѕР±РµР»РѕРІ.")
		return
	}
	client, err := newOvpnUIClientFromEnv()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, err.Error())
		return
	}
	if err := client.createProfile(name); err != nil {
		reply(ctx.Bot, ctx.ChatID, "РќРµ СѓРґР°Р»РѕСЃСЊ СЃРѕР·РґР°С‚СЊ OpenVPN СЃРµСЂС‚РёС„РёРєР°С‚:\n"+truncate(err.Error(), 3500))
		return
	}
	reply(ctx.Bot, ctx.ChatID, "OK: OpenVPN certificate created")
}

func handleOvpnNameConf(ctx *Ctx, arg string)         { ovpnNameAction(ctx, "conf", arg) }
func handleOvpnNameStatsProfile(ctx *Ctx, arg string) { ovpnNameAction(ctx, "stats", arg) }
func handleOvpnNameScore(ctx *Ctx, arg string)        { ovpnNameAction(ctx, "score", arg) }
func handleOvpnNameResetScore(ctx *Ctx, arg string)   { ovpnNameAction(ctx, "reset_score", arg) }
func handleOvpnNameRevoke(ctx *Ctx, arg string)       { ovpnNameAction(ctx, "revoke", arg) }
func handleOvpnNameBurn(ctx *Ctx, arg string)         { ovpnNameAction(ctx, "burn", arg) }
func handleOvpnNameRestart(ctx *Ctx, arg string)      { ovpnNameAction(ctx, "restart", arg) }
func handleOvpnNameRestartContainer(ctx *Ctx, arg string) {
	ovpnNameAction(ctx, "restart_container", arg)
}

func ovpnNameAction(ctx *Ctx, action string, query string) {
	if !ctx.IsPrivate {
		reply(ctx.Bot, ctx.ChatID, "Р­С‚Р° РєРѕРјР°РЅРґР° РґРѕСЃС‚СѓРїРЅР° С‚РѕР»СЊРєРѕ РІ Р»РёС‡РЅС‹С… СЃРѕРѕР±С‰РµРЅРёСЏС… Р±РѕС‚Сѓ.")
		return
	}
	client, err := newOvpnUIClientFromEnv()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, err.Error())
		return
	}
	profiles, err := client.listProfiles()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "РќРµ СѓРґР°Р»РѕСЃСЊ РїРѕР»СѓС‡РёС‚СЊ СЃРїРёСЃРѕРє OpenVPN РїСЂРѕС„РёР»РµР№:\n"+truncate(err.Error(), 3500))
		return
	}
	matches := matchOvpnProfilesByNamePrefix(profiles, query)
	if len(matches) == 0 {
		reply(ctx.Bot, ctx.ChatID, "РџСЂРѕС„РёР»СЊ РЅРµ РЅР°Р№РґРµРЅ: "+query)
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
	m := tgbotapi.NewMessage(ctx.ChatID, "РќР°Р№РґРµРЅРѕ РЅРµСЃРєРѕР»СЊРєРѕ РїСЂРѕС„РёР»РµР№. Р’С‹Р±РµСЂРё С‚РѕС‡РЅС‹Р№:")
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
		reply(ctx.Bot, ctx.ChatID, "РџСЂРѕС„РёР»СЊ Р±РѕР»СЊС€Рµ РЅРµ РЅР°Р№РґРµРЅ: "+profileName)
		return
	}

	switch action {
	case "conf":
		sendOvpnConfigAsFile(ctx, client, profileName)
	case "stats":
		sendOvpnStatsForProfile(ctx, client, profileName)
	case "score":
		sendDNSGuardRiskScore(ctx, "ovpn", profileName, true)
	case "reset_score":
		sendConfirm(ctx, "РЎР±СЂРѕСЃРёС‚СЊ risk score Рё state РґР»СЏ OpenVPN РїСЂРѕС„РёР»СЏ "+profileName+"?", "ovpn:reset_score", profileName)
	case "revoke":
		if strings.TrimSpace(profile.RevokeURL) == "" {
			reply(ctx.Bot, ctx.ChatID, "Р”Р»СЏ РІС‹Р±СЂР°РЅРЅРѕРіРѕ РїСЂРѕС„РёР»СЏ СЌС‚Рѕ РґРµР№СЃС‚РІРёРµ СЃРµР№С‡Р°СЃ РЅРµРґРѕСЃС‚СѓРїРЅРѕ.")
			return
		}
		if err := client.executeProfileAction(profile.RevokeURL); err != nil {
			reply(ctx.Bot, ctx.ChatID, "РќРµ СѓРґР°Р»РѕСЃСЊ РІС‹РїРѕР»РЅРёС‚СЊ РґРµР№СЃС‚РІРёРµ OpenVPN:\n"+truncate(err.Error(), 3500))
			return
		}
		reply(ctx.Bot, ctx.ChatID, "РЎРµСЂС‚РёС„РёРєР°С‚ OpenVPN РѕС‚РѕР·РІР°РЅ. Р”Р»СЏ РЅРµРјРµРґР»РµРЅРЅРѕРіРѕ СЂР°Р·СЂС‹РІР° Р°РєС‚РёРІРЅРѕР№ СЃРµСЃСЃРёРё РјРѕР¶РµС‚ РїРѕС‚СЂРµР±РѕРІР°С‚СЊСЃСЏ restart OpenVPN.")
	case "burn":
		if strings.TrimSpace(profile.BurnURL) == "" {
			reply(ctx.Bot, ctx.ChatID, "Р”Р»СЏ РІС‹Р±СЂР°РЅРЅРѕРіРѕ РїСЂРѕС„РёР»СЏ СЌС‚Рѕ РґРµР№СЃС‚РІРёРµ СЃРµР№С‡Р°СЃ РЅРµРґРѕСЃС‚СѓРїРЅРѕ.")
			return
		}
		if err := client.executeProfileAction(profile.BurnURL); err != nil {
			reply(ctx.Bot, ctx.ChatID, "РќРµ СѓРґР°Р»РѕСЃСЊ РІС‹РїРѕР»РЅРёС‚СЊ РґРµР№СЃС‚РІРёРµ OpenVPN:\n"+truncate(err.Error(), 3500))
			return
		}
		reply(ctx.Bot, ctx.ChatID, "РћС‚РѕР·РІР°РЅРЅС‹Р№ СЃРµСЂС‚РёС„РёРєР°С‚ OpenVPN СѓРґР°Р»РµРЅ.")
	case "restart":
		if err := client.restartServer("SIGUSR1"); err != nil {
			reply(ctx.Bot, ctx.ChatID, "РќРµ СѓРґР°Р»РѕСЃСЊ РїРµСЂРµР·Р°РїСѓСЃС‚РёС‚СЊ OpenVPN server:\n"+truncate(err.Error(), 3500))
			return
		}
		reply(ctx.Bot, ctx.ChatID, "OpenVPN server restarted (SIGUSR1). РђРєС‚РёРІРЅС‹Рµ РєР»РёРµРЅС‚СЃРєРёРµ СЃРµСЃСЃРёРё РґРѕР»Р¶РЅС‹ Р±С‹С‚СЊ РїРµСЂРµРёРЅРёС†РёР°Р»РёР·РёСЂРѕРІР°РЅС‹.")
	case "restart_container":
		if strings.TrimSpace(profile.RestartContainerURL) == "" {
			reply(ctx.Bot, ctx.ChatID, "Р”Р»СЏ РІС‹Р±СЂР°РЅРЅРѕРіРѕ РїСЂРѕС„РёР»СЏ СЌС‚Рѕ РґРµР№СЃС‚РІРёРµ СЃРµР№С‡Р°СЃ РЅРµРґРѕСЃС‚СѓРїРЅРѕ.")
			return
		}
		if err := client.executeProfileAction(profile.RestartContainerURL); err != nil {
			reply(ctx.Bot, ctx.ChatID, "РќРµ СѓРґР°Р»РѕСЃСЊ РїРµСЂРµР·Р°РїСѓСЃС‚РёС‚СЊ OpenVPN container:\n"+truncate(err.Error(), 3500))
			return
		}
		reply(ctx.Bot, ctx.ChatID, "OpenVPN container restart triggered.")
	default:
		reply(ctx.Bot, ctx.ChatID, "РќРµРёР·РІРµСЃС‚РЅРѕРµ РґРµР№СЃС‚РІРёРµ.")
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
