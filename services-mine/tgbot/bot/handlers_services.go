package main

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

func aghUserCooldownFromEnv() time.Duration {
	// Cooldown for non-Admin users for AdGuard list refresh command.
	// Default: 5 minutes. Override via AGH_USER_COOLDOWN_SECONDS (integer, seconds).
	cooldown := 5 * time.Minute
	env := envTrim("AGH_USER_COOLDOWN_SECONDS")
	if env == "" {
		return cooldown
	}
	n, err := strconv.Atoi(strings.TrimSpace(env))
	if err != nil || n <= 0 {
		return cooldown
	}
	return time.Duration(n) * time.Second
}

func aghCooldownNotice() string {
	// Neutral wording without role mentions (requested).
	return "ℹ️ Тайм-аут для повторного запроса: " + fmtDurationRu(aghUserCooldown) + "."
}

var aghUserCooldown = aghUserCooldownFromEnv()
var aghUpdateLimiter = newCooldownLimiter(aghUserCooldown)

var aghRefreshMu sync.Mutex
var aghRefreshInProgress bool
var aghRefreshStartedAt time.Time

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
	// Prevent parallel refresh runs (can be heavy and slow)
	aghRefreshMu.Lock()
	if aghRefreshInProgress {
		aghRefreshMu.Unlock()
		reply(ctx.Bot, ctx.ChatID, "⏳ Обновление списков AdGuard Home уже выполняется. Попробуйте позже.")
		return
	}
	aghRefreshInProgress = true
	aghRefreshStartedAt = time.Now()
	aghRefreshMu.Unlock()
	defer func() {
		aghRefreshMu.Lock()
		aghRefreshInProgress = false
		aghRefreshMu.Unlock()
	}()

	// Cooldown for non-admin users
	if !ctx.User.HasExact(RoleAdmin) {
		ok, wait := aghUpdateLimiter.allow(ctx.User.TelegramID)
		if !ok {
			reply(ctx.Bot, ctx.ChatID, "⏳ Слишком часто. Повторно можно запросить через "+fmtDurationRu(wait)+".")
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
		logAghErrorIfEnabled(ctx, "AdGuard refresh blocklists failed: "+err.Error())
		if ctx.User.HasExact(RoleAdmin) {
			reply(ctx.Bot, ctx.ChatID, "Не удалось обновить списки AdGuard Home:\n"+truncate(err.Error(), 3500))
			return
		}
		reply(ctx.Bot, ctx.ChatID, "Не удалось обновить списки AdGuard Home. Попробуйте позже.\n\n"+aghCooldownNotice())
		return
	}

	updatedWhite, err := client.refreshFilters(true)
	if err != nil {
		logAghErrorIfEnabled(ctx, "AdGuard refresh whitelists failed: "+err.Error())
		if ctx.User.HasExact(RoleAdmin) {
			reply(ctx.Bot, ctx.ChatID, "Не удалось обновить whitelist-фильтры AdGuard Home:\n"+truncate(err.Error(), 3500))
			return
		}
		reply(ctx.Bot, ctx.ChatID, "Не удалось обновить списки AdGuard Home. Попробуйте позже.\n\n"+aghCooldownNotice())
		return
	}

	var b strings.Builder
	b.WriteString("✅ Обновление списков AdGuard Home выполнено.\n")
	if ctx.User.HasExact(RoleAdmin) {
		fmt.Fprintf(&b, "Обновлено списков: блок-листы — %d, whitelist — %d.\n", updatedBlock, updatedWhite)
	}
	if !ctx.User.HasExact(RoleAdmin) {
		b.WriteString("Повторно можно запросить через ")
		b.WriteString(fmtDurationRu(aghUserCooldown))
		b.WriteString(".\n")
		b.WriteString(aghCooldownNotice())
	} else {
		b.WriteString("Повторно можно запросить сразу.")
	}

	reply(ctx.Bot, ctx.ChatID, truncate(b.String(), 3800))
}

func logAghErrorIfEnabled(ctx *Ctx, msg string) {
	if gLogger == nil {
		return
	}
	s := getSettingsCached()
	if !s.LoggingEnabled {
		return
	}
	// include both telegram sender label (if any) and user record name
	label := strings.TrimSpace(ctx.FromUser)
	if ctx.User.Name != "" {
		if label != "" {
			label = label + "/" + ctx.User.Name
		} else {
			label = ctx.User.Name
		}
	}
	gLogger.Append(formatLogLine("ERR", ctx.ChatID, label, truncate(msg, 2000)))
}
