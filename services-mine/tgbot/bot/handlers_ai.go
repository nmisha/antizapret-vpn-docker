package main

import (
	"fmt"
	"strconv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	accCbPrefix = "acc:"
	accCbCancel = "acc:cancel"
	// acc:0, acc:1, ...
)

func RegisterAIHandlers(r *Router) {
	r.Handle("/accounts", handleAccounts,
		LogCommand(),
		WithTyping(),
		RequirePrivateChat("Команда доступна только в личных сообщениях с ботом."),
		RequireRole(RoleAiUser, "Недостаточно прав. Нужна роль AiUser (или Admin)."),
	)
	r.Alias("accounts", "/accounts")
}


func handleAccounts(ctx *Ctx, _ string) {
	if ctx.Accounts == nil {
		reply(ctx.Bot, ctx.ChatID, "AccountsStore не настроен.")
		return
	}
	list, err := ctx.Accounts.List()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка чтения accounts файла: "+err.Error())
		return
	}
	if len(list) == 0 {
		reply(ctx.Bot, ctx.ChatID, "Список учётных записей пуст.")
		return
	}

	kb := buildAccountsKeyboard(list)
	msg := tgbotapi.NewMessage(ctx.ChatID, "Выбери учётную запись:")
	msg.ReplyMarkup = kb
	_, err = ctx.Bot.Send(msg)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка отправки: "+err.Error())
		return
	}
}

func buildAccountsKeyboard(list []Account) tgbotapi.InlineKeyboardMarkup {
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(list)+1)

	for i, a := range list {
		btn := tgbotapi.NewInlineKeyboardButtonData(a.Name, accCbPrefix+strconv.Itoa(i))
		rows = append(rows, []tgbotapi.InlineKeyboardButton{btn})
	}

	rows = append(rows, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("✖ Отмена", accCbCancel),
	})

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func handleAccountsCallback(bot *tgbotapi.BotAPI, ctx *Ctx, data string) {
	// cancel
	if data == accCbCancel {
		reply(bot, ctx.ChatID, "Ок, отменил.")
		return
	}

	// acc:<index>
	idxStr := data[len(accCbPrefix):]
	idx, err := strconv.Atoi(idxStr)
	if err != nil || idx < 0 {
		reply(bot, ctx.ChatID, "Некорректный выбор.")
		return
	}

	list, err := ctx.Accounts.List()
	if err != nil {
		reply(bot, ctx.ChatID, "Ошибка чтения accounts файла: "+err.Error())
		return
	}
	if idx >= len(list) {
		reply(bot, ctx.ChatID, "Выбор устарел. Повтори /accounts.")
		return
	}

	a := list[idx]
	// ВНИМАНИЕ: пароль отправляется в чат.
	text := fmt.Sprintf(
		"Учётная запись: %s\nЛогин: %s\nПароль: %s",
		a.Name, a.Login, a.Password,
	)
	reply(bot, ctx.ChatID, text)
}
