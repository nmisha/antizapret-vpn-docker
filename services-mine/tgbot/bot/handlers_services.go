package main

import (
	"strconv"
	"strings"
	"time"
)

var aghUpdateLimiter = newCooldownLimiter(5 * time.Minute)

func RegisterServiceHandlers(r *Router) {
	r.Handle("/wgstats", handleWgStats,
		RequireRole(RoleWgStats, "Недостаточно прав. Нужна роль WgStats (или Admin)."),
	)
	r.Alias("wgstats", "/wgstats")

	r.Handle("/wgstats_admin", handleWgStatsAdmin,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
	)
	r.Alias("wgstats_admin", "/wgstats_admin")

	// AdGuard Home: update filter lists (blocklists + whitelists)
	// Canonical command: /sr_agh_update_lists
	r.Handle("/sr_agh_update_lists", handleAghUpdateLists,
		RequireAnyRole("Недостаточно прав. Нужна роль DomainEditor или ServiceManager (или Admin).", RoleDomainEditor, RoleServiceManager),
	)
	// Backward compatible alias:
	r.Handle("/agh_update_lists", handleAghUpdateLists,
		RequireAnyRole("Недостаточно прав. Нужна роль DomainEditor или ServiceManager (или Admin).", RoleDomainEditor, RoleServiceManager),
	)
	r.Alias("sr_agh_update_lists", "/sr_agh_update_lists")
	r.Alias("agh_update_lists", "/agh_update_lists")
}

func handleWgStats(ctx *Ctx, arg string) {
	if !ctx.IsPrivate {
		reply(ctx.Bot, ctx.ChatID, "Статистика WireGuard доступна только в личных сообщениях боту.")
		return
	}

	// Admin may request stats for a specific user: /wgstats <name>
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

	filtered := filterPeersByUserPrefixes(peers, target.WgProfiles)
	if len(filtered) == 0 {
		reply(ctx.Bot, ctx.ChatID, "Не найдено ни одного WireGuard профиля по вашим правилам из users.json (wg_profiles).")
		return
	}

	msg := formatWgPeersStats(filtered)
	replyHTML(ctx.Bot, ctx.ChatID, truncate(msg, 3800))
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
	replyHTML(ctx.Bot, ctx.ChatID, truncate(msg, 3800))
}

func handleAghUpdateLists(ctx *Ctx, _ string) {
	// Cooldown for non-admin users
	if !ctx.User.HasExact(RoleAdmin) {
		ok, wait := aghUpdateLimiter.allow(ctx.User.TelegramID)
		if !ok {
			reply(ctx.Bot, ctx.ChatID, "⏳ Слишком часто. Попробуйте через "+fmtDurationRu(wait)+".\n\nℹ️ Для пользователей без роли Admin действует тайм-аут 5 минут.")
			return
		}
	}

	client, err := newAghClientFromEnv()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Не заданы переменные окружения для AdGuard Home.\nНужно: AGH_HOST, AGH_PORT, AGH_LOGIN, AGH_PASSWORD\nОпционально: AGH_SCHEME (http/https)")
		return
	}

	updatedBlock, err := client.refreshFilters(false)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Не удалось обновить списки AdGuard Home:\n"+truncate(err.Error(), 3500))
		return
	}
	updatedWhite, err := client.refreshFilters(true)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Не удалось обновить whitelist-фильтры AdGuard Home:\n"+truncate(err.Error(), 3500))
		return
	}

	msg := "✅ Обновление списков AdGuard Home запущено.\n" +
		"• Blocklists обновлено: " + strconv.Itoa(updatedBlock) + "\n" +
		"• Whitelist-фильтров обновлено: " + strconv.Itoa(updatedWhite)

	if !ctx.User.HasExact(RoleAdmin) {
		msg += "\n\nℹ️ Для пользователей без роли Admin действует тайм-аут 5 минут."
	}
	reply(ctx.Bot, ctx.ChatID, truncate(msg, 3800))
}
