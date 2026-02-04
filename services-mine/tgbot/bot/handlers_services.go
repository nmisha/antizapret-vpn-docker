package main

func RegisterServiceHandlers(r *Router) {
	r.Handle("/wgstats", handleWgStats,
		RequireRole(RoleInfo, "Недостаточно прав. Нужна роль Info (или Admin)."),
	)
	r.Alias("wgstats", "/wgstats")

	r.Handle("/wgstats_admin", handleWgStatsAdmin,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
	)
	r.Alias("wgstats_admin", "/wgstats_admin")

	r.Handle("/agh_update_lists", handleAghUpdateLists,
		RequireRole(RoleServiceManager, "Недостаточно прав. Нужна роль ServiceManager (или Admin)."),
	)
	r.Alias("agh_update_lists", "/agh_update_lists")
}

func handleWgStats(ctx *Ctx, _ string) {
	if !ctx.IsPrivate {
		reply(ctx.Bot, ctx.ChatID, "Статистика WireGuard доступна только в личных сообщениях боту.")
		return
	}
	host := envTrim("WG_HOST")
	port := envTrim("WG_PORT")
	pass := envTrim("WG_PASSWORD")

	client, err := newWgEasyClient(host, port, pass)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Не заданы переменные окружения WireGuard. Нужно: WG_HOST, WG_PORT, WG_PASSWORD")
		return
	}

	peers, err := client.listPeers()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Не удалось получить статистику WireGuard:\n"+truncate(err.Error(), 3500))
		return
	}

	filtered := filterPeersByUserPrefixes(peers, ctx.User.WgProfiles)
	if len(filtered) == 0 {
		reply(ctx.Bot, ctx.ChatID, "Не найдено ни одного WireGuard профиля по вашим правилам из users.json (wg_profiles).")
		return
	}

	msg := formatWgPeersStats(filtered)
	reply(ctx.Bot, ctx.ChatID, truncate(msg, 3800))
}

func handleWgStatsAdmin(ctx *Ctx, _ string) {
	if !ctx.IsPrivate {
		reply(ctx.Bot, ctx.ChatID, "Статистика WireGuard доступна только в личных сообщениях боту.")
		return
	}
	host := envTrim("WG_HOST")
	port := envTrim("WG_PORT")
	pass := envTrim("WG_PASSWORD")

	client, err := newWgEasyClient(host, port, pass)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Не заданы переменные окружения WireGuard. Нужно: WG_HOST, WG_PORT, WG_PASSWORD")
		return
	}

	peers, err := client.listPeers()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Не удалось получить статистику WireGuard:\n"+truncate(err.Error(), 3500))
		return
	}

	msg := formatWgPeersStats(peers)
	reply(ctx.Bot, ctx.ChatID, truncate(msg, 3800))
}

func handleAghUpdateLists(ctx *Ctx, _ string) {
	host := envTrim("AGH_HOST")
	login := envTrim("AGH_LOGIN")
	pass := envTrim("AGH_PASSWORD")
	port := envTrim("AGH_PORT")

	if host == "" || login == "" || pass == "" || port == "" {
		reply(ctx.Bot, ctx.ChatID, "Не заданы переменные окружения для AdGuard. Нужно: AGH_HOST, AGH_LOGIN, AGH_PASSWORD, AGH_PORT")
		return
	}

	out, err := runScript("sr_agh_update_lists.sh", host, login, pass, port)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Скрипт выполнен с ошибкой:\n"+truncate(out, 3500))
		return
	}
	reply(ctx.Bot, ctx.ChatID, truncate(out, 3800))
}
