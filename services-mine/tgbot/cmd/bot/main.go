package main

import (
	"log"

	"tgbot/internal/bot"
	"tgbot/internal/modules"
)

func main() {
	if err := bot.Run(modules.RegisterAll); err != nil {
		log.Fatal(err)
	}
}
