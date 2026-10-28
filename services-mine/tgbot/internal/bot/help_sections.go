package bot

// Sections of /help. Use these constants in CommandSpec.Section: a typo in a literal
// silently creates a separate section.
const (
	secQuickStart = "Быстрый старт"

	secWg      = "WireGuard"
	secAdminWg = "Админ: WireGuard"

	secAwg      = "Amnezia WireGuard"
	secAdminAwg = "Админ: Amnezia WireGuard"

	secOvpn      = "OpenVPN"
	secAdminOvpn = "Админ: OpenVPN"

	secMtproto      = "MTProto"
	secAdminMtproto = "Админ: MTProto"

	secDomains       = "Домены"
	secDNSGuard      = "DNS Guard"
	secServices      = "Сервисы"
	secAdminServices = "Админ: сервисы"
	secNet           = "Сеть"

	secAccounts      = "Аккаунты"
	secAdminAccounts = "Админ: аккаунты"

	secAdminUsers    = "Админ: пользователи и роли"
	secAdminMessages = "Админ: сообщения"
	secAdminSettings = "Админ: настройки и логи"
)

// helpGroup is one screen of the sectioned /help: related sections, user and admin together.
type helpGroup struct {
	Key      string // /help <key>, callback help:<key>
	Title    string
	Sections []string
}

var helpGroups = []helpGroup{
	{Key: "start", Title: "Основное", Sections: []string{secQuickStart}},
	{Key: "wg", Title: "WireGuard", Sections: []string{secWg, secAdminWg}},
	{Key: "awg", Title: "Amnezia WireGuard", Sections: []string{secAwg, secAdminAwg}},
	{Key: "ovpn", Title: "OpenVPN", Sections: []string{secOvpn, secAdminOvpn}},
	{Key: "mtproto", Title: "MTProto", Sections: []string{secMtproto, secAdminMtproto}},
	{Key: "dns", Title: "Домены и DNS", Sections: []string{secDomains, secDNSGuard, secServices, secAdminServices, secNet}},
	{Key: "accounts", Title: "Аккаунты", Sections: []string{secAccounts, secAdminAccounts}},
	{Key: "users", Title: "Админ: пользователи", Sections: []string{secAdminUsers, secAdminMessages}},
	{Key: "bot", Title: "Админ: бот", Sections: []string{secAdminSettings}},
}
