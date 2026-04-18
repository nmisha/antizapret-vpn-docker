package bot

import "strings"

func handleOvpnPendingIfAny(ctx *Ctx, text string) bool {
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
			reply(ctx.Bot, ctx.ChatID, "Действие недоступно: "+err.Error())
			return true
		}
		client, err := newOvpnUIClientFromEnv()
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, err.Error())
			return true
		}
		if err := client.createProfile(input); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Не удалось создать OpenVPN сертификат:\n"+truncate(err.Error(), 3500))
			return true
		}
		reply(ctx.Bot, ctx.ChatID, "OK: OpenVPN certificate created")
		return true
	default:
		reply(ctx.Bot, ctx.ChatID, "Неизвестное ожидаемое действие.")
		return true
	}
}
