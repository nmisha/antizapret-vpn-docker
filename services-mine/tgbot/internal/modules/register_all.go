package modules

import "tgbot/internal/bot"

// RegisterAll wires all feature modules into the bot command registry.
// Keep the order stable.
func RegisterAll(reg *bot.CommandRegistry) {
	bot.RegisterAdminHandlers(reg)
	bot.RegisterAdminAccountsHandlers(reg)
	bot.RegisterAdminAccountsUIHandlers(reg)
	bot.RegisterAdminSettingsHandlers(reg)
	bot.RegisterDomainHandlers(reg)
	bot.RegisterGuardNotifyHandlers(reg)
	bot.RegisterServiceHandlers(reg)
	bot.RegisterWgProfilesHandlers(reg)
	bot.RegisterAwgStatsHandlers(reg)
	bot.RegisterAwgProfilesHandlers(reg)
	bot.RegisterOvpnStatsHandlers(reg)
	bot.RegisterOvpnProfilesHandlers(reg)
	bot.RegisterWgNameCommands(reg)
	bot.RegisterAwgNameCommands(reg)
	bot.RegisterNetHandlers(reg)
	bot.RegisterSupportHandlers(reg)
	bot.RegisterAIHandlers(reg)
}
