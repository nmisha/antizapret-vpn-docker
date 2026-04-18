package bot

import (
	"bytes"
	"fmt"
	"html"
	"sort"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	qrcode "github.com/skip2/go-qrcode"
)

const (
	wgCbPrefix = "wg:"
)

// /wg_profiles (WgUserControl) - user menu
func RegisterWgProfilesHandlers(reg *CommandRegistry) {
	reg.Command(CommandSpec{Cmd: "/wg_profiles", Desc: "UI: мои WireGuard профили", Section: "WireGuard", NeedAny: []Role{RoleWgUserControl}}, handleWgProfiles,
		RequireRole(RoleWgUserControl, "Недостаточно прав. Нужна роль WgUserControl (или Admin)."),
	)
	reg.Alias("wg_profiles", "/wg_profiles")

	// admin menu
	reg.Command(CommandSpec{Cmd: "/wg_profiles_admin", Desc: "UI: WireGuard профили (admin)", Section: "Админ: WireGuard", NeedAny: []Role{RoleAdmin}}, handleWgProfilesAdmin,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
	)
	reg.Alias("wg_profiles_admin", "/wg_profiles_admin")
}

func handleWgProfiles(ctx *Ctx, _ string) {
	if !ctx.IsPrivate {
		reply(ctx.Bot, ctx.ChatID, "Управление WireGuard профилями доступно только в личных сообщениях боту.")
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

	filtered := filterPeersByUserPrefixes(peers, ctx.User.WgProfiles)
	if len(filtered) == 0 {
		reply(ctx.Bot, ctx.ChatID, "Не найдено ни одного WireGuard профиля по вашим правилам из users.json (wg_profiles).")
		return
	}

	sendWgProfilesList(ctx, filtered, "wg:u:p:")
}

func handleWgProfilesAdmin(ctx *Ctx, _ string) {
	if !ctx.IsPrivate {
		reply(ctx.Bot, ctx.ChatID, "Админ-управление WireGuard профилями доступно только в личных сообщениях боту.")
		return
	}

	// Step 1: choose scope
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("My", "wg:a:scope:my"),
			tgbotapi.NewInlineKeyboardButtonData("User", "wg:a:scope:user"),
			tgbotapi.NewInlineKeyboardButtonData("All", "wg:a:scope:all"),
		),
	)
	m := tgbotapi.NewMessage(ctx.ChatID, "Select scope:")
	m.ReplyMarkup = kb
	_, err := ctx.Bot.Send(m)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", "inline send")
}

func sendWgProfilesList(ctx *Ctx, peers []wgEasyPeer, pickPrefix string) {
	// sort by name for menu readability
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

func makeWgClientFromEnv() (*wgEasyClient, error) {
	host := envTrim("WG_HOST")
	port := envTrim("WG_PORT")
	username := envTrim("WG_USERNAME")
	pass := envTrim("WG_PASSWORD")
	client, err := newWgEasyClient(host, port, username, pass)
	if err != nil {
		return nil, fmt.Errorf("Не заданы переменные окружения WireGuard. Нужно: WG_HOST, WG_PORT, WG_PASSWORD. Для wg-easy v15 обычно также нужен WG_USERNAME")
	}
	return client, nil
}

// used by admin "User" selection UI
func sendWgUsersList(ctx *Ctx) {
	users, err := ctx.UsersStore.ListUsers()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка чтения users.json: "+err.Error())
		return
	}
	// sort by name
	sort.Slice(users, func(i, j int) bool { return users[i].Name < users[j].Name })

	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(users))
	for _, u := range users {
		label := u.Name
		if u.TelegramID == ctx.TgID {
			label += " (me)"
		}
		rows = append(rows, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(label, "wg:a:user:"+u.Name),
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

// helper used for admin scope selection
func getPeersForScope(ctx *Ctx, scope wgAdminScope) ([]wgEasyPeer, error) {
	client, err := makeWgClientFromEnv()
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
			return nil, fmt.Errorf("Пользователь не найден: %s", scope.UserName)
		}
		return filterPeersByUserPrefixes(peers, u.WgProfiles), nil
	default:
		return nil, fmt.Errorf("unknown scope")
	}
}

// send admin profile actions menu for a selected profile
func sendAdminProfileActions(ctx *Ctx, peerID string) {
	rows := [][]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Stat", "wg:a:act:stats:"+peerID),
			tgbotapi.NewInlineKeyboardButtonData("Profile", "wg:a:act:conf:"+peerID),
			tgbotapi.NewInlineKeyboardButtonData("QR", "wg:a:act:qr:"+peerID),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Risk score", "wg:a:act:score:"+peerID),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Enable", "wg:a:act:enable:"+peerID),
			tgbotapi.NewInlineKeyboardButtonData("Disable", "wg:a:act:disable:"+peerID),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Rename", "wg:a:act:rename:"+peerID),
			tgbotapi.NewInlineKeyboardButtonData("Delete", "wg:a:act:delete:"+peerID),
		),
	}
	if ctx.User.Has(RoleAwgUserControl) {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Migrate to AWG", "wg:a:act:migrate_awg:"+peerID),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Add profile", "wg:a:act:add:"),
		tgbotapi.NewInlineKeyboardButtonData("Back", "wg:a:back"),
	))
	kb := tgbotapi.NewInlineKeyboardMarkup(rows...)
	m := tgbotapi.NewMessage(ctx.ChatID, "Select action:")
	m.ReplyMarkup = kb
	_, err := ctx.Bot.Send(m)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", "inline send")
}

// send user profile actions menu for a selected profile
func sendUserProfileActions(ctx *Ctx, peerID string) {
	rows := [][]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Stat", "wg:u:act:stats:"+peerID),
			tgbotapi.NewInlineKeyboardButtonData("Profile", "wg:u:act:conf:"+peerID),
			tgbotapi.NewInlineKeyboardButtonData("QR", "wg:u:act:qr:"+peerID),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Risk score", "wg:u:act:score:"+peerID),
		),
	}
	if ctx.User.Has(RoleAwgUserControl) {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Migrate to AWG", "wg:u:act:migrate_awg:"+peerID),
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

func sendWgConfigAsFile(ctx *Ctx, client *wgEasyClient, peerID string) {
	data, filename, err := client.getConfiguration(peerID)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Не удалось получить конфигурацию:\n"+truncate(err.Error(), 3500))
		return
	}
	doc := tgbotapi.NewDocument(ctx.ChatID, tgbotapi.FileBytes{Name: normalizeWgConfigFilename(filename), Bytes: data})
	doc.Caption = "WireGuard profile configuration"
	_, err = ctx.Bot.Send(doc)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", "inline send")
}

func sendWgQRCode(ctx *Ctx, client *wgEasyClient, peerID string) {
	confData, _, err := client.getConfiguration(peerID)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Failed to get QR data:\n"+truncate(err.Error(), 3500))
		return
	}

	qrPayload := normalizeWgQRPayload(confData)
	if qrPayload == "" {
		reply(ctx.Bot, ctx.ChatID, "Failed to generate QR:\nempty WireGuard configuration")
		return
	}

	// Generate a larger QR so Telegram photo compression is less likely to break scanning/import.
	pngData, err := qrcode.Encode(qrPayload, qrcode.High, 2048)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Failed to generate QR:\n"+truncate(err.Error(), 3500))
		return
	}

	photo := tgbotapi.NewPhoto(ctx.ChatID, tgbotapi.FileBytes{Name: "qrcode.png", Bytes: pngData})
	caption := "WireGuard QR"
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

func normalizeWgQRPayload(confData []byte) string {
	trimmed := bytes.TrimPrefix(confData, []byte{0xEF, 0xBB, 0xBF})
	payload := strings.ReplaceAll(string(trimmed), "\r\n", "\n")
	payload = strings.TrimSpace(payload)
	return payload
}

func sendWgStatsForPeerID(ctx *Ctx, client *wgEasyClient, peerID string) {
	peers, err := client.listPeers()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Не удалось получить статистику:\n"+truncate(err.Error(), 3500))
		return
	}
	for _, p := range peers {
		if string(p.ID) == peerID {
			msg := formatWgPeersStats([]wgEasyPeer{p})
			replyHTML(ctx.Bot, ctx.ChatID, truncate(msg, 3800))
			return
		}
	}
	reply(ctx.Bot, ctx.ChatID, "Профиль не найден.")
}

func adminScopeLabel(scope wgAdminScope) string {
	switch scope.Mode {
	case wgScopeAll:
		return "All"
	case wgScopeMy:
		return "My"
	case wgScopeUser:
		return "User: " + html.EscapeString(scope.UserName)
	default:
		return "?"
	}
}
