package bot

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

type Ctx struct {
	Bot        *tgbotapi.BotAPI
	ChatID     int64
	TgID       int64
	User       User
	UsersStore *UsersStore
	Domains    *Store
	Accounts   *AccountsStore

	// chat meta
	IsPrivate bool
	ChatType  string
	ChatTitle string

	// telegram sender label (from telegram, may be empty)
	FromUser string

	// middleware/runtime
	Cmd    string
	Fields []string
}
