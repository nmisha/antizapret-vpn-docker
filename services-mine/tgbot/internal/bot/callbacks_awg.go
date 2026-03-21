package bot

import "strings"

func handleAwgCallback(ctx *Ctx, data string) {
	if !ctx.IsPrivate {
		reply(ctx.Bot, ctx.ChatID, "Amnezia WireGuard доступен только в личных сообщениях боту.")
		return
	}

	if strings.HasPrefix(data, "awg:u:p:") {
		peerID := strings.TrimPrefix(data, "awg:u:p:")
		client, err := makeAwgClientFromEnv()
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, err.Error())
			return
		}
		if _, err := validateAwgUserPeerAccess(ctx, client, peerID); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Доступ к профилю недоступен: "+err.Error())
			return
		}
		sendAwgUserProfileActions(ctx, peerID)
		return
	}
	if strings.HasPrefix(data, "awg:u:act:") {
		rest := strings.TrimPrefix(data, "awg:u:act:")
		parts := strings.SplitN(rest, ":", 2)
		if len(parts) != 2 {
			reply(ctx.Bot, ctx.ChatID, "Неверный формат.")
			return
		}
		act, peerID := parts[0], parts[1]
		client, err := makeAwgClientFromEnv()
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, err.Error())
			return
		}
		if _, err := validateAwgUserPeerAccess(ctx, client, peerID); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Доступ к профилю недоступен: "+err.Error())
			return
		}
		switch act {
		case "stats":
			sendAwgStatsForPeerID(ctx, client, peerID)
		case "conf":
			sendAwgConfigAsFile(ctx, client, peerID)
		case "qr":
			sendAwgQRCode(ctx, client, peerID)
		default:
			reply(ctx.Bot, ctx.ChatID, "Неизвестное действие.")
		}
		return
	}

	if strings.HasPrefix(data, "awg:a:scope:") {
		scope := strings.TrimPrefix(data, "awg:a:scope:")
		switch scope {
		case "my":
			setAwgAdminScope(ctx.TgID, wgAdminScope{Mode: wgScopeMy})
			peers, err := getAwgPeersForScope(ctx, wgAdminScope{Mode: wgScopeMy})
			if err != nil {
				reply(ctx.Bot, ctx.ChatID, "Ошибка: "+truncate(err.Error(), 3500))
				return
			}
			sendAwgProfilesList(ctx, peers, "awg:a:p:")
			return
		case "all":
			setAwgAdminScope(ctx.TgID, wgAdminScope{Mode: wgScopeAll})
			peers, err := getAwgPeersForScope(ctx, wgAdminScope{Mode: wgScopeAll})
			if err != nil {
				reply(ctx.Bot, ctx.ChatID, "Ошибка: "+truncate(err.Error(), 3500))
				return
			}
			sendAwgProfilesList(ctx, peers, "awg:a:p:")
			return
		case "user":
			sendAwgUsersList(ctx)
			return
		default:
			reply(ctx.Bot, ctx.ChatID, "Неверный scope.")
			return
		}
	}

	if strings.HasPrefix(data, "awg:a:user:") {
		name := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(data, "awg:a:user:")))
		setAwgAdminScope(ctx.TgID, wgAdminScope{Mode: wgScopeUser, UserName: name})
		peers, err := getAwgPeersForScope(ctx, wgAdminScope{Mode: wgScopeUser, UserName: name})
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Ошибка: "+truncate(err.Error(), 3500))
			return
		}
		sendAwgProfilesList(ctx, peers, "awg:a:p:")
		return
	}

	if data == "awg:a:back" {
		clearAwgAdminScope(ctx.TgID)
		handleAwgProfilesAdmin(ctx, "")
		return
	}

	if strings.HasPrefix(data, "awg:a:p:") {
		peerID := strings.TrimPrefix(data, "awg:a:p:")
		if _, err := validateAwgAdminPeerAccess(ctx, peerID); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Действие недоступно: "+err.Error())
			return
		}
		sendAwgAdminProfileActions(ctx, peerID)
		return
	}

	if strings.HasPrefix(data, "awg:a:act:") {
		rest := strings.TrimPrefix(data, "awg:a:act:")
		parts := strings.SplitN(rest, ":", 2)
		if len(parts) < 1 {
			reply(ctx.Bot, ctx.ChatID, "Неверный формат.")
			return
		}
		act := parts[0]
		peerID := ""
		if len(parts) == 2 {
			peerID = parts[1]
		}
		client, err := makeAwgClientFromEnv()
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, err.Error())
			return
		}
		if act == "add" {
			if err := validateAwgAdminAddAccess(ctx); err != nil {
				reply(ctx.Bot, ctx.ChatID, "Действие недоступно: "+err.Error())
				return
			}
		} else {
			if _, err := validateAwgAdminPeerAccess(ctx, peerID); err != nil {
				reply(ctx.Bot, ctx.ChatID, "Действие недоступно: "+err.Error())
				return
			}
		}
		switch act {
		case "stats":
			sendAwgStatsForPeerID(ctx, client, peerID)
		case "conf":
			sendAwgConfigAsFile(ctx, client, peerID)
		case "qr":
			sendAwgQRCode(ctx, client, peerID)
		case "enable":
			if err := client.enableClient(peerID); err != nil {
				reply(ctx.Bot, ctx.ChatID, "Не удалось включить:\n"+truncate(err.Error(), 3500))
				return
			}
			reply(ctx.Bot, ctx.ChatID, "OK: enabled")
		case "disable":
			if err := client.disableClient(peerID); err != nil {
				reply(ctx.Bot, ctx.ChatID, "Не удалось выключить:\n"+truncate(err.Error(), 3500))
				return
			}
			reply(ctx.Bot, ctx.ChatID, "OK: disabled")
		case "delete":
			sendConfirm(ctx, "Удалить Amnezia WireGuard профиль (id="+peerID+")?", "awg:delete", peerID)
		case "rename":
			setAwgPending(ctx.TgID, wgPending{Kind: wgPendingRename, PeerID: peerID})
			reply(ctx.Bot, ctx.ChatID, "Отправь новое имя профиля одним сообщением.")
		case "add":
			setAwgPending(ctx.TgID, wgPending{Kind: wgPendingAdd})
			reply(ctx.Bot, ctx.ChatID, "Отправь имя нового профиля одним сообщением.")
		default:
			reply(ctx.Bot, ctx.ChatID, "Неизвестное действие.")
		}
		return
	}

	reply(ctx.Bot, ctx.ChatID, "Неизвестный callback.")
}
