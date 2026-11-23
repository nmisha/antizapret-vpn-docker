package bot

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Run starts the Telegram bot and blocks processing updates.
// register is a required callback that must register all commands via CommandRegistry.
func Run(register func(reg *CommandRegistry)) error {
	rand.Seed(time.Now().UnixNano())

	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return fmt.Errorf("set TELEGRAM_BOT_TOKEN")
	}

	usersFile := os.Getenv("USERS_FILE")
	if usersFile == "" {
		usersFile = "./users.json"
	}
	usersStore := NewUsersStore(usersFile)

	// Ensure users.json exists (create empty array if missing)
	if err := ensureJSONFile(usersFile, "[]\n"); err != nil {
		return err
	}

	// settings and logger live next to users.json
	gSettings = NewSettingsStore(usersFile)
	if err := gSettings.Ensure(); err != nil {
		return err
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
		return err
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
		return err
	}
	accountsStore := NewAccountsStore(accountsFile)
	log.Printf("Accounts file: %s", accountsFile)
	startRiskNotificationWorker(bot, usersStore)

	router := NewRouter()
	reg := NewCommandRegistry(router)
	gCmdRegistry = reg
	if register == nil {
		return fmt.Errorf("register callback is nil")
	}
	register(reg)

	ValidateHelpCoverage(reg)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	h := &updateHandler{bot: bot, usersStore: usersStore, store: store, accountsStore: accountsStore, router: router}
	d := newUpdateDispatcher(updateMaxParallel, updateQueueLen, updateIdleTimeout, h.handleUpdate)
	for update := range updates {
		d.submit(update)
	}

	return nil
}

// updateHandler carries what one update needs; the dispatcher calls handleUpdate concurrently
// for different senders.
type updateHandler struct {
	bot           *tgbotapi.BotAPI
	usersStore    *UsersStore
	store         *Store
	accountsStore *AccountsStore
	router        *Router
}

func (h *updateHandler) handleUpdate(update tgbotapi.Update) {
	bot, usersStore, store, accountsStore, router := h.bot, h.usersStore, h.store, h.accountsStore, h.router

	if update.CallbackQuery != nil {
		//			handleCallback(bot, usersStore, store, update.CallbackQuery)
		handleCallback(bot, usersStore, store, accountsStore, update.CallbackQuery)

		return
	}

	if update.Message == nil || update.Message.From == nil {
		return
	}

	chatID := update.Message.Chat.ID
	tgID := update.Message.From.ID
	fromLabel := tgUserLabel(update.Message.From)
	setChatUserLabel(chatID, fromLabel)

	text := Trim(update.Message.Text)
	if text == "" {
		return
	}

	// log incoming
	if gLogger != nil {
		s := getSettingsCached()
		if s.LogCommandsEnabled {
			gLogger.Append(formatLogLine("IN", chatID, fromLabel, text))
		}
	}

	user, ok, err := usersStore.GetByID(tgID)
	if err != nil {
		// Cannot tell users from strangers: answer like to a stranger and keep details in the log.
		log.Printf("users.json read failed (tg_id=%d): %v", tgID, err)
		reply(bot, chatID, randomJoke())
		return
	}
	if !ok || user.Disabled {
		// A disabled user looks like a stranger, but admins already know them: no join request.
		reply(bot, chatID, randomJoke())
		if !ok {
			notifyAdminsAboutStranger(bot, usersStore, update.Message)
		}
		return
	}

	// enrich chat label with user record name (if present)
	label := strings.TrimSpace(fromLabel)
	if user.Name != "" {
		if label != "" {
			label = label + "/" + user.Name
		} else {
			label = user.Name
		}
	}
	setChatUserLabel(chatID, label)

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
		return
	}

	cmd, arg, argStartUTF16 := splitCmdMeta(text)
	argEntities := sliceEntitiesForSuffix(update.Message.Entities, argStartUTF16)

	ctx := &Ctx{
		Bot:             bot,
		ChatID:          chatID,
		TgID:            tgID,
		User:            user,
		UsersStore:      usersStore,
		Domains:         store,
		Accounts:        accountsStore,
		IsPrivate:       isPriv,
		ChatType:        chatType,
		ChatTitle:       chatTitle,
		FromUser:        fromLabel,
		MessageText:     text,
		MessageEntities: update.Message.Entities,
		ArgEntities:     argEntities,
	}

	// A new command leaves the previous private input flow; /cancel is answered here.
	if resetPrivateFlowsOnCommand(ctx, text, cmd) {
		return
	}

	// pending wg admin actions (rename/add)
	if handleWgPendingIfAny(ctx, text) {
		return
	}

	// pending awg admin actions (rename/add)
	if handleAwgPendingIfAny(ctx, text) {
		return
	}

	// pending ovpn admin actions (create certificate)
	if handleOvpnPendingIfAny(ctx, text) {
		return
	}

	// pending mtproto admin actions (create proxy, set limit)
	if handleMtpPendingIfAny(ctx, text) {
		return
	}

	// pending users admin ui actions (add, rename, prefix)
	if handleUsrPendingIfAny(ctx, text) {
		return
	}

	// pending admin accounts ui actions
	if handleAccAdminPendingIfAny(ctx, text) {
		return
	}

	// soft UI wizards (support + admin messaging)
	if handleConversationStateIfAny(ctx, text, cmd, arg) {
		return
	}

	if handled := router.Dispatch(ctx, cmd, arg); !handled {
		reply(bot, chatID, "Не понял команду. /help")
	}
}
