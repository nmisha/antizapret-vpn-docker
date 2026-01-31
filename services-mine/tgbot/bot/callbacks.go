package main

import (
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func handleCallback(bot *tgbotapi.BotAPI, usersStore *UsersStore, store *Store, accounts *AccountsStore, q *tgbotapi.CallbackQuery) {
	ack := tgbotapi.NewCallback(q.ID, "")
	_, _ = bot.Request(ack)

	if q.From == nil || q.Message == nil {
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

	ctx := &Ctx{
		Bot:        bot,
		ChatID:     chatID,
		TgID:       tgID,
		User:       user,
		UsersStore: usersStore,
		Domains:    store,
		Accounts:   accounts,
	}

	data := q.Data

	// accounts callbacks
	if strings.HasPrefix(data, accCbPrefix) || data == accCbCancel {
		// только личка
		if q.Message == nil || q.Message.Chat == nil || !q.Message.Chat.IsPrivate() {
			reply(bot, chatID, "Выдача учётных данных доступна только в личных сообщениях с ботом.")
			return
		}

		if !user.Has(RoleAiUser) { // Admin пройдёт из-за Has()
			reply(bot, chatID, "Недостаточно прав. Нужна роль AiUser (или Admin).")
			return
		}
		if accounts == nil {
			reply(bot, chatID, "AccountsStore не настроен.")
			return
		}

		// важно: ctx.IsPrivate=true (для единообразия)
		ctx.IsPrivate = true

		handleAccountsCallback(bot, ctx, data)
		return
	}

	// domains pending callbacks (add:...)
	handleDomainPendingCallback(bot, ctx, q)
}
