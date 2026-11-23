package bot

import "strings"

func handleAwgPendingIfAny(ctx *Ctx, text string) bool {
	// Pending actions originate in the private chat; ignore input from other chats.
	if !ctx.IsPrivate {
		return false
	}
	p, ok := popAwgPending(ctx.TgID)
	if !ok {
		return false
	}
	if !ctx.User.Has(RoleAdmin) {
		return false
	}

	input := strings.TrimSpace(text)
	if input == "" {
		reply(ctx.Bot, ctx.ChatID, "Пустое значение, отменено.")
		return true
	}

	client, err := makeAwgClientFromEnv()
	if err != nil {
		replyServiceError(ctx, "Amnezia WireGuard", "Действие недоступно", err)
		return true
	}

	switch p.Kind {
	case wgPendingRename:
		if p.PeerID == "" {
			reply(ctx.Bot, ctx.ChatID, "Ошибка: не задан профиль.")
			return true
		}
		if _, err := validateAwgAdminPeerAccess(ctx, p.PeerID); err != nil {
			replyServiceError(ctx, "Amnezia WireGuard", "Действие недоступно", err)
			return true
		}
		if err := client.renameClient(p.PeerID, input); err != nil {
			replyServiceError(ctx, "Amnezia WireGuard", "Не удалось переименовать", err)
			return true
		}
		reply(ctx.Bot, ctx.ChatID, "OK: renamed")
		return true
	case wgPendingAdd:
		if err := validateAwgAdminAddAccess(ctx); err != nil {
			replyServiceError(ctx, "Amnezia WireGuard", "Действие недоступно", err)
			return true
		}
		if err := client.createClient(input); err != nil {
			replyServiceError(ctx, "Amnezia WireGuard", "Не удалось добавить профиль", err)
			return true
		}
		reply(ctx.Bot, ctx.ChatID, "OK: created")
		return true
	default:
		reply(ctx.Bot, ctx.ChatID, "Неизвестное ожидаемое действие.")
		return true
	}
}
