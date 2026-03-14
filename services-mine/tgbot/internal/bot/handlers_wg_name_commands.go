package bot

import (
	"net/url"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const wgNameCbPrefix = "wgn:"

func RegisterWgNameCommands(reg *CommandRegistry) {
	// Admin-only name-based actions with disambiguation
	reg.Command(CommandSpec{Cmd: "/wg_enable", Args: "<profile_name_or_prefix>", Desc: "включить профиль", Section: "Админ: WireGuard", NeedAny: []Role{RoleAdmin}}, handleWgNameEnable, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."), RequireNonEmptyArg("Формат: /wg_enable <profile_name_or_prefix>"))
	reg.Command(CommandSpec{Cmd: "/wg_disable", Args: "<profile_name_or_prefix>", Desc: "выключить профиль", Section: "Админ: WireGuard", NeedAny: []Role{RoleAdmin}}, handleWgNameDisable, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."), RequireNonEmptyArg("Формат: /wg_disable <profile_name_or_prefix>"))
	reg.Command(CommandSpec{Cmd: "/wg_conf", Args: "<profile_name_or_prefix>", Desc: "получить конфиг", Section: "Админ: WireGuard", NeedAny: []Role{RoleAdmin}}, handleWgNameConf, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."), RequireNonEmptyArg("Формат: /wg_conf <profile_name_or_prefix>"))
	reg.Command(CommandSpec{Cmd: "/wg_qr", Args: "<profile_name_or_prefix>", Desc: "получить QR", Section: "Админ: WireGuard", NeedAny: []Role{RoleAdmin}}, handleWgNameQR, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."), RequireNonEmptyArg("Формат: /wg_qr <profile_name_or_prefix>"))
	reg.Command(CommandSpec{Cmd: "/wg_del", Args: "<profile_name_or_prefix>", Desc: "удалить профиль", Section: "Админ: WireGuard", NeedAny: []Role{RoleAdmin}}, handleWgNameDel, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."), RequireNonEmptyArg("Формат: /wg_del <profile_name_or_prefix>"))
	reg.Command(CommandSpec{Cmd: "/wg_rename", Args: "<old_name_or_prefix> <new_name>", Desc: "переименовать профиль", Section: "Админ: WireGuard", NeedAny: []Role{RoleAdmin}}, handleWgNameRename, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."), RequireNonEmptyArg("Формат: /wg_rename <old_name_or_prefix> <new_name>"))
}

func handleWgNameEnable(ctx *Ctx, arg string)  { wgNameAction(ctx, "enable", arg, "") }
func handleWgNameDisable(ctx *Ctx, arg string) { wgNameAction(ctx, "disable", arg, "") }
func handleWgNameConf(ctx *Ctx, arg string)    { wgNameAction(ctx, "conf", arg, "") }
func handleWgNameQR(ctx *Ctx, arg string)      { wgNameAction(ctx, "qr", arg, "") }
func handleWgNameDel(ctx *Ctx, arg string)     { wgNameAction(ctx, "delete", arg, "") }

func handleWgNameRename(ctx *Ctx, arg string) {
	fields := strings.Fields(arg)
	if len(fields) < 2 {
		reply(ctx.Bot, ctx.ChatID, "Формат: /wg_rename <old_name_or_prefix> <new_name>")
		return
	}
	old := fields[0]
	newName := strings.Join(fields[1:], " ")
	wgNameAction(ctx, "rename", old, newName)
}

func wgNameAction(ctx *Ctx, action string, query string, extra string) {
	if !ctx.IsPrivate {
		reply(ctx.Bot, ctx.ChatID, "Эта команда доступна только в личных сообщениях боту.")
		return
	}
	client, err := makeWgClientFromEnv()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, err.Error())
		return
	}
	peers, err := client.listPeers()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Не удалось получить список профилей:\n"+truncate(err.Error(), 3500))
		return
	}

	matches := matchPeersByNamePrefix(peers, query)
	if len(matches) == 0 {
		reply(ctx.Bot, ctx.ChatID, "Профиль не найден: "+query)
		return
	}
	if len(matches) == 1 {
		performWgPeerAction(ctx, client, action, string(matches[0].ID), extra)
		return
	}

	// disambiguation
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(matches)+1)
	extraEnc := url.QueryEscape(extra)
	for _, p := range matches {
		cb := wgNameCbPrefix + action + ":" + string(p.ID)
		if extra != "" {
			cb += ":" + extraEnc
		}
		rows = append(rows, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(p.Name, cb),
		})
	}
	rows = append(rows, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("Cancel", "ui:cancel"),
	})
	kb := tgbotapi.NewInlineKeyboardMarkup(rows...)
	m := tgbotapi.NewMessage(ctx.ChatID, "Найдено несколько профилей. Выбери точный:")
	m.ReplyMarkup = kb
	_, err = ctx.Bot.Send(m)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", "inline send")
}

func matchPeersByNamePrefix(peers []wgEasyPeer, query string) []wgEasyPeer {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	out := make([]wgEasyPeer, 0, 8)
	for _, p := range peers {
		name := strings.ToLower(p.Name)
		if strings.HasPrefix(name, q) {
			out = append(out, p)
		}
	}
	return out
}

func performWgPeerAction(ctx *Ctx, client *wgEasyClient, action string, peerID string, extra string) {
	switch action {
	case "enable":
		if err := client.enableClient(peerID); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Не удалось включить:\n"+truncate(err.Error(), 3500))
			return
		}
		reply(ctx.Bot, ctx.ChatID, "OK: enabled")
	case "disable":
		if err := client.disableClient(peerID); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Не удалось выключить:\n"+truncate(err.Error(), 3500))
			return
		}
		reply(ctx.Bot, ctx.ChatID, "OK: disabled")
	case "delete":
		// Confirm deletion (actual deletion in handleConfirmCallback)
		sendConfirm(ctx, "Удалить WireGuard профиль (id="+peerID+")?", "wgn:delete", peerID)
	case "conf":
		sendWgConfigAsFile(ctx, client, peerID)
	case "qr":
		sendWgQRCode(ctx, client, peerID)
	case "rename":
		if strings.TrimSpace(extra) == "" {
			reply(ctx.Bot, ctx.ChatID, "Новое имя не задано.")
			return
		}
		if err := client.renameClient(peerID, extra); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Не удалось переименовать:\n"+truncate(err.Error(), 3500))
			return
		}
		reply(ctx.Bot, ctx.ChatID, "OK: renamed")
	default:
		reply(ctx.Bot, ctx.ChatID, "Неизвестное действие.")
	}
}
