package bot

import (
	"strings"
)

func handleWgCallback(ctx *Ctx, data string) {
	if !ctx.IsPrivate {
		reply(ctx.Bot, ctx.ChatID, "WireGuard доступен только в личных сообщениях боту.")
		return
	}

	// USER FLOW:
	// wg:u:p:<id>                 -> show actions
	// wg:u:act:stats:<id>         -> stats
	// wg:u:act:conf:<id>          -> configuration
	// wg:u:act:qr:<id>            -> qrcode

	if strings.HasPrefix(data, "wg:u:p:") {
		peerID := strings.TrimPrefix(data, "wg:u:p:")
		client, err := makeWgClientFromEnv()
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, err.Error())
			return
		}
		if _, err := validateWgUserPeerAccess(ctx, client, peerID); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Доступ к профилю недоступен: "+err.Error())
			return
		}
		sendUserProfileActions(ctx, peerID)
		return
	}
	if strings.HasPrefix(data, "wg:u:act:") {
		rest := strings.TrimPrefix(data, "wg:u:act:")
		parts := strings.SplitN(rest, ":", 2)
		if len(parts) != 2 {
			reply(ctx.Bot, ctx.ChatID, "Неверный формат.")
			return
		}
		act, peerID := parts[0], parts[1]
		client, err := makeWgClientFromEnv()
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, err.Error())
			return
		}
		if _, err := validateWgUserPeerAccess(ctx, client, peerID); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Доступ к профилю недоступен: "+err.Error())
			return
		}
		switch act {
		case "stats":
			sendWgStatsForPeerID(ctx, client, peerID)
		case "score":
			peer, err := validateWgUserPeerAccess(ctx, client, peerID)
			if err != nil {
				reply(ctx.Bot, ctx.ChatID, "Доступ к профилю недоступен: "+err.Error())
				return
			}
			sendDNSGuardRiskScore(ctx, "wg", peer.Name, false)
		case "conf":
			sendWgConfigAsFile(ctx, client, peerID)
		case "qr":
			sendWgQRCode(ctx, client, peerID)
		case "migrate_awg":
			if !ctx.User.Has(RoleAwgUserControl) {
				reply(ctx.Bot, ctx.ChatID, "Недостаточно прав. Нужна роль AwgUserControl.")
				return
			}
			sendConfirm(ctx, "Мигрировать профиль в AWG? Целевой профиль с тем же именем будет удалён и пересоздан, исходный WG-профиль будет удалён.", "wg:u:migrate:awg", peerID)
		default:
			reply(ctx.Bot, ctx.ChatID, "Неизвестное действие.")
		}
		return
	}

	// ADMIN FLOW:
	// wg:a:scope:my|user|all
	// wg:a:user:<name>
	// wg:a:p:<id>
	// wg:a:act:<action>:<id>
	// wg:a:act:add:
	// wg:a:back

	if strings.HasPrefix(data, "wg:a:scope:") {
		scope := strings.TrimPrefix(data, "wg:a:scope:")
		switch scope {
		case "my":
			setAdminScope(ctx.TgID, wgAdminScope{Mode: wgScopeMy})
			peers, err := getPeersForScope(ctx, wgAdminScope{Mode: wgScopeMy})
			if err != nil {
				reply(ctx.Bot, ctx.ChatID, "Ошибка: "+truncate(err.Error(), 3500))
				return
			}
			sendWgProfilesList(ctx, peers, "wg:a:p:")
			return
		case "all":
			setAdminScope(ctx.TgID, wgAdminScope{Mode: wgScopeAll})
			peers, err := getPeersForScope(ctx, wgAdminScope{Mode: wgScopeAll})
			if err != nil {
				reply(ctx.Bot, ctx.ChatID, "Ошибка: "+truncate(err.Error(), 3500))
				return
			}
			sendWgProfilesList(ctx, peers, "wg:a:p:")
			return
		case "user":
			sendWgUsersList(ctx)
			return
		default:
			reply(ctx.Bot, ctx.ChatID, "Неверный scope.")
			return
		}
	}

	if strings.HasPrefix(data, "wg:a:user:") {
		name := strings.TrimPrefix(data, "wg:a:user:")
		name = strings.ToLower(strings.TrimSpace(name))
		setAdminScope(ctx.TgID, wgAdminScope{Mode: wgScopeUser, UserName: name})

		peers, err := getPeersForScope(ctx, wgAdminScope{Mode: wgScopeUser, UserName: name})
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Ошибка: "+truncate(err.Error(), 3500))
			return
		}
		sendWgProfilesList(ctx, peers, "wg:a:p:")
		return
	}

	if data == "wg:a:back" {
		// return to scope selection
		clearAdminScope(ctx.TgID)
		handleWgProfilesAdmin(ctx, "")
		return
	}

	if strings.HasPrefix(data, "wg:a:p:") {
		peerID := strings.TrimPrefix(data, "wg:a:p:")
		client, err := makeWgClientFromEnv()
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, err.Error())
			return
		}
		if _, err := validateWgAdminPeerAccess(ctx, client, peerID); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Действие недоступно: "+err.Error())
			return
		}
		sendAdminProfileActions(ctx, peerID)
		return
	}

	if strings.HasPrefix(data, "wg:a:act:") {
		rest := strings.TrimPrefix(data, "wg:a:act:")
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

		client, err := makeWgClientFromEnv()
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, err.Error())
			return
		}
		if act == "add" {
			if err := validateWgAdminAddAccess(ctx); err != nil {
				reply(ctx.Bot, ctx.ChatID, "Действие недоступно: "+err.Error())
				return
			}
		} else {
			if _, err := validateWgAdminPeerAccess(ctx, client, peerID); err != nil {
				reply(ctx.Bot, ctx.ChatID, "Действие недоступно: "+err.Error())
				return
			}
		}

		switch act {
		case "stats":
			sendWgStatsForPeerID(ctx, client, peerID)
		case "score":
			peer, err := validateWgAdminPeerAccess(ctx, client, peerID)
			if err != nil {
				reply(ctx.Bot, ctx.ChatID, "Действие недоступно: "+err.Error())
				return
			}
			sendDNSGuardRiskScore(ctx, "wg", peer.Name, true)
		case "reset_score":
			peer, err := validateWgAdminPeerAccess(ctx, client, peerID)
			if err != nil {
				reply(ctx.Bot, ctx.ChatID, "Р”РµР№СЃС‚РІРёРµ РЅРµРґРѕСЃС‚СѓРїРЅРѕ: "+err.Error())
				return
			}
			sendConfirm(ctx, "СЃР±СЂРѕСЃРёС‚СЊ risk score Рё state РґР»СЏ WireGuard РїСЂРѕС„РёР»СЏ "+peer.Name+"?", "wg:reset_score", peer.Name)
		case "conf":
			sendWgConfigAsFile(ctx, client, peerID)
		case "qr":
			sendWgQRCode(ctx, client, peerID)
		case "migrate_awg":
			if !ctx.User.Has(RoleAwgUserControl) {
				reply(ctx.Bot, ctx.ChatID, "Недостаточно прав. Нужна роль AwgUserControl.")
				return
			}
			sendConfirm(ctx, "Мигрировать профиль в AWG? Целевой профиль с тем же именем будет удалён и пересоздан, исходный WG-профиль будет удалён.", "wg:a:migrate:awg", peerID)
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
			// Confirm deletion
			sendConfirm(ctx, "Удалить WireGuard профиль (id="+peerID+")?", "wg:delete", peerID)
		case "rename":
			setWgPending(ctx.TgID, wgPending{Kind: wgPendingRename, PeerID: peerID})
			reply(ctx.Bot, ctx.ChatID, "Отправь новое имя профиля одним сообщением.")
		case "add":
			setWgPending(ctx.TgID, wgPending{Kind: wgPendingAdd})
			reply(ctx.Bot, ctx.ChatID, "Отправь имя нового профиля одним сообщением.")
		default:
			reply(ctx.Bot, ctx.ChatID, "Неизвестное действие.")
		}
		return
	}

	reply(ctx.Bot, ctx.ChatID, "Неизвестный callback.")
}
