package bot

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

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

func logSendErrorIfEnabled(chatID int64, err error, op string, payload string) {
	if err == nil || gLogger == nil {
		return
	}
	s := getSettingsCached()
	if !s.LogErrorsEnabled {
		return
	}
	user := getChatUserLabel(chatID)
	msg := op + ": " + err.Error()
	if payload != "" {
		msg += " | " + payload
	}
	gLogger.Append(formatLogLine("SEND_ERR", chatID, user, truncate(msg, 2000)))
}
func reply(bot *tgbotapi.BotAPI, chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.DisableWebPagePreview = true
	_, err := bot.Send(msg)
	logSendErrorIfEnabled(chatID, err, "send message", text)
}

func replyHTML(bot *tgbotapi.BotAPI, chatID int64, htmlText string) {
	msg := tgbotapi.NewMessage(chatID, htmlText)
	msg.DisableWebPagePreview = true
	msg.ParseMode = "HTML"
	_, err := bot.Send(msg)
	logSendErrorIfEnabled(chatID, err, "send message", htmlText)
}

func replyHTMLChunks(bot *tgbotapi.BotAPI, chatID int64, htmlText string) {
	const maxLen = 3500
	htmlText = strings.TrimSpace(htmlText)
	if htmlText == "" {
		return
	}
	for len(htmlText) > 0 {
		chunk := htmlText
		if len(chunk) > maxLen {
			chunk = chunk[:maxLen]
			if i := strings.LastIndex(chunk, "\n"); i > 500 {
				chunk = chunk[:i]
			}
		}
		msg := tgbotapi.NewMessage(chatID, strings.TrimSpace(chunk))
		msg.DisableWebPagePreview = true
		msg.ParseMode = "HTML"
		_, err := bot.Send(msg)
		logSendErrorIfEnabled(chatID, err, "send message", chunk)
		htmlText = strings.TrimSpace(htmlText[len(chunk):])
	}
}

func editMessage(bot *tgbotapi.BotAPI, chatID int64, messageID int, text string, markup *tgbotapi.InlineKeyboardMarkup) {
	edit := tgbotapi.NewEditMessageText(chatID, messageID, text)
	edit.ParseMode = "Markdown"
	if markup != nil {
		edit.ReplyMarkup = markup
	}
	_, err := bot.Send(edit)
	logSendErrorIfEnabled(chatID, err, "edit message", text)
}

func splitCmd(s string) (cmd, arg string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	cmd = s
	arg = ""
	for i, r := range s {
		if unicode.IsSpace(r) {
			cmd = s[:i]
			j := i
			for j < len(s) {
				rr, size := utf8.DecodeRuneInString(s[j:])
				if !unicode.IsSpace(rr) {
					break
				}
				j += size
			}
			arg = s[j:]
			break
		}
	}
	if strings.Contains(cmd, "@") {
		cmd = strings.SplitN(cmd, "@", 2)[0]
	}
	return cmd, arg
}

func splitCmdMeta(s string) (cmd, arg string, argStartUTF16 int) {
	cmd, arg = splitCmd(s)
	if arg == "" {
		return cmd, "", utf16Len(s)
	}
	i := strings.Index(s, arg)
	if i < 0 {
		return cmd, arg, utf16Len(s)
	}
	return cmd, arg, utf16Len(s[:i])
}

func utf16Len(s string) int {
	return len(utf16.Encode([]rune(s)))
}

func sliceEntitiesForSuffix(entities []tgbotapi.MessageEntity, startUTF16 int) []tgbotapi.MessageEntity {
	if len(entities) == 0 {
		return nil
	}
	out := make([]tgbotapi.MessageEntity, 0, len(entities))
	for _, e := range entities {
		es := e.Offset
		ee := e.Offset + e.Length
		if ee <= startUTF16 {
			continue
		}
		ne := e
		if es < startUTF16 {
			ne.Offset = 0
			ne.Length = ee - startUTF16
		} else {
			ne.Offset = es - startUTF16
		}
		if ne.Length > 0 {
			out = append(out, ne)
		}
	}
	return out
}

func prependAndShiftEntities(prefix, text string, entities []tgbotapi.MessageEntity) (string, []tgbotapi.MessageEntity) {
	body := prefix + text
	if len(entities) == 0 {
		return body, nil
	}
	shift := utf16Len(prefix)
	out := make([]tgbotapi.MessageEntity, 0, len(entities))
	for _, e := range entities {
		ne := e
		ne.Offset += shift
		out = append(out, ne)
	}
	return body, out
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
