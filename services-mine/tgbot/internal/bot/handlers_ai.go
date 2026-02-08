package bot

import (
	"fmt"
	"strconv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	accCbPrefix = "acc:"
	accCbCancel = "acc:cancel"
)

func RegisterAIHandlers(reg *CommandRegistry) {
	reg.Command(CommandSpec{Cmd: "/accounts", Desc: "UI: получить учётные записи", Section: "Аккаунты", NeedAny: []Role{RoleAiUser}}, handleAccounts,
		LogCommand(),
		WithTyping(),
		RequireRole(RoleAiUser, "Недостаточно прав. Нужна роль AiUser (или Admin)."),
	)
	reg.Alias("accounts", "/accounts")
}

func handleAccounts(ctx *Ctx, _ string) {
	// Мягкий UI: если не личка — даём кнопку открыть личку
	if !ctx.IsPrivate {
		replyWithOpenDM(ctx)
		return
	}

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

func replyWithOpenDM(ctx *Ctx) {
	username := ctx.Bot.Self.UserName
	if username == "" {
		reply(ctx.Bot, ctx.ChatID, "Выдача учётных данных доступна только в личных сообщениях с ботом.")
		return
	}

	// deep-link: откроет личку и может стартануть сценарий
	url := fmt.Sprintf("https://t.me/%s?start=accounts", username)

	kb := tgbotapi.NewInlineKeyboardMarkup(
		[]tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonURL("💬 Открыть личку", url),
		},
		[]tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("✖ Отмена", "ui:cancel"),
		},
	)

	msg := tgbotapi.NewMessage(ctx.ChatID, "Учётные данные выдаются **только в личке** с ботом.\nНажми кнопку ниже:")
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = kb
	_, err := ctx.Bot.Send(msg)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", msg.Text)
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
	if data == accCbCancel {
		reply(bot, ctx.ChatID, "Ок, отменил.")
		return
	}

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
	text := fmt.Sprintf(
		"Учётная запись: %s\nЛогин: %s\nПароль: %s",
		a.Name, a.Login, a.Password,
	)
	reply(bot, ctx.ChatID, text)
}
