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

	// Ensure users.json exists (create empty array if missing)
	if err := ensureJSONFile(usersFile, "[]\n"); err != nil {
		log.Fatal(err)
	}

	// settings and logger live next to users.json
	gSettings = NewSettingsStore(usersFile)
	if err := gSettings.Ensure(); err != nil {
		log.Fatal(err)
	}
	loadSettingsIntoCache()
	gLogger = NewBotLogger(usersFile)

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

	accountsFile := os.Getenv("ACCOUNTS_FILE")
	if accountsFile == "" {
		accountsFile = "./accounts.json"
	}
	if err := ensureJSONFile(accountsFile, "[]\n"); err != nil {
		log.Fatal(err)
	}
	accountsStore := NewAccountsStore(accountsFile)
	log.Printf("Accounts file: %s", accountsFile)

	router := NewRouter()
	RegisterAdminHandlers(router)
	RegisterAdminAccountsHandlers(router)
	RegisterAdminAccountsUIHandlers(router)
	RegisterAdminSettingsHandlers(router)
	RegisterDomainHandlers(router)
	RegisterServiceHandlers(router)
	RegisterWgProfilesHandlers(router)
	RegisterWgNameCommands(router)
	RegisterNetHandlers(router)
	RegisterSupportHandlers(router)
	RegisterAIHandlers(router)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.CallbackQuery != nil {
			//			handleCallback(bot, usersStore, store, update.CallbackQuery)
			handleCallback(bot, usersStore, store, accountsStore, update.CallbackQuery)

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

		// log incoming
		if gLogger != nil {
			s := getSettingsCached()
			if s.LoggingEnabled {
				gLogger.Append(formatLogLine("IN", chatID, text))
			}
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

		isPriv := update.Message.Chat != nil && update.Message.Chat.IsPrivate()
		chatType := ""
		chatTitle := ""
		if update.Message.Chat != nil {
			chatType = update.Message.Chat.Type
			chatTitle = update.Message.Chat.Title
		}

		// global disable for non-admin
		s := getSettingsCached()
		if !s.BotEnabledForUsers && !user.Has(RoleAdmin) {
			reply(bot, chatID, "Бот временно отключён.")
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
			Accounts:   accountsStore,
			IsPrivate:  isPriv,
			ChatType:   chatType,
			ChatTitle:  chatTitle,
		}

		// pending wg admin actions (rename/add)
		if handleWgPendingIfAny(ctx, text) {
			continue
		}

		// pending admin accounts ui actions
		if handleAccAdminPendingIfAny(ctx, text) {
			continue
		}

		// soft UI wizards (support + admin messaging)
		if handleConversationStateIfAny(ctx, text, cmd, arg) {
			continue
		}

		if handled := router.Dispatch(ctx, cmd, arg); !handled {
			reply(bot, chatID, "Не понял команду. /help")
		}
	}
}
