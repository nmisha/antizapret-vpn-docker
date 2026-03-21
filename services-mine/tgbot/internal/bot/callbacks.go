package bot

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
	fromLabel := tgUserLabel(q.From)
	setChatUserLabel(chatID, fromLabel)

	user, ok, err := usersStore.GetByID(tgID)
	if err != nil {
		reply(bot, chatID, "Ошибка чтения users.json: "+err.Error())
		return
	}
	if !ok {
		reply(bot, chatID, randomJoke())
		return
	}

	label := strings.TrimSpace(fromLabel)
	if user.Name != "" {
		if label != "" {
			label = label + "/" + user.Name
		} else {
			label = user.Name
		}
	}
	setChatUserLabel(chatID, label)

	s := getSettingsCached()
	if !s.BotEnabledForUsers && !user.Has(RoleAdmin) {
		reply(bot, chatID, "Бот временно отключён.")
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
		FromUser:   fromLabel,
	}

	data := q.Data

	// log callback as incoming
	if gLogger != nil {
		s := getSettingsCached()
		if s.LogCommandsEnabled {
			gLogger.Append(formatLogLine("IN", chatID, fromLabel, "callback:"+data))
		}
	}

	if data == "ui:cancel" {
		// cancel any active wizard state for this chat/user
		clearConv(chatID, tgID)
		reply(bot, chatID, "Ок, отменил.")
		return
	}

	// confirmation callbacks (delete, etc.)
	if strings.HasPrefix(data, confirmCbPrefix) {
		if handleConfirmCallback(ctx, data) {
			return
		}
	}

	// admin messaging wizard callbacks
	if strings.HasPrefix(data, adminMsgCbPrefix) {
		if !user.Has(RoleAdmin) {
			reply(bot, chatID, "Недостаточно прав. Нужна роль Admin.")
			return
		}
		handleAdminMessagingCallback(ctx, data)
		return
	}

	// accounts callbacks (строго только личка)
	if strings.HasPrefix(data, accCbPrefix) || data == accCbCancel {
		if q.Message == nil || q.Message.Chat == nil || !q.Message.Chat.IsPrivate() {
			reply(bot, chatID, "Учётные данные выдаются только в личных сообщениях с ботом.")
			return
		}
		if !user.Has(RoleAiUser) { // Admin пройдёт, т.к. Has() true
			reply(bot, chatID, "Недостаточно прав. Нужна роль AiUser (или Admin).")
			return
		}
		if accounts == nil {
			reply(bot, chatID, "AccountsStore не настроен.")
			return
		}
		ctx.IsPrivate = true
		handleAccountsCallback(bot, ctx, data)
		return
	}

	// accounts admin ui callbacks (строго только личка)
	if strings.HasPrefix(data, accAdminCbPrefix) {
		if q.Message == nil || q.Message.Chat == nil || !q.Message.Chat.IsPrivate() {
			reply(bot, chatID, "Управление учётными записями доступно только в личных сообщениях с ботом.")
			return
		}
		if !user.Has(RoleAdmin) {
			reply(bot, chatID, "Недостаточно прав. Нужна роль Admin.")
			return
		}
		if accounts == nil {
			reply(bot, chatID, "AccountsStore не настроен.")
			return
		}
		ctx.IsPrivate = true
		handleAccountsAdminCallback(ctx, data)
		return
	}

	// wg name-based disambiguation callbacks
	if strings.HasPrefix(data, wgNameCbPrefix) {
		ctx.IsPrivate = q.Message.Chat != nil && q.Message.Chat.IsPrivate()
		handleWgNameCallback(ctx, data)
		return
	}

	// wg profiles callbacks
	if strings.HasPrefix(data, wgCbPrefix) {
		ctx.IsPrivate = q.Message.Chat != nil && q.Message.Chat.IsPrivate()
		handleWgCallback(ctx, data)
		return
	}

	if strings.HasPrefix(data, ovpnCbPrefix) {
		ctx.IsPrivate = q.Message.Chat != nil && q.Message.Chat.IsPrivate()
		handleOvpnCallback(ctx, data)
		return
	}

	// domains pending callbacks (add:...)
	handleDomainPendingCallback(bot, ctx, q)
}
