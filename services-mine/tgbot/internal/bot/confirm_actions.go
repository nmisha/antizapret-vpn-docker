package bot

import (
	"fmt"
	"net/url"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Generic confirmation callbacks for destructive actions.
//
// Callback format: "confirm:<kind>:<payload>"
// where payload is URL-escaped and can contain ':' separators.
const confirmCbPrefix = "confirm:"

func sendConfirm(ctx *Ctx, prompt string, kind string, payload string) {
	if kind == "domain:del" {
		handleConfirmCallback(ctx, confirmCbPrefix+kind+":"+url.QueryEscape(payload))
		return
	}

	cb := confirmCbPrefix + kind + ":" + url.QueryEscape(payload)
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Delete", cb),
			tgbotapi.NewInlineKeyboardButtonData("Cancel", "ui:cancel"),
		),
	)
	m := tgbotapi.NewMessage(ctx.ChatID, prompt)
	m.ReplyMarkup = kb
	_, err := ctx.Bot.Send(m)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", prompt)
}

func handleConfirmCallback(ctx *Ctx, data string) bool {
	if !strings.HasPrefix(data, confirmCbPrefix) {
		return false
	}

	rest := strings.TrimPrefix(data, confirmCbPrefix)
	idx := strings.LastIndex(rest, ":")
	if idx <= 0 || idx >= len(rest)-1 {
		reply(ctx.Bot, ctx.ChatID, "Некорректный запрос подтверждения.")
		return true
	}

	kind := rest[:idx]
	payloadEsc := rest[idx+1:]
	payload, _ := url.QueryUnescape(payloadEsc)

	switch kind {
	case "wg:delete":
		peerID := strings.TrimSpace(payload)
		if !ctx.User.Has(RoleAdmin) {
			reply(ctx.Bot, ctx.ChatID, "Недостаточно прав. Нужна роль Admin.")
			return true
		}
		client, err := makeWgClientFromEnv()
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, err.Error())
			return true
		}
		if _, err := validateWgAdminPeerAccess(ctx, client, peerID); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Действие недоступно: "+err.Error())
			return true
		}
		if err := client.deleteClient(peerID); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Не удалось удалить:\n"+truncate(err.Error(), 3500))
			return true
		}
		reply(ctx.Bot, ctx.ChatID, "OK: deleted")
		return true

	case "awg:delete":
		peerID := strings.TrimSpace(payload)
		if !ctx.User.Has(RoleAdmin) {
			reply(ctx.Bot, ctx.ChatID, "Недостаточно прав. Нужна роль Admin.")
			return true
		}
		client, err := makeAwgClientFromEnv()
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, err.Error())
			return true
		}
		if _, err := validateAwgAdminPeerAccess(ctx, peerID); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Действие недоступно: "+err.Error())
			return true
		}
		if err := client.deleteClient(peerID); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Не удалось удалить:\n"+truncate(err.Error(), 3500))
			return true
		}
		reply(ctx.Bot, ctx.ChatID, "OK: deleted")
		return true

	case "domain:del":
		d := strings.TrimSpace(payload)
		if d == "" {
			reply(ctx.Bot, ctx.ChatID, "Пустой домен.")
			return true
		}
		if ctx.User.Has(RoleDomainManager) || ctx.User.Has(RoleAdmin) {
			sections, err := ctx.Domains.DelDomainAny(d)
			if err != nil {
				reply(ctx.Bot, ctx.ChatID, "Ошибка сохранения: "+err.Error())
				return true
			}
			if len(sections) == 0 {
				reply(ctx.Bot, ctx.ChatID, "Не найдено ни в одной секции: "+d)
				return true
			}
			if len(sections) == 1 {
				reply(ctx.Bot, ctx.ChatID, "Удалено из секции #"+sections[0]+": "+d)
				return true
			}
			reply(ctx.Bot, ctx.ChatID, "Удалено из секций #"+strings.Join(sections, ", #")+": "+d)
			return true
		}

		removed, err := ctx.Domains.DelDomain(ctx.User.Name, d)
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Ошибка сохранения: "+err.Error())
			return true
		}
		if !removed {
			reply(ctx.Bot, ctx.ChatID, "Не найдено в твоей секции: "+d)
			return true
		}
		reply(ctx.Bot, ctx.ChatID, "Удалено из секции #"+ctx.User.Name+": "+d)
		return true

	case "acc:del":
		name := strings.TrimSpace(payload)
		if !ctx.User.Has(RoleAdmin) {
			reply(ctx.Bot, ctx.ChatID, "Недостаточно прав. Нужна роль Admin.")
			return true
		}
		msg, err := ctx.Accounts.DeleteByName(name)
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
			return true
		}
		reply(ctx.Bot, ctx.ChatID, msg)
		return true

	case "wgn:delete":
		peerID := strings.TrimSpace(payload)
		if !ctx.User.Has(RoleAdmin) {
			reply(ctx.Bot, ctx.ChatID, "Недостаточно прав. Нужна роль Admin.")
			return true
		}
		client, err := makeWgClientFromEnv()
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, err.Error())
			return true
		}
		if err := client.deleteClient(peerID); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Не удалось удалить:\n"+truncate(err.Error(), 3500))
			return true
		}
		reply(ctx.Bot, ctx.ChatID, "OK: deleted")
		return true

	case "awgn:delete":
		peerID := strings.TrimSpace(payload)
		if !ctx.User.Has(RoleAdmin) {
			reply(ctx.Bot, ctx.ChatID, "Недостаточно прав. Нужна роль Admin.")
			return true
		}
		client, err := makeAwgClientFromEnv()
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, err.Error())
			return true
		}
		if err := client.deleteClient(peerID); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Не удалось удалить:\n"+truncate(err.Error(), 3500))
			return true
		}
		reply(ctx.Bot, ctx.ChatID, "OK: deleted")
		return true

	default:
		reply(ctx.Bot, ctx.ChatID, fmt.Sprintf("Неизвестное подтверждение: %s", kind))
		return true
	}
}
