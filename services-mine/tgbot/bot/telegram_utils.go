package main

import (
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func formatLogLine(dir string, chatID int64, user string, text string) string {
	text = strings.ReplaceAll(text, "\n", "\\n")
	text = truncate(text, 900)
	user = strings.TrimSpace(user)
	if user != "" {
		return fmt.Sprintf("%s %s chat=%d user=%s text=%s", time.Now().Format("2006-01-02 15:04:05"), dir, chatID, user, text)
	}
	return fmt.Sprintf("%s %s chat=%d text=%s", time.Now().Format("2006-01-02 15:04:05"), dir, chatID, text)
}

func tgUserLabel(u *tgbotapi.User) string {
	if u == nil {
		return ""
	}
	if u.UserName != "" {
		return "@" + u.UserName
	}
	name := strings.TrimSpace(strings.TrimSpace(u.FirstName + " " + u.LastName))
	if name != "" {
		return name
	}
	return "id=" + fmt.Sprint(u.ID)
}

func reply(bot *tgbotapi.BotAPI, chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.DisableWebPagePreview = true
	_, _ = bot.Send(msg)
}

func replyHTML(bot *tgbotapi.BotAPI, chatID int64, htmlText string) {
	msg := tgbotapi.NewMessage(chatID, htmlText)
	msg.DisableWebPagePreview = true
	msg.ParseMode = "HTML"
	_, _ = bot.Send(msg)
}

func editMessage(bot *tgbotapi.BotAPI, chatID int64, messageID int, text string, markup *tgbotapi.InlineKeyboardMarkup) {
	edit := tgbotapi.NewEditMessageText(chatID, messageID, text)
	edit.ParseMode = "Markdown"
	if markup != nil {
		edit.ReplyMarkup = markup
	}
	_, _ = bot.Send(edit)
}

func splitCmd(s string) (cmd, arg string) {
	s = strings.TrimSpace(s)
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return "", ""
	}
	cmd = parts[0]
	if strings.Contains(cmd, "@") {
		cmd = strings.SplitN(cmd, "@", 2)[0]
	}
	if len(parts) > 1 {
		arg = strings.Join(parts[1:], " ")
		arg = strings.TrimSpace(arg)
	}
	return cmd, arg
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n…(truncated)"
}

func formatSection(section string, domains []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", section)
	for _, d := range domains {
		b.WriteString(d)
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "\n(%d домен(ов), %s)", len(domains), time.Now().Format("2006-01-02 15:04:05"))
	return b.String()
}

func formatUsers(list []User) string {
	var b strings.Builder
	b.WriteString("Пользователи:\n")
	for _, u := range list {
		roles := strings.Join(uniqueStringsCaseInsensitive(u.RolesRaw), ", ")
		if roles == "" {
			roles = "(нет)"
		}
		fmt.Fprintf(&b, "• %s — tg_id=%d — %s\n", u.Name, u.TelegramID, roles)
	}
	return b.String()
}

func Trim(s string) string { return strings.TrimSpace(s) }
