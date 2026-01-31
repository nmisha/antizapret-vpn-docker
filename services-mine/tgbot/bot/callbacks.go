package main

import (
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func handleCallback(bot *tgbotapi.BotAPI, usersStore *UsersStore, store *Store, q *tgbotapi.CallbackQuery) {
	ack := tgbotapi.NewCallback(q.ID, "")
	_, _ = bot.Request(ack)

	if q.From == nil {
		return
	}
	chatID := q.Message.Chat.ID
	tgID := q.From.ID

	user, ok, err := usersStore.GetByID(tgID)
	if err != nil {
		reply(bot, chatID, "Ошибка чтения users.json: "+err.Error())
		return
	}
	if !ok {
		reply(bot, chatID, randomJoke())
		return
	}

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
		if !user.Has(RoleDomainEditor) {
			editMessage(bot, chatID, p.MessageID, "Недостаточно прав. Нужна роль DomainEditor (или Admin).", nil)
			clearPending(tgID)
			return
		}
		canReplace := (p.ParentSection == user.Name) || user.Has(RoleDomainManager)
		if !canReplace {
			editMessage(bot, chatID, p.MessageID, "Нельзя: покрывающий домен в чужой секции, а роли DomainManager/Admin нет.", nil)
			return
		}
		if err := store.ReplaceDomain(p.ParentSection, p.ParentDomain, p.TargetSection, p.Candidate); err != nil {
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
		if !user.Has(RoleDomainManager) {
			editMessage(bot, chatID, p.MessageID, "Недостаточно прав. Нужна роль DomainManager (или Admin).", nil)
			return
		}
		added, err := store.AddDomain(p.TargetSection, p.Candidate)
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

func buildAddDecisionKeyboard(canReplace, canSub bool) tgbotapi.InlineKeyboardMarkup {
	var row []tgbotapi.InlineKeyboardButton
	if canReplace {
		row = append(row, tgbotapi.NewInlineKeyboardButtonData("➕ Заменить (удалить домен)", cbAddReplace))
	}
	if canSub {
		row = append(row, tgbotapi.NewInlineKeyboardButtonData("➕ Добавить поддомен (не удалять)", cbAddSub))
	}
	cancelRow := []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("✖ Отмена", cbAddCancel),
	}
	kb := [][]tgbotapi.InlineKeyboardButton{}
	if len(row) > 0 {
		kb = append(kb, row)
	}
	kb = append(kb, cancelRow)
	return tgbotapi.NewInlineKeyboardMarkup(kb...)
}
