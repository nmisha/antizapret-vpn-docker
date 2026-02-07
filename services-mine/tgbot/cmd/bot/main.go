package main

import (
	"log"

	"tgbot/internal/bot"
)

func main() {
	if err := bot.Run(); err != nil {
		log.Fatal(err)
	}
}
