package main

import (
	"log"
	"math/rand"
	"os"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	rand.Seed(time.Now().UnixNano())

	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("set TELEGRAM_BOT_TOKEN")
	}

	usersFile := os.Getenv("USERS_FILE")
	if usersFile == "" {
		usersFile = "./users.json"
	}
	usersStore := NewUsersStore(usersFile)

	domainsPath := os.Getenv("DOMAINS_FILE")
	if domainsPath == "" {
		domainsPath = "./domains.txt"
	}
	store := NewStore(domainsPath)

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatal(err)
	}
	bot.Debug = false

	log.Printf("Bot authorized as @%s", bot.Self.UserName)
	log.Printf("Users file: %s", usersFile)
	log.Printf("Domains file: %s", domainsPath)

	router := NewRouter()
	RegisterAdminHandlers(router)
	RegisterDomainHandlers(router)
	RegisterServiceHandlers(router)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.CallbackQuery != nil {
			handleCallback(bot, usersStore, store, update.CallbackQuery)
			continue
		}

		if update.Message == nil || update.Message.From == nil {
			continue
		}

		chatID := update.Message.Chat.ID
		tgID := update.Message.From.ID

		text := Trim(update.Message.Text)
		if text == "" {
			continue
		}

		user, ok, err := usersStore.GetByID(tgID)
		if err != nil {
			reply(bot, chatID, "Ошибка чтения users.json: "+err.Error())
			continue
		}
		if !ok {
			reply(bot, chatID, randomJoke())
			continue
		}

		cmd, arg := splitCmd(text)

		ctx := &Ctx{
			Bot:        bot,
			ChatID:     chatID,
			TgID:       tgID,
			User:       user,
			UsersStore: usersStore,
			Domains:    store,
		}

		if handled := router.Dispatch(ctx, cmd, arg); !handled {
			reply(bot, chatID, "Не понял команду. /help")
		}
	}
}
