package bot

import (
	"fmt"
	"html"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Admin Accounts UI callbacks prefix
const accAdminCbPrefix = "accm:" // accounts management

// callback data helpers
func accm(action string, parts ...string) string {
	// accm:<action>:<p1>:<p2>...
	b := strings.Builder{}
	b.WriteString(accAdminCbPrefix)
	b.WriteString(action)
	for _, p := range parts {
		b.WriteString(":")
		// keep simple; do not allow ':' inside parts
		b.WriteString(strings.ReplaceAll(p, ":", ""))
	}
	return b.String()
}

func RegisterAdminAccountsUIHandlers(reg *CommandRegistry) {
	reg.Command(CommandSpec{Cmd: "/accounts_admin", Desc: "UI: управление учётными записями", Section: "Админ: аккаунты", NeedAny: []Role{RoleAdmin}}, handleAccountsAdminOpen,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequirePrivate("Управление учётными записями доступно только в личных сообщениях с ботом."),
	)
	reg.Alias("accounts_admin", "/accounts_admin")
}

func handleAccountsAdminOpen(ctx *Ctx, _ string) {
	showAccountsAdminList(ctx)
}

func showAccountsAdminList(ctx *Ctx) {
	list, err := ctx.Accounts.List()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка чтения accounts.json: "+err.Error())
		return
	}
	if len(list) == 0 {
		kb := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("➕ Add", accm("add")),
				tgbotapi.NewInlineKeyboardButtonData("✖ Close", "ui:cancel"),
			),
		)
		msg := tgbotapi.NewMessage(ctx.ChatID, "Учётные записи: (пусто)")
		msg.ReplyMarkup = kb
		_, err := ctx.Bot.Send(msg)
		logSendErrorIfEnabled(ctx.ChatID, err, "send message", msg.Text)
		return
	}

	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(list)+2)
	for _, a := range list {
		title := a.Name
		if a.Login != "" {
			title = fmt.Sprintf("%s (%s)", a.Name, a.Login)
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(title, accm("open", a.Name)),
		))
	}
	rows = append(rows,
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➕ Add", accm("add")),
			tgbotapi.NewInlineKeyboardButtonData("✖ Close", "ui:cancel"),
		),
	)

	kb := tgbotapi.NewInlineKeyboardMarkup(rows...)
	msg := tgbotapi.NewMessage(ctx.ChatID, "Учётные записи:")
	msg.ReplyMarkup = kb
	_, err = ctx.Bot.Send(msg)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", msg.Text)
}

func showAccountsAdminItem(ctx *Ctx, name string) {
	// Find account (case-insensitive exact name)
	list, err := ctx.Accounts.List()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка чтения accounts.json: "+err.Error())
		return
	}
	var acc *Account
	key := strings.ToLower(strings.TrimSpace(name))
	for i := range list {
		if strings.ToLower(list[i].Name) == key {
			acc = &list[i]
			break
		}
	}
	if acc == nil {
		reply(ctx.Bot, ctx.ChatID, "Учётная запись не найдена: "+name)
		showAccountsAdminList(ctx)
		return
	}

	// Mask password length only
	passMask := ""
	if acc.Password == "" {
		passMask = "(empty)"
	} else {
		passMask = fmt.Sprintf("(%d chars)", len(acc.Password))
	}

	text := fmt.Sprintf("<b>Account</b>: %s\nLogin: %s\nPassword: %s", html.EscapeString(acc.Name), html.EscapeString(acc.Login), html.EscapeString(passMask))
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👁 Show 30s", accm("show30", acc.Name)),
			tgbotapi.NewInlineKeyboardButtonData("✏ Rename", accm("rename", acc.Name)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✏ Set login", accm("setlogin", acc.Name)),
			tgbotapi.NewInlineKeyboardButtonData("✏ Set password", accm("setpass", acc.Name)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🗑 Delete", accm("del", acc.Name)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("← Back", accm("back")),
		),
	)
	msg := tgbotapi.NewMessage(ctx.ChatID, text)
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = kb
	_, err = ctx.Bot.Send(msg)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", msg.Text)
}

// handleAccountsAdminCallback routes inline actions.
func handleAccountsAdminCallback(ctx *Ctx, data string) {
	// data: accm:<action>[:...]
	if !strings.HasPrefix(data, accAdminCbPrefix) {
		return
	}
	parts := strings.Split(data[len(accAdminCbPrefix):], ":")
	if len(parts) == 0 {
		return
	}
	action := parts[0]
	arg1 := ""
	if len(parts) > 1 {
		arg1 = parts[1]
	}

	switch action {
	case "back":
		showAccountsAdminList(ctx)
	case "open":
		showAccountsAdminItem(ctx, arg1)
	case "add":
		setAccAdminPending(ctx.TgID, accAdminPending{Kind: accAdminAdd})
		reply(ctx.Bot, ctx.ChatID, "Отправь данные для новой учётной записи в формате:\n<name>;<login>;<password>")
	case "rename":
		if arg1 == "" {
			reply(ctx.Bot, ctx.ChatID, "Не выбрана учётная запись.")
			return
		}
		setAccAdminPending(ctx.TgID, accAdminPending{Kind: accAdminRename, Name: arg1})
		reply(ctx.Bot, ctx.ChatID, "Новое имя для \""+arg1+"\":")
	case "setlogin":
		if arg1 == "" {
			reply(ctx.Bot, ctx.ChatID, "Не выбрана учётная запись.")
			return
		}
		setAccAdminPending(ctx.TgID, accAdminPending{Kind: accAdminSetLogin, Name: arg1})
		reply(ctx.Bot, ctx.ChatID, "Новый login для \""+arg1+"\":")
	case "setpass":
		if arg1 == "" {
			reply(ctx.Bot, ctx.ChatID, "Не выбрана учётная запись.")
			return
		}
		setAccAdminPending(ctx.TgID, accAdminPending{Kind: accAdminSetPass, Name: arg1})
		reply(ctx.Bot, ctx.ChatID, "Новый password для \""+arg1+"\":")
	case "del":
		if arg1 == "" {
			reply(ctx.Bot, ctx.ChatID, "Не выбрана учётная запись.")
			return
		}
		// Ask for confirmation
		kb := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✅ Да, удалить", accm("del_confirm", arg1)),
				tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", accm("open", arg1)),
			),
		)
		msg := tgbotapi.NewMessage(ctx.ChatID, "Удалить учётную запись \""+arg1+"\"?")
		msg.ReplyMarkup = kb
		_, err := ctx.Bot.Send(msg)
		logSendErrorIfEnabled(ctx.ChatID, err, "send message", msg.Text)
	case "del_confirm":
		if arg1 == "" {
			reply(ctx.Bot, ctx.ChatID, "Не выбрана учётная запись.")
			return
		}
		out, err := ctx.Accounts.DeleteByName(arg1)
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
			return
		}
		reply(ctx.Bot, ctx.ChatID, out)
		showAccountsAdminList(ctx)
	case "show30":
		// show real password (admin) in private chat
		name := arg1
		list, err := ctx.Accounts.List()
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Ошибка чтения accounts.json: "+err.Error())
			return
		}
		key := strings.ToLower(strings.TrimSpace(name))
		for _, a := range list {
			if strings.ToLower(a.Name) == key {
				// Send sensitive data as plain text; still in private
				m := tgbotapi.NewMessage(ctx.ChatID, fmt.Sprintf("%s\nlogin: %s\npassword: %s\n\n⏳ Это сообщение будет удалено через 30 секунд.", a.Name, a.Login, a.Password))
				sent, err := ctx.Bot.Send(m)
				if err == nil {
					chatID := ctx.ChatID
					msgID := sent.MessageID
					go func() {
						time.Sleep(30 * time.Second)
						del := tgbotapi.DeleteMessageConfig{ChatID: chatID, MessageID: msgID}
						_, _ = ctx.Bot.Request(del)
					}()
				}
				return
			}
		}
		reply(ctx.Bot, ctx.ChatID, "Учётная запись не найдена: "+name)
	default:
		reply(ctx.Bot, ctx.ChatID, "Неизвестное действие.")
	}
}
