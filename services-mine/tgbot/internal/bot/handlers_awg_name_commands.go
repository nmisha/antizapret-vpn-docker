package bot

import (
	"net/url"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func RegisterAwgNameCommands(reg *CommandRegistry) {
	reg.Command(CommandSpec{Cmd: "/awg_enable", Args: "<profile_name_or_prefix>", Desc: "включить профиль", Section: "Админ: Amnezia WireGuard", NeedAny: []Role{RoleAdmin}}, handleAwgNameEnable, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."), RequireNonEmptyArg("Формат: /awg_enable <profile_name_or_prefix>"))
	reg.Command(CommandSpec{Cmd: "/awg_disable", Args: "<profile_name_or_prefix>", Desc: "выключить профиль", Section: "Админ: Amnezia WireGuard", NeedAny: []Role{RoleAdmin}}, handleAwgNameDisable, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."), RequireNonEmptyArg("Формат: /awg_disable <profile_name_or_prefix>"))
	reg.Command(CommandSpec{Cmd: "/awg_conf", Args: "<profile_name_or_prefix>", Desc: "получить конфиг", Section: "Админ: Amnezia WireGuard", NeedAny: []Role{RoleAdmin}}, handleAwgNameConf, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."), RequireNonEmptyArg("Формат: /awg_conf <profile_name_or_prefix>"))
	reg.Command(CommandSpec{Cmd: "/awg_qr", Args: "<profile_name_or_prefix>", Desc: "получить QR", Section: "Админ: Amnezia WireGuard", NeedAny: []Role{RoleAdmin}}, handleAwgNameQR, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."), RequireNonEmptyArg("Формат: /awg_qr <profile_name_or_prefix>"))
	reg.Command(CommandSpec{Cmd: "/awg_score", Args: "<profile_name_or_prefix>", Desc: "получить risk score профиля", Section: "Админ: Amnezia WireGuard", NeedAny: []Role{RoleAdmin}}, handleAwgNameScore, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."), RequireNonEmptyArg("Формат: /awg_score <profile_name_or_prefix>"))
	reg.Command(CommandSpec{Cmd: "/awg_reset_score", Args: "<profile_name_or_prefix>", Desc: "reset dns-guard risk score", Section: "Admin: Amnezia WireGuard", NeedAny: []Role{RoleAdmin}}, handleAwgNameResetScore, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."), RequireNonEmptyArg("Формат: /awg_reset_score <profile_name_or_prefix>"))
	reg.Command(CommandSpec{Cmd: "/awg_del", Args: "<profile_name_or_prefix>", Desc: "удалить профиль", Section: "Админ: Amnezia WireGuard", NeedAny: []Role{RoleAdmin}}, handleAwgNameDel, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."), RequireNonEmptyArg("Формат: /awg_del <profile_name_or_prefix>"))
	reg.Command(CommandSpec{Cmd: "/awg_rename", Args: "<old_name_or_prefix> <new_name>", Desc: "переименовать профиль", Section: "Админ: Amnezia WireGuard", NeedAny: []Role{RoleAdmin}}, handleAwgNameRename, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."), RequireNonEmptyArg("Формат: /awg_rename <old_name_or_prefix> <new_name>"))
}

func handleAwgNameEnable(ctx *Ctx, arg string)  { awgNameAction(ctx, "enable", arg, "") }
func handleAwgNameDisable(ctx *Ctx, arg string) { awgNameAction(ctx, "disable", arg, "") }
func handleAwgNameConf(ctx *Ctx, arg string)    { awgNameAction(ctx, "conf", arg, "") }
func handleAwgNameQR(ctx *Ctx, arg string)      { awgNameAction(ctx, "qr", arg, "") }
func handleAwgNameScore(ctx *Ctx, arg string)   { awgNameAction(ctx, "score", arg, "") }
func handleAwgNameResetScore(ctx *Ctx, arg string) {
	awgNameAction(ctx, "reset_score", arg, "")
}
func handleAwgNameDel(ctx *Ctx, arg string) { awgNameAction(ctx, "delete", arg, "") }

func handleAwgNameRename(ctx *Ctx, arg string) {
	fields := strings.Fields(arg)
	if len(fields) < 2 {
		reply(ctx.Bot, ctx.ChatID, "Формат: /awg_rename <old_name_or_prefix> <new_name>")
		return
	}
	old := fields[0]
	newName := strings.Join(fields[1:], " ")
	awgNameAction(ctx, "rename", old, newName)
}

func awgNameAction(ctx *Ctx, action string, query string, extra string) {
	if !ctx.IsPrivate {
		reply(ctx.Bot, ctx.ChatID, "Эта команда доступна только в личных сообщениях боту.")
		return
	}
	client, err := makeAwgClientFromEnv()
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
		performAwgPeerAction(ctx, client, action, string(matches[0].ID), extra)
		return
	}

	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(matches)+1)
	extraEnc := url.QueryEscape(extra)
	for _, p := range matches {
		cb := awgNameCbPrefix + action + ":" + string(p.ID)
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

func performAwgPeerAction(ctx *Ctx, client *wgEasyClient, action string, peerID string, extra string) {
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
		sendConfirm(ctx, "Удалить Amnezia WireGuard профиль (id="+peerID+")?", "awgn:delete", peerID)
	case "conf":
		sendAwgConfigAsFile(ctx, client, peerID)
	case "qr":
		sendAwgQRCode(ctx, client, peerID)
	case "score":
		peers, err := client.listPeers()
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Не удалось получить список профилей:\n"+truncate(err.Error(), 3500))
			return
		}
		for _, p := range peers {
			if string(p.ID) == peerID {
				sendDNSGuardRiskScore(ctx, "awg", p.Name, true)
				return
			}
		}
		reply(ctx.Bot, ctx.ChatID, "Профиль не найден.")
	case "reset_score":
		peers, err := client.listPeers()
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Не удалось получить список профилей:\n"+truncate(err.Error(), 3500))
			return
		}
		for _, p := range peers {
			if string(p.ID) == peerID {
				sendConfirm(ctx, "Сбросить risk score и state для Amnezia WireGuard профиля "+p.Name+"?", "awg:reset_score", p.Name)
				return
			}
		}
		reply(ctx.Bot, ctx.ChatID, "Профиль не найден.")
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
