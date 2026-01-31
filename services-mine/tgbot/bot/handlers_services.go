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
	serviceName := envTrim("WG_STATS_SWARM_SERVICE")
	if serviceName == "" {
		reply(ctx.Bot, ctx.ChatID, "Не задана переменная окружения WG_STATS_SWARM_SERVICE")
		return
	}
	wgUser := ctx.User.Name + "_"
	out, err := runScript("sr_wg_stats.sh", serviceName, wgUser)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Скрипт выполнен с ошибкой:\n"+truncate(out, 3500))
		return
	}
	reply(ctx.Bot, ctx.ChatID, truncate(out, 3800))
}

func handleWgStatsAdmin(ctx *Ctx, _ string) {
	serviceName := envTrim("WG_STATS_SWARM_SERVICE")
	if serviceName == "" {
		reply(ctx.Bot, ctx.ChatID, "Не задана переменная окружения WG_STATS_SWARM_SERVICE")
		return
	}
	out, err := runScript("sr_wg_stats.sh", serviceName, "")
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Скрипт выполнен с ошибкой:\n"+truncate(out, 3500))
		return
	}
	reply(ctx.Bot, ctx.ChatID, truncate(out, 3800))
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
