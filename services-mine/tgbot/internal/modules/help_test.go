package modules

import (
	"testing"

	"tgbot/internal/bot"
)

// Every section of the real command set belongs to a /help group and every screen fits a message.
func TestHelpCoversAllCommands(t *testing.T) {
	t.Setenv("TELEMT_TOKEN", "tok")
	reg := bot.NewCommandRegistry(bot.NewRouter())
	RegisterAll(reg)
	if p := bot.HelpProblems(reg); len(p) != 0 {
		t.Fatalf("help problems: %v", p)
	}
}
