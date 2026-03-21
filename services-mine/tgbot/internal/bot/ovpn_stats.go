package bot

import (
	"fmt"
	"html"
	"sort"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func RegisterOvpnStatsHandlers(reg *CommandRegistry) {
	reg.Command(CommandSpec{Cmd: "/ovpnstats", Desc: "статистика OpenVPN (legacy alias)", Section: "OpenVPN", NeedAny: []Role{RoleOvpnStats}, Hidden: true}, handleOvpnStats,
		RequireRole(RoleOvpnStats, "Недостаточно прав. Нужна роль OvpnStats (или Admin)."),
		RequirePrivateWithOpenDM("Статистика OpenVPN доступна только в личных сообщениях с ботом."),
	)
	reg.Alias("ovpnstats", "/ovpnstats")

	reg.Command(CommandSpec{Cmd: "/ovpn_stats", Desc: "статистика OpenVPN по своим профилям", Section: "OpenVPN", NeedAny: []Role{RoleOvpnStats}}, handleOvpnStats,
		RequireRole(RoleOvpnStats, "Недостаточно прав. Нужна роль OvpnStats (или Admin)."),
		RequirePrivateWithOpenDM("Статистика OpenVPN доступна только в личных сообщениях с ботом."),
	)
	reg.Alias("ovpn_stats", "/ovpn_stats")

	reg.Command(CommandSpec{Cmd: "/ovpnstats_admin", Desc: "статистика OpenVPN (legacy alias)", Section: "Админ: OpenVPN", NeedAny: []Role{RoleAdmin}, Hidden: true}, handleOvpnStatsAdmin,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequirePrivateWithOpenDM("Статистика OpenVPN доступна только в личных сообщениях с ботом."),
	)
	reg.Alias("ovpnstats_admin", "/ovpnstats_admin")

	reg.Command(CommandSpec{Cmd: "/ovpn_stats_admin", Desc: "статистика OpenVPN (admin)", Section: "Админ: OpenVPN", NeedAny: []Role{RoleAdmin}}, handleOvpnStatsAdmin,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequirePrivateWithOpenDM("Статистика OpenVPN доступна только в личных сообщениях с ботом."),
	)
	reg.Alias("ovpn_stats_admin", "/ovpn_stats_admin")
}

func handleOvpnStats(ctx *Ctx, arg string) {
	target := ctx.User
	if strings.TrimSpace(arg) != "" && ctx.User.Has(RoleAdmin) {
		u, ok, err := ctx.UsersStore.GetByName(arg)
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Ошибка чтения users.json: "+err.Error())
			return
		}
		if !ok {
			reply(ctx.Bot, ctx.ChatID, "Пользователь не найден: "+arg)
			return
		}
		target = u
	}

	client, err := newOvpnUIClientFromEnv()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, err.Error())
		return
	}
	sessions, err := client.listSessions()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Не удалось получить статистику OpenVPN:\n"+truncate(err.Error(), 3500))
		return
	}

	filtered := filterOvpnSessionsByUserPrefixes(sessions.ClientList, target.WgProfiles)
	if len(filtered) == 0 {
		reply(ctx.Bot, ctx.ChatID, "Не найдено ни одной OpenVPN-сессии по вашим правилам из users.json (wg_profiles).")
		return
	}
	replyHTML(ctx.Bot, ctx.ChatID, truncate(formatOvpnSessionsStats(filtered), 3800))
}

func handleOvpnStatsAdmin(ctx *Ctx, _ string) {
	client, err := newOvpnUIClientFromEnv()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, err.Error())
		return
	}
	sessions, err := client.listSessions()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Не удалось получить статистику OpenVPN:\n"+truncate(err.Error(), 3500))
		return
	}
	if len(sessions.ClientList) == 0 {
		reply(ctx.Bot, ctx.ChatID, "Сейчас нет активных OpenVPN-сессий.")
		return
	}
	replyHTML(ctx.Bot, ctx.ChatID, truncate(formatOvpnSessionsStats(sessions.ClientList), 3800))
}

func sendOvpnStatsForProfile(ctx *Ctx, client *ovpnUIClient, profileName string) {
	sessions, err := client.listSessions()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Не удалось получить статистику OpenVPN:\n"+truncate(err.Error(), 3500))
		return
	}
	filtered := filterOvpnSessionsByProfileName(sessions.ClientList, profileName)
	if len(filtered) == 0 {
		reply(ctx.Bot, ctx.ChatID, "Для профиля "+profileName+" нет активных OpenVPN-сессий.")
		return
	}
	replyHTML(ctx.Bot, ctx.ChatID, truncate(formatOvpnSessionsStats(filtered), 3800))
}

func filterOvpnSessionsByUserPrefixes(clients []ovpnSessionClient, allowed []string) []ovpnSessionClient {
	if len(allowed) == 0 {
		return nil
	}
	out := make([]ovpnSessionClient, 0, len(clients))
	for _, c := range clients {
		name := strings.ToLower(strings.TrimSpace(c.CommonName))
		for _, prefix := range allowed {
			pp := strings.ToLower(strings.TrimSpace(prefix))
			if pp != "" && strings.HasPrefix(name, pp) {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

func filterOvpnSessionsByProfileName(clients []ovpnSessionClient, profileName string) []ovpnSessionClient {
	out := make([]ovpnSessionClient, 0, len(clients))
	want := strings.ToLower(strings.TrimSpace(profileName))
	for _, c := range clients {
		if strings.ToLower(strings.TrimSpace(c.CommonName)) == want {
			out = append(out, c)
		}
	}
	return out
}

func formatOvpnSessionsStats(clients []ovpnSessionClient) string {
	sort.Slice(clients, func(i, j int) bool {
		li := strings.ToLower(strings.TrimSpace(clients[i].CommonName))
		lj := strings.ToLower(strings.TrimSpace(clients[j].CommonName))
		if li == lj {
			return clients[i].ConnectedSince > clients[j].ConnectedSince
		}
		return li < lj
	})

	lines := make([]string, 0, len(clients)*7)
	for i, c := range clients {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines,
			fmt.Sprintf("<b>Profile: %s</b>", html.EscapeString(strings.TrimSpace(c.CommonName))),
			fmt.Sprintf("Connected Since: %s", html.EscapeString(strings.TrimSpace(c.ConnectedSince))),
			fmt.Sprintf("Virtual IP: %s", html.EscapeString(strings.TrimSpace(c.VirtualAddress))),
			fmt.Sprintf("Real Address: %s", html.EscapeString(strings.TrimSpace(c.RealAddress))),
			fmt.Sprintf("TX: %s", humanBytesIEC(uint64ToInt64(c.BytesSent))),
			fmt.Sprintf("RX: %s", humanBytesIEC(uint64ToInt64(c.BytesReceived))),
		)
	}
	return strings.Join(lines, "\n")
}

func humanBytesIEC(v int64) string {
	if v < 0 {
		v = 0
	}
	const (
		KiB = 1024
		MiB = 1024 * KiB
		GiB = 1024 * MiB
	)
	switch {
	case v >= GiB:
		return fmt.Sprintf("%.2f ГБ", float64(v)/float64(GiB))
	case v >= MiB:
		return fmt.Sprintf("%.2f МБ", float64(v)/float64(MiB))
	case v >= KiB:
		return fmt.Sprintf("%.2f КБ", float64(v)/float64(KiB))
	default:
		return fmt.Sprintf("%d Б", v)
	}
}

func uint64ToInt64(v uint64) int64 {
	if v > uint64(^uint64(0)>>1) {
		return int64(^uint64(0) >> 1)
	}
	return int64(v)
}

func sendOvpnProfileActionsWithStats(ctx *Ctx, profileName, scope string) {
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Stat", scope+":act:stats:"+profileName),
			tgbotapi.NewInlineKeyboardButtonData("Profile", scope+":act:conf:"+profileName),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Back", scope+":back"),
			tgbotapi.NewInlineKeyboardButtonData("Cancel", "ui:cancel"),
		),
	)
	m := tgbotapi.NewMessage(ctx.ChatID, "Select action:")
	m.ReplyMarkup = kb
	_, err := ctx.Bot.Send(m)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", "inline send")
}
