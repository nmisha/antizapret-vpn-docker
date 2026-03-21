package bot

import "strings"

func RegisterAwgStatsHandlers(reg *CommandRegistry) {
	reg.Command(CommandSpec{Cmd: "/awgstats", Desc: "статистика AWG (legacy alias)", Section: "Amnezia WireGuard", NeedAny: []Role{RoleAwgStats}, Hidden: true}, handleAwgStats,
		RequireRole(RoleAwgStats, "Недостаточно прав. Нужна роль AwgStats (или Admin)."),
	)
	reg.Alias("awgstats", "/awgstats")
	reg.Command(CommandSpec{Cmd: "/awg_stats", Desc: "статистика AWG по своим профилям", Section: "Amnezia WireGuard", NeedAny: []Role{RoleAwgStats}}, handleAwgStats,
		RequireRole(RoleAwgStats, "Недостаточно прав. Нужна роль AwgStats (или Admin)."),
	)
	reg.Alias("awg_stats", "/awg_stats")

	reg.Command(CommandSpec{Cmd: "/awgstats_admin", Desc: "статистика AWG (legacy alias)", Section: "Админ: Amnezia WireGuard", NeedAny: []Role{RoleAdmin}, Hidden: true}, handleAwgStatsAdmin,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
	)
	reg.Alias("awgstats_admin", "/awgstats_admin")
	reg.Command(CommandSpec{Cmd: "/awg_stats_admin", Desc: "статистика AWG (admin)", Section: "Админ: Amnezia WireGuard", NeedAny: []Role{RoleAdmin}}, handleAwgStatsAdmin,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
	)
	reg.Alias("awg_stats_admin", "/awg_stats_admin")
}

func handleAwgStats(ctx *Ctx, arg string) {
	if !ctx.IsPrivate {
		reply(ctx.Bot, ctx.ChatID, "Статистика Amnezia WireGuard доступна только в личных сообщениях боту.")
		return
	}
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
	client, err := makeAwgClientFromEnv()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, err.Error())
		return
	}
	peers, err := client.listPeers()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Не удалось получить статистику Amnezia WireGuard:\n"+truncate(err.Error(), 3500))
		return
	}
	filtered := filterPeersByUserPrefixes(peers, target.WgProfiles)
	if len(filtered) == 0 {
		reply(ctx.Bot, ctx.ChatID, "Не найдено ни одного Amnezia WireGuard профиля по вашим правилам из users.json (wg_profiles).")
		return
	}
	replyHTML(ctx.Bot, ctx.ChatID, truncate(formatWgPeersStats(filtered), 3800))
}

func handleAwgStatsAdmin(ctx *Ctx, _ string) {
	if !ctx.IsPrivate {
		reply(ctx.Bot, ctx.ChatID, "Статистика Amnezia WireGuard доступна только в личных сообщениях боту.")
		return
	}
	client, err := makeAwgClientFromEnv()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, err.Error())
		return
	}
	peers, err := client.listPeers()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Не удалось получить статистику Amnezia WireGuard:\n"+truncate(err.Error(), 3500))
		return
	}
	replyHTML(ctx.Bot, ctx.ChatID, truncate(formatWgPeersStats(peers), 3800))
}
