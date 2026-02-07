package main

import (
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// sendTextChunks sends a potentially long text in multiple messages, staying below Telegram limits.
// Uses a conservative maxLen to avoid edge-case failures.
func sendTextChunks(bot *tgbotapi.BotAPI, chatID int64, text string) {
	const maxLen = 3500
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	for len(text) > 0 {
		chunk := text
		if len(chunk) > maxLen {
			chunk = chunk[:maxLen]
			// try to split on newline boundary
			if i := strings.LastIndex(chunk, "\n"); i > 500 {
				chunk = chunk[:i]
			}
		}
		m := tgbotapi.NewMessage(chatID, chunk)
		m.DisableWebPagePreview = true
		_, err := bot.Send(m)
		logSendErrorIfEnabled(chatID, err, "send message", chunk)
		text = strings.TrimSpace(text[len(chunk):])
	}
}
