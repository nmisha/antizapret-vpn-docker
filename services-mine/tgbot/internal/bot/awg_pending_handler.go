package bot

import "strings"

func handleAwgPendingIfAny(ctx *Ctx, text string) bool {
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
		reply(ctx.Bot, ctx.ChatID, err.Error())
		return true
	}

	switch p.Kind {
	case wgPendingRename:
		if p.PeerID == "" {
			reply(ctx.Bot, ctx.ChatID, "Ошибка: не задан профиль.")
			return true
		}
		if _, err := validateAwgAdminPeerAccess(ctx, p.PeerID); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Действие недоступно: "+err.Error())
			return true
		}
		if err := client.renameClient(p.PeerID, input); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Не удалось переименовать:\n"+truncate(err.Error(), 3500))
			return true
		}
		reply(ctx.Bot, ctx.ChatID, "OK: renamed")
		return true
	case wgPendingAdd:
		if err := validateAwgAdminAddAccess(ctx); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Действие недоступно: "+err.Error())
			return true
		}
		if err := client.createClient(input); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Не удалось добавить профиль:\n"+truncate(err.Error(), 3500))
			return true
		}
		reply(ctx.Bot, ctx.ChatID, "OK: created")
		return true
	default:
		reply(ctx.Bot, ctx.ChatID, "Неизвестное ожидаемое действие.")
		return true
	}
}
