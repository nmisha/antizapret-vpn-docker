package bot

import "strings"

func handleOvpnCallback(ctx *Ctx, data string) {
	if !ctx.IsPrivate {
		reply(ctx.Bot, ctx.ChatID, "OpenVPN доступен только в личных сообщениях боту.")
		return
	}

	if strings.HasPrefix(data, "ovpn:u:p:") {
		profileName := strings.TrimPrefix(data, "ovpn:u:p:")
		client, err := newOvpnUIClientFromEnv()
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, err.Error())
			return
		}
		if _, err := validateOvpnUserProfileAccess(ctx, client, profileName); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Доступ к профилю недоступен: "+err.Error())
			return
		}
		sendOvpnProfileActionsWithStats(ctx, profileName, "ovpn:u")
		return
	}
	if strings.HasPrefix(data, "ovpn:u:act:") {
		rest := strings.TrimPrefix(data, "ovpn:u:act:")
		parts := strings.SplitN(rest, ":", 2)
		if len(parts) != 2 {
			reply(ctx.Bot, ctx.ChatID, "Неверный формат.")
			return
		}
		act, profileName := parts[0], parts[1]
		client, err := newOvpnUIClientFromEnv()
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, err.Error())
			return
		}
		if _, err := validateOvpnUserProfileAccess(ctx, client, profileName); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Доступ к профилю недоступен: "+err.Error())
			return
		}
		switch act {
		case "stats":
			sendOvpnStatsForProfile(ctx, client, profileName)
		case "conf":
			sendOvpnConfigAsFile(ctx, client, profileName)
		default:
			reply(ctx.Bot, ctx.ChatID, "Неизвестное действие.")
		}
		return
	}
	if data == "ovpn:u:back" {
		handleOvpnProfiles(ctx, "")
		return
	}

	if strings.HasPrefix(data, "ovpn:a:scope:") {
		scope := strings.TrimPrefix(data, "ovpn:a:scope:")
		switch scope {
		case "my":
			setAdminScope(ctx.TgID, wgAdminScope{Mode: wgScopeMy})
			profiles, err := getOvpnProfilesForScope(ctx, wgAdminScope{Mode: wgScopeMy})
			if err != nil {
				reply(ctx.Bot, ctx.ChatID, "Ошибка: "+truncate(err.Error(), 3500))
				return
			}
			sendOvpnProfilesList(ctx, profiles, "ovpn:a:p:")
			return
		case "all":
			setAdminScope(ctx.TgID, wgAdminScope{Mode: wgScopeAll})
			profiles, err := getOvpnProfilesForScope(ctx, wgAdminScope{Mode: wgScopeAll})
			if err != nil {
				reply(ctx.Bot, ctx.ChatID, "Ошибка: "+truncate(err.Error(), 3500))
				return
			}
			sendOvpnProfilesList(ctx, profiles, "ovpn:a:p:")
			return
		case "user":
			sendOvpnUsersList(ctx)
			return
		default:
			reply(ctx.Bot, ctx.ChatID, "Неверный scope.")
			return
		}
	}
	if strings.HasPrefix(data, "ovpn:a:user:") {
		name := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(data, "ovpn:a:user:")))
		setAdminScope(ctx.TgID, wgAdminScope{Mode: wgScopeUser, UserName: name})
		profiles, err := getOvpnProfilesForScope(ctx, wgAdminScope{Mode: wgScopeUser, UserName: name})
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Ошибка: "+truncate(err.Error(), 3500))
			return
		}
		sendOvpnProfilesList(ctx, profiles, "ovpn:a:p:")
		return
	}
	if data == "ovpn:a:back" {
		clearAdminScope(ctx.TgID)
		handleOvpnProfilesAdmin(ctx, "")
		return
	}
	if strings.HasPrefix(data, "ovpn:a:p:") {
		profileName := strings.TrimPrefix(data, "ovpn:a:p:")
		client, err := newOvpnUIClientFromEnv()
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, err.Error())
			return
		}
		if _, err := validateOvpnAdminProfileAccess(ctx, client, profileName); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Действие недоступно: "+err.Error())
			return
		}
		sendOvpnProfileActionsWithStats(ctx, profileName, "ovpn:a")
		return
	}
	if strings.HasPrefix(data, "ovpn:a:act:") {
		rest := strings.TrimPrefix(data, "ovpn:a:act:")
		parts := strings.SplitN(rest, ":", 2)
		if len(parts) != 2 {
			reply(ctx.Bot, ctx.ChatID, "Неверный формат.")
			return
		}
		act, profileName := parts[0], parts[1]
		client, err := newOvpnUIClientFromEnv()
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, err.Error())
			return
		}
		if _, err := validateOvpnAdminProfileAccess(ctx, client, profileName); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Действие недоступно: "+err.Error())
			return
		}
		switch act {
		case "stats":
			sendOvpnStatsForProfile(ctx, client, profileName)
		case "conf":
			sendOvpnConfigAsFile(ctx, client, profileName)
		default:
			reply(ctx.Bot, ctx.ChatID, "Неизвестное действие.")
		}
		return
	}

	reply(ctx.Bot, ctx.ChatID, "Неизвестный callback.")
}
