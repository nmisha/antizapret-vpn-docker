package bot

import (
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func handleDomainPendingCallback(bot *tgbotapi.BotAPI, ctx *Ctx, q *tgbotapi.CallbackQuery) {
	chatID := ctx.ChatID
	tgID := ctx.TgID

	p, ok := getPending(tgID)
	if !ok {
		reply(bot, chatID, "Нет ожидающего добавления. Используй /add <domain>.")
		return
	}
	if q.Message != nil && (q.Message.MessageID != p.MessageID || q.Message.Chat.ID != p.ChatID) {
		reply(bot, chatID, "Эта кнопка уже устарела. Повтори /add <domain>.")
		return
	}

	switch q.Data {
	case cbAddCancel:
		clearPending(tgID)
		editMessage(bot, chatID, p.MessageID, "Ок, отменил.", nil)

	case cbAddReplace:
		if !ctx.User.Has(RoleDomainEditor) {
			editMessage(bot, chatID, p.MessageID, "Недостаточно прав. Нужна роль DomainEditor (или Admin).", nil)
			clearPending(tgID)
			return
		}
		canReplace := (p.ParentSection == ctx.User.Name) || ctx.User.Has(RoleDomainManager)
		if !canReplace {
			editMessage(bot, chatID, p.MessageID, "Нельзя: покрывающий домен в чужой секции, а роли DomainManager/Admin нет.", nil)
			return
		}
		if err := ctx.Domains.ReplaceDomain(p.ParentSection, p.ParentDomain, p.TargetSection, p.Candidate); err != nil {
			editMessage(bot, chatID, p.MessageID, "Ошибка сохранения: "+err.Error(), nil)
			return
		}
		clearPending(tgID)
		editMessage(bot, chatID, p.MessageID,
			fmt.Sprintf("OK: удалил *%s* из #%s и добавил *%s* в #%s",
				p.ParentDomain, p.ParentSection, p.Candidate, p.TargetSection),
			nil,
		)

	case cbAddSub:
		if !ctx.User.Has(RoleDomainManager) {
			editMessage(bot, chatID, p.MessageID, "Недостаточно прав. Нужна роль DomainManager (или Admin).", nil)
			return
		}
		added, err := ctx.Domains.AddDomain(p.TargetSection, p.Candidate)
		if err != nil {
			editMessage(bot, chatID, p.MessageID, "Ошибка сохранения: "+err.Error(), nil)
			return
		}
		clearPending(tgID)
		if !added {
			editMessage(bot, chatID, p.MessageID, "Уже есть в твоей секции: "+p.Candidate, nil)
			return
		}
		editMessage(bot, chatID, p.MessageID,
			fmt.Sprintf("OK: добавил поддомен *%s* в #%s (домен *%s* в #%s не трогал)",
				p.Candidate, p.TargetSection, p.ParentDomain, p.ParentSection),
			nil,
		)

	default:
		reply(bot, chatID, "Неизвестное действие.")
	}
}
