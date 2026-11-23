package bot

import "strings"

func handleOvpnNameCallback(ctx *Ctx, data string) {
	if !ctx.IsPrivate {
		reply(ctx.Bot, ctx.ChatID, "OpenVPN доступен только в личных сообщениях боту.")
		return
	}
	if !ctx.User.Has(RoleAdmin) {
		reply(ctx.Bot, ctx.ChatID, "Недостаточно прав. Нужна роль Admin.")
		return
	}

	rest := strings.TrimPrefix(data, ovpnNameCbPrefix)
	parts := strings.SplitN(rest, ":", 2)
	if len(parts) != 2 {
		reply(ctx.Bot, ctx.ChatID, "Неверный формат.")
		return
	}
	action := parts[0]
	profileName := parts[1]

	client, err := newOvpnUIClientFromEnv()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, err.Error())
		return
	}
	performOvpnProfileAction(ctx, client, action, profileName)
}
