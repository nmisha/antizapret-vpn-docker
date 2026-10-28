package bot

import "strings"

func handleOvpnPendingIfAny(ctx *Ctx, text string) bool {
	// Pending actions originate in the private chat; ignore input from other chats.
	if !ctx.IsPrivate {
		return false
	}
	p, ok := popOvpnPending(ctx.TgID)
	if !ok {
		return false
	}
	if !ctx.User.Has(RoleAdmin) {
		return false
	}

	input := strings.TrimSpace(text)
	if strings.EqualFold(input, "/cancel") {
		reply(ctx.Bot, ctx.ChatID, "Ок, отменил.")
		return true
	}
	if input == "" {
		reply(ctx.Bot, ctx.ChatID, "Пустое значение, отменено.")
		return true
	}

	switch p.Kind {
	case ovpnPendingAdd:
		if err := validateOvpnAdminAddAccess(ctx); err != nil {
			replyServiceError(ctx, "OpenVPN", "Действие недоступно", err)
			return true
		}
		client, err := newOvpnUIClientFromEnv()
		if err != nil {
			replyServiceError(ctx, "OpenVPN", "Действие недоступно", err)
			return true
		}
		if err := client.createProfile(input); err != nil {
			replyServiceError(ctx, "OpenVPN", "Не удалось создать сертификат", err)
			return true
		}
		reply(ctx.Bot, ctx.ChatID, "OK: OpenVPN certificate created")
		return true
	default:
		reply(ctx.Bot, ctx.ChatID, "Неизвестное ожидаемое действие.")
		return true
	}
}
