package bot

import (
	"net/url"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const wgNameCbPrefix = "wgn:"

func RegisterWgNameCommands(reg *CommandRegistry) {
	// Admin-only name-based actions with disambiguation
	reg.Command(CommandSpec{Cmd: "/wg_enable", Args: "<profile_name_or_prefix>", Desc: "РІРєР»СЋС‡РёС‚СЊ РїСЂРѕС„РёР»СЊ", Section: "РђРґРјРёРЅ: WireGuard", NeedAny: []Role{RoleAdmin}}, handleWgNameEnable, RequireRole(RoleAdmin, "РќРµРґРѕСЃС‚Р°С‚РѕС‡РЅРѕ РїСЂР°РІ. РќСѓР¶РЅР° СЂРѕР»СЊ Admin."), RequireNonEmptyArg("Р¤РѕСЂРјР°С‚: /wg_enable <profile_name_or_prefix>"))
	reg.Command(CommandSpec{Cmd: "/wg_disable", Args: "<profile_name_or_prefix>", Desc: "РІС‹РєР»СЋС‡РёС‚СЊ РїСЂРѕС„РёР»СЊ", Section: "РђРґРјРёРЅ: WireGuard", NeedAny: []Role{RoleAdmin}}, handleWgNameDisable, RequireRole(RoleAdmin, "РќРµРґРѕСЃС‚Р°С‚РѕС‡РЅРѕ РїСЂР°РІ. РќСѓР¶РЅР° СЂРѕР»СЊ Admin."), RequireNonEmptyArg("Р¤РѕСЂРјР°С‚: /wg_disable <profile_name_or_prefix>"))
	reg.Command(CommandSpec{Cmd: "/wg_conf", Args: "<profile_name_or_prefix>", Desc: "РїРѕР»СѓС‡РёС‚СЊ РєРѕРЅС„РёРі", Section: "РђРґРјРёРЅ: WireGuard", NeedAny: []Role{RoleAdmin}}, handleWgNameConf, RequireRole(RoleAdmin, "РќРµРґРѕСЃС‚Р°С‚РѕС‡РЅРѕ РїСЂР°РІ. РќСѓР¶РЅР° СЂРѕР»СЊ Admin."), RequireNonEmptyArg("Р¤РѕСЂРјР°С‚: /wg_conf <profile_name_or_prefix>"))
	reg.Command(CommandSpec{Cmd: "/wg_qr", Args: "<profile_name_or_prefix>", Desc: "РїРѕР»СѓС‡РёС‚СЊ QR", Section: "РђРґРјРёРЅ: WireGuard", NeedAny: []Role{RoleAdmin}}, handleWgNameQR, RequireRole(RoleAdmin, "РќРµРґРѕСЃС‚Р°С‚РѕС‡РЅРѕ РїСЂР°РІ. РќСѓР¶РЅР° СЂРѕР»СЊ Admin."), RequireNonEmptyArg("Р¤РѕСЂРјР°С‚: /wg_qr <profile_name_or_prefix>"))
	reg.Command(CommandSpec{Cmd: "/wg_score", Args: "<profile_name_or_prefix>", Desc: "РїРѕР»СѓС‡РёС‚СЊ risk score РїСЂРѕС„РёР»СЏ", Section: "РђРґРјРёРЅ: WireGuard", NeedAny: []Role{RoleAdmin}}, handleWgNameScore, RequireRole(RoleAdmin, "РќРµРґРѕСЃС‚Р°С‚РѕС‡РЅРѕ РїСЂР°РІ. РќСѓР¶РЅР° СЂРѕР»СЊ Admin."), RequireNonEmptyArg("Р¤РѕСЂРјР°С‚: /wg_score <profile_name_or_prefix>"))
	reg.Command(CommandSpec{Cmd: "/wg_reset_score", Args: "<profile_name_or_prefix>", Desc: "reset dns-guard risk score", Section: "Admin: WireGuard", NeedAny: []Role{RoleAdmin}}, handleWgNameResetScore, RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."), RequireNonEmptyArg("Формат: /wg_reset_score <profile_name_or_prefix>"))
	reg.Command(CommandSpec{Cmd: "/wg_del", Args: "<profile_name_or_prefix>", Desc: "СѓРґР°Р»РёС‚СЊ РїСЂРѕС„РёР»СЊ", Section: "РђРґРјРёРЅ: WireGuard", NeedAny: []Role{RoleAdmin}}, handleWgNameDel, RequireRole(RoleAdmin, "РќРµРґРѕСЃС‚Р°С‚РѕС‡РЅРѕ РїСЂР°РІ. РќСѓР¶РЅР° СЂРѕР»СЊ Admin."), RequireNonEmptyArg("Р¤РѕСЂРјР°С‚: /wg_del <profile_name_or_prefix>"))
	reg.Command(CommandSpec{Cmd: "/wg_rename", Args: "<old_name_or_prefix> <new_name>", Desc: "РїРµСЂРµРёРјРµРЅРѕРІР°С‚СЊ РїСЂРѕС„РёР»СЊ", Section: "РђРґРјРёРЅ: WireGuard", NeedAny: []Role{RoleAdmin}}, handleWgNameRename, RequireRole(RoleAdmin, "РќРµРґРѕСЃС‚Р°С‚РѕС‡РЅРѕ РїСЂР°РІ. РќСѓР¶РЅР° СЂРѕР»СЊ Admin."), RequireNonEmptyArg("Р¤РѕСЂРјР°С‚: /wg_rename <old_name_or_prefix> <new_name>"))
}

func handleWgNameEnable(ctx *Ctx, arg string)  { wgNameAction(ctx, "enable", arg, "") }
func handleWgNameDisable(ctx *Ctx, arg string) { wgNameAction(ctx, "disable", arg, "") }
func handleWgNameConf(ctx *Ctx, arg string)    { wgNameAction(ctx, "conf", arg, "") }
func handleWgNameQR(ctx *Ctx, arg string)      { wgNameAction(ctx, "qr", arg, "") }
func handleWgNameScore(ctx *Ctx, arg string)   { wgNameAction(ctx, "score", arg, "") }
func handleWgNameResetScore(ctx *Ctx, arg string) {
	wgNameAction(ctx, "reset_score", arg, "")
}
func handleWgNameDel(ctx *Ctx, arg string) { wgNameAction(ctx, "delete", arg, "") }

func handleWgNameRename(ctx *Ctx, arg string) {
	fields := strings.Fields(arg)
	if len(fields) < 2 {
		reply(ctx.Bot, ctx.ChatID, "Р¤РѕСЂРјР°С‚: /wg_rename <old_name_or_prefix> <new_name>")
		return
	}
	old := fields[0]
	newName := strings.Join(fields[1:], " ")
	wgNameAction(ctx, "rename", old, newName)
}

func wgNameAction(ctx *Ctx, action string, query string, extra string) {
	if !ctx.IsPrivate {
		reply(ctx.Bot, ctx.ChatID, "Р­С‚Р° РєРѕРјР°РЅРґР° РґРѕСЃС‚СѓРїРЅР° С‚РѕР»СЊРєРѕ РІ Р»РёС‡РЅС‹С… СЃРѕРѕР±С‰РµРЅРёСЏС… Р±РѕС‚Сѓ.")
		return
	}
	client, err := makeWgClientFromEnv()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, err.Error())
		return
	}
	peers, err := client.listPeers()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "РќРµ СѓРґР°Р»РѕСЃСЊ РїРѕР»СѓС‡РёС‚СЊ СЃРїРёСЃРѕРє РїСЂРѕС„РёР»РµР№:\n"+truncate(err.Error(), 3500))
		return
	}

	matches := matchPeersByNamePrefix(peers, query)
	if len(matches) == 0 {
		reply(ctx.Bot, ctx.ChatID, "РџСЂРѕС„РёР»СЊ РЅРµ РЅР°Р№РґРµРЅ: "+query)
		return
	}
	if len(matches) == 1 {
		performWgPeerAction(ctx, client, action, string(matches[0].ID), extra)
		return
	}

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
	m := tgbotapi.NewMessage(ctx.ChatID, "РќР°Р№РґРµРЅРѕ РЅРµСЃРєРѕР»СЊРєРѕ РїСЂРѕС„РёР»РµР№. Р’С‹Р±РµСЂРё С‚РѕС‡РЅС‹Р№:")
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
			reply(ctx.Bot, ctx.ChatID, "РќРµ СѓРґР°Р»РѕСЃСЊ РІРєР»СЋС‡РёС‚СЊ:\n"+truncate(err.Error(), 3500))
			return
		}
		reply(ctx.Bot, ctx.ChatID, "OK: enabled")
	case "disable":
		if err := client.disableClient(peerID); err != nil {
			reply(ctx.Bot, ctx.ChatID, "РќРµ СѓРґР°Р»РѕСЃСЊ РІС‹РєР»СЋС‡РёС‚СЊ:\n"+truncate(err.Error(), 3500))
			return
		}
		reply(ctx.Bot, ctx.ChatID, "OK: disabled")
	case "delete":
		sendConfirm(ctx, "РЈРґР°Р»РёС‚СЊ WireGuard РїСЂРѕС„РёР»СЊ (id="+peerID+")?", "wgn:delete", peerID)
	case "conf":
		sendWgConfigAsFile(ctx, client, peerID)
	case "qr":
		sendWgQRCode(ctx, client, peerID)
	case "score":
		peers, err := client.listPeers()
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "РќРµ СѓРґР°Р»РѕСЃСЊ РїРѕР»СѓС‡РёС‚СЊ СЃРїРёСЃРѕРє РїСЂРѕС„РёР»РµР№:\n"+truncate(err.Error(), 3500))
			return
		}
		for _, p := range peers {
			if string(p.ID) == peerID {
				sendDNSGuardRiskScore(ctx, "wg", p.Name, true)
				return
			}
		}
		reply(ctx.Bot, ctx.ChatID, "РџСЂРѕС„РёР»СЊ РЅРµ РЅР°Р№РґРµРЅ.")
	case "reset_score":
		peers, err := client.listPeers()
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "РќРµ СѓРґР°Р»РѕСЃСЊ РїРѕР»СѓС‡РёС‚СЊ СЃРїРёСЃРѕРє РїСЂРѕС„РёР»РµР№:\n"+truncate(err.Error(), 3500))
			return
		}
		for _, p := range peers {
			if string(p.ID) == peerID {
				sendConfirm(ctx, "РЎР±СЂРѕСЃРёС‚СЊ risk score Рё state РґР»СЏ WireGuard РїСЂРѕС„РёР»СЏ "+p.Name+"?", "wg:reset_score", p.Name)
				return
			}
		}
		reply(ctx.Bot, ctx.ChatID, "РџСЂРѕС„РёР»СЊ РЅРµ РЅР°Р№РґРµРЅ.")
	case "rename":
		if strings.TrimSpace(extra) == "" {
			reply(ctx.Bot, ctx.ChatID, "РќРѕРІРѕРµ РёРјСЏ РЅРµ Р·Р°РґР°РЅРѕ.")
			return
		}
		if err := client.renameClient(peerID, extra); err != nil {
			reply(ctx.Bot, ctx.ChatID, "РќРµ СѓРґР°Р»РѕСЃСЊ РїРµСЂРµРёРјРµРЅРѕРІР°С‚СЊ:\n"+truncate(err.Error(), 3500))
			return
		}
		reply(ctx.Bot, ctx.ChatID, "OK: renamed")
	default:
		reply(ctx.Bot, ctx.ChatID, "РќРµРёР·РІРµСЃС‚РЅРѕРµ РґРµР№СЃС‚РІРёРµ.")
	}
}
