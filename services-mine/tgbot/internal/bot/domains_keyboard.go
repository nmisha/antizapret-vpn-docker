package bot

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

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
