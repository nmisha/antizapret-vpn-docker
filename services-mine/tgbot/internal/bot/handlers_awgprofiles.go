package bot

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	qrcode "github.com/skip2/go-qrcode"
)

func RegisterAwgProfilesHandlers(reg *CommandRegistry) {
	reg.Command(CommandSpec{Cmd: "/awg_profiles", Desc: "UI: мои Amnezia WireGuard профили", Section: "Amnezia WireGuard", NeedAny: []Role{RoleAwgUserControl}}, handleAwgProfiles,
		RequireRole(RoleAwgUserControl, "Недостаточно прав. Нужна роль AwgUserControl (или Admin)."),
	)
	reg.Alias("awg_profiles", "/awg_profiles")

	reg.Command(CommandSpec{Cmd: "/awg_profiles_admin", Desc: "UI: Amnezia WireGuard профили (admin)", Section: "Админ: Amnezia WireGuard", NeedAny: []Role{RoleAdmin}}, handleAwgProfilesAdmin,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
	)
	reg.Alias("awg_profiles_admin", "/awg_profiles_admin")
}

func handleAwgProfiles(ctx *Ctx, _ string) {
	if !ctx.IsPrivate {
		reply(ctx.Bot, ctx.ChatID, "Управление Amnezia WireGuard профилями доступно только в личных сообщениях боту.")
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
	filtered := filterPeersByUserPrefixes(peers, ctx.User.WgProfiles)
	if len(filtered) == 0 {
		reply(ctx.Bot, ctx.ChatID, "Не найдено ни одного Amnezia WireGuard профиля по вашим правилам из users.json (wg_profiles).")
		return
	}
	sendAwgProfilesList(ctx, filtered, "awg:u:p:")
}

func handleAwgProfilesAdmin(ctx *Ctx, _ string) {
	if !ctx.IsPrivate {
		reply(ctx.Bot, ctx.ChatID, "Админ-управление Amnezia WireGuard профилями доступно только в личных сообщениях боту.")
		return
	}
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("My", "awg:a:scope:my"),
			tgbotapi.NewInlineKeyboardButtonData("User", "awg:a:scope:user"),
			tgbotapi.NewInlineKeyboardButtonData("All", "awg:a:scope:all"),
		),
	)
	m := tgbotapi.NewMessage(ctx.ChatID, "Select scope:")
	m.ReplyMarkup = kb
	_, err := ctx.Bot.Send(m)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", "inline send")
}

func sendAwgProfilesList(ctx *Ctx, peers []wgEasyPeer, pickPrefix string) {
	sort.Slice(peers, func(i, j int) bool {
		return strings.ToLower(peers[i].Name) < strings.ToLower(peers[j].Name)
	})
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, (len(peers)+1)/2)
	for i := 0; i < len(peers); i += 2 {
		row := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(peers[i].Name, pickPrefix+string(peers[i].ID)),
		}
		if i+1 < len(peers) {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(peers[i+1].Name, pickPrefix+string(peers[i+1].ID)))
		}
		rows = append(rows, row)
	}
	rows = append(rows, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("Cancel", "ui:cancel"),
	})
	kb := tgbotapi.NewInlineKeyboardMarkup(rows...)
	m := tgbotapi.NewMessage(ctx.ChatID, "Select profile:")
	m.ReplyMarkup = kb
	_, err := ctx.Bot.Send(m)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", "inline send")
}

func sendAwgUsersList(ctx *Ctx) {
	users, err := ctx.UsersStore.ListUsers()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка чтения users.json: "+err.Error())
		return
	}
	sort.Slice(users, func(i, j int) bool { return users[i].Name < users[j].Name })
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(users))
	for _, u := range users {
		label := u.Name
		if u.TelegramID == ctx.TgID {
			label += " (me)"
		}
		rows = append(rows, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(label, "awg:a:user:"+u.Name),
		})
	}
	rows = append(rows, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("Cancel", "ui:cancel"),
	})
	kb := tgbotapi.NewInlineKeyboardMarkup(rows...)
	m := tgbotapi.NewMessage(ctx.ChatID, "Select user:")
	m.ReplyMarkup = kb
	_, err = ctx.Bot.Send(m)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", "inline send")
}

func getAwgPeersForScope(ctx *Ctx, scope wgAdminScope) ([]wgEasyPeer, error) {
	client, err := makeAwgClientFromEnv()
	if err != nil {
		return nil, err
	}
	peers, err := client.listPeers()
	if err != nil {
		return nil, err
	}
	switch scope.Mode {
	case wgScopeAll:
		return peers, nil
	case wgScopeMy:
		return filterPeersByUserPrefixes(peers, ctx.User.WgProfiles), nil
	case wgScopeUser:
		u, ok, err := ctx.UsersStore.GetByName(scope.UserName)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("пользователь не найден: %s", scope.UserName)
		}
		return filterPeersByUserPrefixes(peers, u.WgProfiles), nil
	default:
		return nil, fmt.Errorf("unknown scope")
	}
}

func sendAwgAdminProfileActions(ctx *Ctx, peerID string) {
	rows := [][]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Stat", "awg:a:act:stats:"+peerID),
			tgbotapi.NewInlineKeyboardButtonData("Profile", "awg:a:act:conf:"+peerID),
			tgbotapi.NewInlineKeyboardButtonData("QR", "awg:a:act:qr:"+peerID),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Risk", "awg:a:act:score:"+peerID),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Enable", "awg:a:act:enable:"+peerID),
			tgbotapi.NewInlineKeyboardButtonData("Disable", "awg:a:act:disable:"+peerID),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Rename", "awg:a:act:rename:"+peerID),
			tgbotapi.NewInlineKeyboardButtonData("Delete", "awg:a:act:delete:"+peerID),
		),
	}
	if ctx.User.Has(RoleWgUserControl) {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Migrate to WG", "awg:a:act:migrate_wg:"+peerID),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Add profile", "awg:a:act:add:"),
		tgbotapi.NewInlineKeyboardButtonData("Back", "awg:a:back"),
	))
	kb := tgbotapi.NewInlineKeyboardMarkup(rows...)
	m := tgbotapi.NewMessage(ctx.ChatID, "Select action:")
	m.ReplyMarkup = kb
	_, err := ctx.Bot.Send(m)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", "inline send")
}

func sendAwgUserProfileActions(ctx *Ctx, peerID string) {
	rows := [][]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Stat", "awg:u:act:stats:"+peerID),
			tgbotapi.NewInlineKeyboardButtonData("Profile", "awg:u:act:conf:"+peerID),
			tgbotapi.NewInlineKeyboardButtonData("QR", "awg:u:act:qr:"+peerID),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Risk", "awg:u:act:score:"+peerID),
		),
	}
	if ctx.User.Has(RoleWgUserControl) {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Migrate to WG", "awg:u:act:migrate_wg:"+peerID),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Cancel", "ui:cancel"),
	))
	kb := tgbotapi.NewInlineKeyboardMarkup(rows...)
	m := tgbotapi.NewMessage(ctx.ChatID, "Select action:")
	m.ReplyMarkup = kb
	_, err := ctx.Bot.Send(m)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", "inline send")
}

func sendAwgConfigAsFile(ctx *Ctx, client *wgEasyClient, peerID string) {
	data, filename, err := client.getConfiguration(peerID)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Не удалось получить конфигурацию:\n"+truncate(err.Error(), 3500))
		return
	}
	doc := tgbotapi.NewDocument(ctx.ChatID, tgbotapi.FileBytes{Name: normalizeWgConfigFilename(filename), Bytes: data})
	doc.Caption = "Amnezia WireGuard profile configuration"
	_, err = ctx.Bot.Send(doc)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", "inline send")
}

func sendAwgQRCode(ctx *Ctx, client *wgEasyClient, peerID string) {
	confData, _, err := client.getConfiguration(peerID)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Failed to get QR data:\n"+truncate(err.Error(), 3500))
		return
	}
	qrPayload := normalizeWgQRPayload(confData)
	if qrPayload == "" {
		reply(ctx.Bot, ctx.ChatID, "Failed to generate QR:\nempty Amnezia WireGuard configuration")
		return
	}
	pngData, err := qrcode.Encode(qrPayload, qrcode.High, 2048)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Failed to generate QR:\n"+truncate(err.Error(), 3500))
		return
	}
	photo := tgbotapi.NewPhoto(ctx.ChatID, tgbotapi.FileBytes{Name: "qrcode.png", Bytes: pngData})
	caption := "Amnezia WireGuard QR"
	if peers, listErr := client.listPeers(); listErr == nil {
		for _, p := range peers {
			if string(p.ID) == peerID && strings.TrimSpace(p.Name) != "" {
				caption = p.Name
				break
			}
		}
	}
	photo.Caption = caption
	_, err = ctx.Bot.Send(photo)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", "inline send")
}

func sendAwgStatsForPeerID(ctx *Ctx, client *wgEasyClient, peerID string) {
	peers, err := client.listPeers()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Не удалось получить статистику:\n"+truncate(err.Error(), 3500))
		return
	}
	for _, p := range peers {
		if string(p.ID) == peerID {
			replyHTML(ctx.Bot, ctx.ChatID, truncate(formatWgPeersStats([]wgEasyPeer{p}), 3800))
			return
		}
	}
	reply(ctx.Bot, ctx.ChatID, "Профиль не найден.")
}

func normalizeAwgQRPayload(confData []byte) string {
	trimmed := bytes.TrimPrefix(confData, []byte{0xEF, 0xBB, 0xBF})
	payload := strings.ReplaceAll(string(trimmed), "\r\n", "\n")
	return strings.TrimSpace(payload)
}
