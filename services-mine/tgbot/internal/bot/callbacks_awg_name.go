package bot

import (
	"net/url"
	"strings"
)

func handleAwgNameCallback(ctx *Ctx, data string) {
	if !ctx.IsPrivate {
		reply(ctx.Bot, ctx.ChatID, "Amnezia WireGuard доступен только в личных сообщениях боту.")
		return
	}
	if !ctx.User.Has(RoleAdmin) {
		reply(ctx.Bot, ctx.ChatID, "Недостаточно прав. Нужна роль Admin.")
		return
	}

	rest := strings.TrimPrefix(data, awgNameCbPrefix)
	parts := strings.SplitN(rest, ":", 3)
	if len(parts) < 2 {
		reply(ctx.Bot, ctx.ChatID, "Неверный формат.")
		return
	}
	action := parts[0]
	peerID := parts[1]
	extra := ""
	if len(parts) == 3 {
		extra, _ = url.QueryUnescape(parts[2])
	}

	client, err := makeAwgClientFromEnv()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, err.Error())
		return
	}
	performAwgPeerAction(ctx, client, action, peerID, extra)
}
