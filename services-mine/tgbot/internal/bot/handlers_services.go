package bot

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
var aghRefreshStartedBy string
var aghRefreshStartedChat int64

// Best-effort: last successful refresh completion time.
var aghLastRefreshMu sync.RWMutex
var aghLastRefreshAt time.Time

func RegisterServiceHandlers(reg *CommandRegistry) {
	// WireGuard stats (historically used both /wgstats and /wg_stats)
	reg.Command(CommandSpec{Cmd: "/wgstats", Desc: "статистика WG (legacy alias)", Section: "WireGuard", NeedAny: []Role{RoleWgStats}, Hidden: true}, handleWgStats,
		RequireRole(RoleWgStats, "Недостаточно прав. Нужна роль WgStats (или Admin)."),
	)
	reg.Alias("wgstats", "/wgstats")
	reg.Command(CommandSpec{Cmd: "/wg_stats", Desc: "статистика WG (для своего пользователя)", Section: "WireGuard", NeedAny: []Role{RoleWgStats}}, handleWgStats,
		RequireRole(RoleWgStats, "Недостаточно прав. Нужна роль WgStats (или Admin)."),
	)
	reg.Alias("wg_stats", "/wg_stats")

	reg.Command(CommandSpec{Cmd: "/wgstats_admin", Desc: "статистика WG (legacy alias)", Section: "Админ: WireGuard", NeedAny: []Role{RoleAdmin}, Hidden: true}, handleWgStatsAdmin,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
	)
	reg.Alias("wgstats_admin", "/wgstats_admin")
	reg.Command(CommandSpec{Cmd: "/wg_stats_admin", Desc: "статистика WG (admin)", Section: "Админ: WireGuard", NeedAny: []Role{RoleAdmin}}, handleWgStatsAdmin,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
	)
	reg.Alias("wg_stats_admin", "/wg_stats_admin")

	// AdGuard Home: update filter lists (blocklists + whitelists)
	// Canonical command: /agh_update_lists
	reg.Command(CommandSpec{Cmd: "/agh_update_lists", Desc: "обновить списки AdGuard Home (filters)", Section: "Сервисы", NeedAny: []Role{RoleDomainEditor, RoleServiceManager}}, handleAghUpdateLists,
		RequireAnyRole("Недостаточно прав. Нужна роль DomainEditor или ServiceManager (или Admin).", RoleDomainEditor, RoleServiceManager),
	)
	reg.Alias("agh_update_lists", "/agh_update_lists")
	// Backward compatible (old name):
	reg.Command(CommandSpec{Cmd: "/sr_agh_update_lists", Desc: "legacy alias", Section: "Сервисы", NeedAny: []Role{RoleDomainEditor, RoleServiceManager}, Hidden: true}, handleAghUpdateLists,
		RequireAnyRole("Недостаточно прав. Нужна роль DomainEditor или ServiceManager (или Admin).", RoleDomainEditor, RoleServiceManager),
	)
	reg.Alias("sr_agh_update_lists", "/sr_agh_update_lists")

	// AdGuard Home: show last update time (Admin only)
	reg.Command(CommandSpec{Cmd: "/agh_last_update", Desc: "дата и время последнего обновления списков AdGuard Home", Section: "Админ: сервисы", NeedAny: []Role{RoleAdmin}}, handleAghLastUpdate,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
	)
	reg.Alias("agh_last_update", "/agh_last_update")
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
	// Prevent parallel refresh runs (can be heavy and slow). Do NOT block the whole bot.
	aghRefreshMu.Lock()
	if aghRefreshInProgress {
		startedAt := aghRefreshStartedAt
		aghRefreshMu.Unlock()
		msg := "⏳ Обновление списков AdGuard Home уже выполняется. Попробуйте позже."
		// For non-admin add a neutral cooldown line (no role mention).
		if !ctx.User.HasExact(RoleAdmin) {
			msg += "\n" + aghCooldownNotice()
		}
		// Admin can see how long it's been running (optional, short).
		if ctx.User.HasExact(RoleAdmin) && !startedAt.IsZero() {
			msg += "\n(выполняется уже " + fmtDurationRu(time.Since(startedAt)) + ")"
		}
		reply(ctx.Bot, ctx.ChatID, msg)
		return
	}
	aghRefreshMu.Unlock()

	// Cooldown for non-admin users (rate limit between invocations)
	if !ctx.User.HasExact(RoleAdmin) {
		ok, wait := aghUpdateLimiter.allow(ctx.User.TelegramID)
		if !ok {
			reply(ctx.Bot, ctx.ChatID, "⏳ Слишком часто. Повторно можно запросить через "+fmtDurationRu(wait)+".")
			return
		}
	}

	// Mark as in progress now
	aghRefreshMu.Lock()
	aghRefreshInProgress = true
	aghRefreshStartedAt = time.Now()
	aghRefreshStartedBy = strings.TrimSpace(ctx.FromUser)
	if ctx.User.Name != "" {
		if aghRefreshStartedBy != "" {
			aghRefreshStartedBy += "/" + ctx.User.Name
		} else {
			aghRefreshStartedBy = ctx.User.Name
		}
	}
	aghRefreshStartedChat = ctx.ChatID
	aghRefreshMu.Unlock()

	// Immediate acknowledgement, then do work asynchronously.
	startMsg := "⏳ Обновление списков AdGuard Home запущено."
	if !ctx.User.HasExact(RoleAdmin) {
		startMsg += "\n" + aghCooldownNotice()
	}
	reply(ctx.Bot, ctx.ChatID, startMsg)

	// Run refresh in background so other commands keep working.
	bot := ctx.Bot
	chatID := ctx.ChatID
	isAdmin := ctx.User.HasExact(RoleAdmin)
	fromLabel := strings.TrimSpace(ctx.FromUser)
	if ctx.User.Name != "" {
		if fromLabel != "" {
			fromLabel += "/" + ctx.User.Name
		} else {
			fromLabel = ctx.User.Name
		}
	}
	go func() {
		defer func() {
			aghRefreshMu.Lock()
			aghRefreshInProgress = false
			aghRefreshMu.Unlock()
		}()

		client, err := newAghClientFromEnv()
		if err != nil {
			logAghErrorIfEnabledChat(chatID, fromLabel, "AGH env error: "+err.Error())
			// keep user-facing message generic unless admin
			if isAdmin {
				reply(bot, chatID, "Не заданы переменные окружения для AdGuard Home.\nНужно: AGH_HOST, AGH_PORT, AGH_LOGIN, AGH_PASSWORD\nОпционально: AGH_SCHEME (http/https)")
				return
			}
			reply(bot, chatID, "Не удалось обновить списки AdGuard Home. Попробуйте позже.\n"+aghCooldownNotice())
			return
		}

		updatedBlock, err := client.refreshFilters(false)
		if err != nil {
			logAghErrorIfEnabledChat(chatID, fromLabel, "AdGuard refresh blocklists failed: "+err.Error())
			if isAdmin {
				reply(bot, chatID, "Не удалось обновить списки AdGuard Home:\n"+truncate(err.Error(), 3500))
				return
			}
			reply(bot, chatID, "Не удалось обновить списки AdGuard Home. Попробуйте позже.\n"+aghCooldownNotice())
			return
		}

		updatedWhite, err := client.refreshFilters(true)
		if err != nil {
			logAghErrorIfEnabledChat(chatID, fromLabel, "AdGuard refresh whitelists failed: "+err.Error())
			if isAdmin {
				reply(bot, chatID, "Не удалось обновить whitelist-фильтры AdGuard Home:\n"+truncate(err.Error(), 3500))
				return
			}
			reply(bot, chatID, "Не удалось обновить списки AdGuard Home. Попробуйте позже.\n"+aghCooldownNotice())
			return
		}

		// record successful completion time
		aghLastRefreshMu.Lock()
		aghLastRefreshAt = time.Now()
		aghLastRefreshMu.Unlock()

		var b strings.Builder
		b.WriteString("✅ Обновление списков AdGuard Home выполнено.")
		if isAdmin {
			fmt.Fprintf(&b, "\nОбновлено списков: блок-листы — %d, whitelist — %d.", updatedBlock, updatedWhite)
		} else {
			b.WriteString("\nПовторно можно запросить через ")
			b.WriteString(fmtDurationRu(aghUserCooldown))
			b.WriteString(".")
			b.WriteString("\n" + aghCooldownNotice())
		}
		reply(bot, chatID, truncate(b.String(), 3800))
	}()
}

func handleAghLastUpdate(ctx *Ctx, _ string) {
	client, err := newAghClientFromEnv()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Не заданы переменные окружения для AdGuard Home.")
		return
	}
	blockT, whiteT, err := client.lastFilterUpdateTimes()
	if err != nil {
		logAghErrorIfEnabled(ctx, "AdGuard status failed: "+err.Error())
		reply(ctx.Bot, ctx.ChatID, "Не удалось получить статус обновлений AdGuard Home:\n"+truncate(err.Error(), 3500))
		return
	}

	aghLastRefreshMu.RLock()
	local := aghLastRefreshAt
	aghLastRefreshMu.RUnlock()

	var b strings.Builder
	b.WriteString("Последнее обновление списков AdGuard Home:\n")
	b.WriteString("• блок-листы: ")
	b.WriteString(formatTimeRu(blockT))
	b.WriteString("\n• whitelist: ")
	b.WriteString(formatTimeRu(whiteT))
	if !local.IsZero() {
		b.WriteString("\n• (по данным бота, последнее успешное): ")
		b.WriteString(formatTimeRu(local))
	}

	reply(ctx.Bot, ctx.ChatID, truncate(b.String(), 3800))
}

func logAghErrorIfEnabled(ctx *Ctx, msg string) {
	if gLogger == nil {
		return
	}
	s := getSettingsCached()
	if !s.LogErrorsEnabled {
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

func logAghErrorIfEnabledChat(chatID int64, userLabel string, msg string) {
	if gLogger == nil {
		return
	}
	s := getSettingsCached()
	if !s.LogErrorsEnabled {
		return
	}
	gLogger.Append(formatLogLine("ERR", chatID, userLabel, truncate(msg, 2000)))
}

func formatTimeRu(t time.Time) string {
	if t.IsZero() {
		return "нет данных"
	}
	// Use local time zone on the host running the bot.
	return t.Local().Format("2006-01-02 15:04:05")
}
