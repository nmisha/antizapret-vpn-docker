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
		case "score":
			sendDNSGuardRiskScore(ctx, "ovpn", profileName, false)
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
			setOvpnAdminScope(ctx.TgID, wgAdminScope{Mode: wgScopeMy})
			profiles, err := getOvpnProfilesForScope(ctx, wgAdminScope{Mode: wgScopeMy})
			if err != nil {
				reply(ctx.Bot, ctx.ChatID, "Ошибка: "+truncate(err.Error(), 3500))
				return
			}
			sendOvpnProfilesList(ctx, profiles, "ovpn:a:p:")
			return
		case "all":
			setOvpnAdminScope(ctx.TgID, wgAdminScope{Mode: wgScopeAll})
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
		setOvpnAdminScope(ctx.TgID, wgAdminScope{Mode: wgScopeUser, UserName: name})
		profiles, err := getOvpnProfilesForScope(ctx, wgAdminScope{Mode: wgScopeUser, UserName: name})
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Ошибка: "+truncate(err.Error(), 3500))
			return
		}
		sendOvpnProfilesList(ctx, profiles, "ovpn:a:p:")
		return
	}
	if data == "ovpn:a:back" {
		clearOvpnAdminScope(ctx.TgID)
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
		if act == "add" {
			if err := validateOvpnAdminAddAccess(ctx); err != nil {
				reply(ctx.Bot, ctx.ChatID, "Действие недоступно: "+err.Error())
				return
			}
		} else {
			if _, err := validateOvpnAdminProfileAccess(ctx, client, profileName); err != nil {
				reply(ctx.Bot, ctx.ChatID, "Действие недоступно: "+err.Error())
				return
			}
		}
		switch act {
		case "stats":
			sendOvpnStatsForProfile(ctx, client, profileName)
		case "score":
			sendDNSGuardRiskScore(ctx, "ovpn", profileName, true)
		case "conf":
			sendOvpnConfigAsFile(ctx, client, profileName)
		case "add":
			setOvpnPending(ctx.TgID, ovpnPending{Kind: ovpnPendingAdd})
			reply(ctx.Bot, ctx.ChatID, "Отправь имя нового OpenVPN сертификата одним сообщением. Без пробелов. Срок действия будет 8250 дней.")
		case "restart":
			sendOvpnAdminRestart(ctx, client, profileName)
		case "restart_container":
			sendOvpnAdminRestartContainer(ctx, client, profileName)
		case "revoke":
			sendOvpnAdminCertificateAction(ctx, client, profileName, "revoke")
		case "burn":
			sendOvpnAdminCertificateAction(ctx, client, profileName, "burn")
		default:
			reply(ctx.Bot, ctx.ChatID, "Неизвестное действие.")
		}
		return
	}

	reply(ctx.Bot, ctx.ChatID, "Неизвестный callback.")
}

func sendOvpnAdminCertificateAction(ctx *Ctx, client *ovpnUIClient, profileName, action string) {
	profile, err := validateOvpnAdminProfileAccess(ctx, client, profileName)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Действие недоступно: "+err.Error())
		return
	}

	var (
		actionURL string
		okText    string
	)
	switch action {
	case "revoke":
		actionURL = profile.RevokeURL
		okText = "Сертификат OpenVPN отозван. Для немедленного разрыва активной сессии может потребоваться restart OpenVPN."
	case "burn":
		actionURL = profile.BurnURL
		okText = "Отозванный сертификат OpenVPN удален."
	default:
		reply(ctx.Bot, ctx.ChatID, "Неизвестное действие.")
		return
	}

	if strings.TrimSpace(actionURL) == "" {
		reply(ctx.Bot, ctx.ChatID, "Для выбранного профиля это действие сейчас недоступно.")
		return
	}
	if err := client.executeProfileAction(actionURL); err != nil {
		reply(ctx.Bot, ctx.ChatID, "Не удалось выполнить действие OpenVPN:\n"+truncate(err.Error(), 3500))
		return
	}

	reply(ctx.Bot, ctx.ChatID, okText)
	sendOvpnProfileActionsWithStats(ctx, profileName, "ovpn:a")
}

func sendOvpnAdminRestart(ctx *Ctx, client *ovpnUIClient, profileName string) {
	if _, err := validateOvpnAdminProfileAccess(ctx, client, profileName); err != nil {
		reply(ctx.Bot, ctx.ChatID, "Действие недоступно: "+err.Error())
		return
	}
	if err := client.restartServer("SIGUSR1"); err != nil {
		reply(ctx.Bot, ctx.ChatID, "Не удалось перезапустить OpenVPN server:\n"+truncate(err.Error(), 3500))
		return
	}
	reply(ctx.Bot, ctx.ChatID, "OpenVPN server restarted (SIGUSR1). Активные клиентские сессии должны быть переинициализированы.")
	sendOvpnProfileActionsWithStats(ctx, profileName, "ovpn:a")
}

func sendOvpnAdminRestartContainer(ctx *Ctx, client *ovpnUIClient, profileName string) {
	profile, err := validateOvpnAdminProfileAccess(ctx, client, profileName)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Действие недоступно: "+err.Error())
		return
	}
	if strings.TrimSpace(profile.RestartContainerURL) == "" {
		reply(ctx.Bot, ctx.ChatID, "Restart container недоступен в текущем OpenVPN UI.")
		return
	}
	if err := client.executeProfileAction(profile.RestartContainerURL); err != nil {
		reply(ctx.Bot, ctx.ChatID, "Не удалось перезапустить OpenVPN container:\n"+truncate(err.Error(), 3500))
		return
	}
	reply(ctx.Bot, ctx.ChatID, "OpenVPN container restart triggered.")
}
