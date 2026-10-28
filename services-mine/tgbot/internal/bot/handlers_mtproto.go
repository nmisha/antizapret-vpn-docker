package bot

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	qrcode "github.com/skip2/go-qrcode"
)

const mtpCbPrefix = "mtp:"

// Telegram limits callback data to 64 bytes; the longest prefix is "confirm:mtp:a:rotate:" (21).
const mtpMaxButtonName = 40

// Names created from the bot are shorter, so every action button fits.
const mtpMaxNewName = 32

// mtprotoEnabled: the MTProto section exists only with TELEMT_TOKEN set. Commands are registered
// at startup; callbacks check it again to answer buttons from old messages.
func mtprotoEnabled() bool { return envTrim("TELEMT_TOKEN") != "" }

const mtprotoDisabledText = "Раздел MTProto выключен."

func RegisterMtprotoHandlers(reg *CommandRegistry) {
	if !mtprotoEnabled() {
		log.Printf("MTProto: TELEMT_TOKEN не задан, раздел /mtproto выключен")
		return
	}
	reg.Command(CommandSpec{Cmd: "/mtproto", Desc: "UI: мои MTProto прокси для Telegram", Section: secMtproto, NeedAny: []Role{RoleMtprotoUserControl}}, handleMtproto,
		RequirePrivateWithOpenDM("Ссылки на MTProto прокси выдаются только в личных сообщениях с ботом."),
	)
	reg.Alias("mtproto", "/mtproto")

	reg.Command(CommandSpec{Cmd: "/mtproto_admin", Desc: "UI: MTProto прокси (admin)", Section: secAdminMtproto, NeedAny: []Role{RoleAdmin}}, handleMtprotoAdmin,
		RequirePrivateWithOpenDM("Управление MTProto прокси доступно только в личных сообщениях с ботом."),
	)
	reg.Alias("mtproto_admin", "/mtproto_admin")
}

func handleMtproto(ctx *Ctx, _ string) {
	client, err := newTelemtClientFromEnv()
	if err != nil {
		replyServiceError(ctx, "MTProto", "Не удалось получить список прокси", err)
		return
	}
	users, err := client.listUsers()
	if err != nil {
		replyServiceError(ctx, "MTProto", "Не удалось получить список прокси", err)
		return
	}
	filtered := filterMtpUsersByPrefixes(users, ctx.User.MtprotoProfiles)
	if len(filtered) == 0 {
		reply(ctx.Bot, ctx.ChatID, "Не найдено ни одного MTProto прокси по вашим правилам из users.json (mtproto_profiles).")
		return
	}
	sendMtpUsersList(ctx, filtered, "mtp:u:p:", false)
}

func handleMtprotoAdmin(ctx *Ctx, _ string) {
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("My", "mtp:a:scope:my"),
			tgbotapi.NewInlineKeyboardButtonData("User", "mtp:a:scope:user"),
			tgbotapi.NewInlineKeyboardButtonData("All", "mtp:a:scope:all"),
		),
	)
	m := tgbotapi.NewMessage(ctx.ChatID, "Select scope:")
	m.ReplyMarkup = kb
	_, err := ctx.Bot.Send(m)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", "inline send")
}

func filterMtpUsersByPrefixes(users []telemtUser, allowed []string) []telemtUser {
	out := make([]telemtUser, 0, len(users))
	for _, u := range users {
		if canAccessOvpnProfileByPrefixes(u.Username, allowed) {
			out = append(out, u)
		}
	}
	return out
}

func sendMtpUsersList(ctx *Ctx, users []telemtUser, pickPrefix string, admin bool) {
	sort.Slice(users, func(i, j int) bool {
		return strings.ToLower(users[i].Username) < strings.ToLower(users[j].Username)
	})

	// Names that do not fit into callback data cannot get buttons.
	hidden := 0
	shown := make([]telemtUser, 0, len(users))
	for _, u := range users {
		if len(u.Username) > mtpMaxButtonName {
			hidden++
			continue
		}
		shown = append(shown, u)
	}

	label := func(u telemtUser) string {
		if !u.Enabled {
			return "⛔ " + u.Username
		}
		if u.CurrentConnections > 0 {
			return "🟢 " + u.Username
		}
		return u.Username
	}

	rows := make([][]tgbotapi.InlineKeyboardButton, 0, (len(shown)+1)/2+2)
	for i := 0; i < len(shown); i += 2 {
		row := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(label(shown[i]), pickPrefix+shown[i].Username),
		}
		if i+1 < len(shown) {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(label(shown[i+1]), pickPrefix+shown[i+1].Username))
		}
		rows = append(rows, row)
	}
	if admin {
		rows = append(rows, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData("Add proxy", "mtp:a:add"),
		})
	}
	rows = append(rows, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("Cancel", "ui:cancel"),
	})

	text := "Select proxy:"
	if len(shown) == 0 {
		text = "Прокси не найдены."
	}
	if hidden > 0 {
		text += fmt.Sprintf("\n\nСкрыто с именем длиннее %d символов: %d (управление через telemt-users.sh).", mtpMaxButtonName, hidden)
	}
	m := tgbotapi.NewMessage(ctx.ChatID, text)
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	_, err := ctx.Bot.Send(m)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", "inline send")
}

func sendMtpBotUsersList(ctx *Ctx) {
	users, err := ctx.UsersStore.ListUsers()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка чтения users.json: "+err.Error())
		return
	}
	sort.Slice(users, func(i, j int) bool { return users[i].Name < users[j].Name })

	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(users)+1)
	for _, u := range users {
		label := u.Name
		if u.TelegramID == ctx.TgID {
			label += " (me)"
		}
		rows = append(rows, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(label, "mtp:a:user:"+u.Name),
		})
	}
	rows = append(rows, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("Cancel", "ui:cancel"),
	})

	m := tgbotapi.NewMessage(ctx.ChatID, "Select user:")
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	_, err = ctx.Bot.Send(m)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", "inline send")
}

func getMtpUsersForScope(ctx *Ctx, client *telemtClient, scope wgAdminScope) ([]telemtUser, error) {
	users, err := client.listUsers()
	if err != nil {
		return nil, err
	}
	switch scope.Mode {
	case wgScopeAll:
		return users, nil
	case wgScopeMy:
		return filterMtpUsersByPrefixes(users, ctx.User.MtprotoProfiles), nil
	case wgScopeUser:
		u, ok, err := ctx.UsersStore.GetByName(scope.UserName)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("пользователь не найден: %s", scope.UserName)
		}
		return filterMtpUsersByPrefixes(users, u.MtprotoProfiles), nil
	default:
		return nil, fmt.Errorf("unknown scope")
	}
}

func findMtpUser(users []telemtUser, name string) (telemtUser, bool) {
	for _, u := range users {
		if u.Username == name {
			return u, true
		}
	}
	return telemtUser{}, false
}

func validateMtpUserAccess(ctx *Ctx, client *telemtClient, name string) (telemtUser, error) {
	if !ctx.User.Has(RoleMtprotoUserControl) {
		return telemtUser{}, userErrorf("недостаточно прав")
	}
	if !canAccessOvpnProfileByPrefixes(name, ctx.User.MtprotoProfiles) && !ctx.User.Has(RoleAdmin) {
		return telemtUser{}, userErrorf("доступ к прокси отозван")
	}
	u, err := client.getUser(name)
	if isTelemtNotFound(err) {
		return telemtUser{}, userErrorf("прокси больше не найден")
	}
	if err != nil {
		return telemtUser{}, err
	}
	return u, nil
}

func validateMtpAdminAccess(ctx *Ctx, client *telemtClient, name string) (telemtUser, error) {
	if !ctx.User.Has(RoleAdmin) {
		return telemtUser{}, userErrorf("недостаточно прав")
	}
	scope, ok := getMtpAdminScope(ctx.TgID)
	if !ok || scope.Mode == "" {
		return telemtUser{}, userErrorf("кнопка устарела, открой список прокси заново")
	}
	users, err := getMtpUsersForScope(ctx, client, scope)
	if err != nil {
		return telemtUser{}, err
	}
	u, ok := findMtpUser(users, name)
	if !ok {
		return telemtUser{}, userErrorf("прокси больше не доступен в текущем scope")
	}
	return u, nil
}

func formatMtpLimit(v *uint64, format func(uint64) string) string {
	if v == nil {
		return "по умолчанию"
	}
	return format(*v)
}

func formatMtpBits(v uint64) string {
	switch {
	case v >= 1_000_000:
		return fmt.Sprintf("%.1f Мбит/с", float64(v)/1e6)
	case v >= 1_000:
		return fmt.Sprintf("%.0f Кбит/с", float64(v)/1e3)
	default:
		return fmt.Sprintf("%d бит/с", v)
	}
}

func formatMtpExpiration(v *string, now time.Time) string {
	if v == nil || strings.TrimSpace(*v) == "" {
		return "бессрочно"
	}
	t, err := time.Parse(time.RFC3339, *v)
	if err != nil {
		return *v
	}
	s := t.Local().Format("2006-01-02 15:04")
	if t.Before(now) {
		return s + " (истёк)"
	}
	return s
}

func formatMtpUserInfo(u telemtUser, admin bool, now time.Time) string {
	num := func(v uint64) string { return fmt.Sprintf("%d", v) }
	bytes := func(v uint64) string { return humanBytesIEC(uint64ToInt64(v)) }

	state := "включён"
	if !u.Enabled {
		state = "⛔ отключён"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "<b>%s</b> — %s\n", htmlEscape(u.Username), state)
	fmt.Fprintf(&b, "Соединения: %d / %s\n", u.CurrentConnections, formatMtpLimit(u.MaxTCPConns, num))
	fmt.Fprintf(&b, "Устройства (IP): %d / %s\n", u.ActiveUniqueIPs, formatMtpLimit(u.MaxUniqueIPs, num))
	fmt.Fprintf(&b, "Трафик: %s", bytes(u.TotalOctets))
	if u.DataQuotaBytes != nil {
		fmt.Fprintf(&b, " / квота %s", bytes(*u.DataQuotaBytes))
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "Срок: %s\n", htmlEscape(formatMtpExpiration(u.ExpirationRFC3339, now)))
	if u.RateLimitUpBps != nil || u.RateLimitDownBps != nil {
		fmt.Fprintf(&b, "Скорость: ↑ %s, ↓ %s\n",
			formatMtpLimit(u.RateLimitUpBps, formatMtpBits), formatMtpLimit(u.RateLimitDownBps, formatMtpBits))
	}
	if admin && len(u.ActiveUniqueIPsList) > 0 {
		fmt.Fprintf(&b, "Активные IP: <code>%s</code>\n", htmlEscape(strings.Join(u.ActiveUniqueIPsList, ", ")))
	}
	return b.String()
}

func sendMtpUserActions(ctx *Ctx, u telemtUser, admin bool) {
	text := formatMtpUserInfo(u, admin, time.Now())
	name := u.Username

	var rows [][]tgbotapi.InlineKeyboardButton
	if admin {
		toggle := tgbotapi.NewInlineKeyboardButtonData("Disable", "mtp:a:x:off:"+name)
		if !u.Enabled {
			toggle = tgbotapi.NewInlineKeyboardButtonData("Enable", "mtp:a:x:on:"+name)
		}
		rows = append(rows,
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Link + QR", "mtp:a:x:link:"+name),
				tgbotapi.NewInlineKeyboardButtonData("Limits", "mtp:a:x:lim:"+name),
			),
			tgbotapi.NewInlineKeyboardRow(
				toggle,
				tgbotapi.NewInlineKeyboardButtonData("Reset quota", "mtp:a:x:rq:"+name),
			),
			// No Delete: telemt-panel keeps per-user state (quota auto-reset, history, WEB profiles)
			// that only its own delete cleans up. Disable covers the bot's needs.
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("New secret", "mtp:a:x:rot:"+name),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Back", "mtp:a:back"),
				tgbotapi.NewInlineKeyboardButtonData("Cancel", "ui:cancel"),
			),
		)
	} else {
		rows = append(rows,
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Link + QR", "mtp:u:x:link:"+name),
				tgbotapi.NewInlineKeyboardButtonData("New secret", "mtp:u:x:rot:"+name),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Back", "mtp:u:back"),
				tgbotapi.NewInlineKeyboardButtonData("Cancel", "ui:cancel"),
			),
		)
	}

	m := tgbotapi.NewMessage(ctx.ChatID, text)
	m.ParseMode = "HTML"
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	_, err := ctx.Bot.Send(m)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", "inline send")
}

func sendMtpLimitsMenu(ctx *Ctx, u telemtUser) {
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(mtpLimitFields)/2+2)
	for i := 0; i < len(mtpLimitFields); i += 2 {
		row := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(mtpLimitFields[i].Label, "mtp:a:l:"+mtpLimitFields[i].Key+":"+u.Username),
		}
		if i+1 < len(mtpLimitFields) {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(mtpLimitFields[i+1].Label, "mtp:a:l:"+mtpLimitFields[i+1].Key+":"+u.Username))
		}
		rows = append(rows, row)
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Back", "mtp:a:p:"+u.Username),
		tgbotapi.NewInlineKeyboardButtonData("Cancel", "ui:cancel"),
	))

	m := tgbotapi.NewMessage(ctx.ChatID, formatMtpUserInfo(u, true, time.Now())+"\nКакой лимит изменить?")
	m.ParseMode = "HTML"
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	_, err := ctx.Bot.Send(m)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", "inline send")
}

// sendMtpLink sends the proxy link as text with a connect button, and as a QR code.
func sendMtpLink(ctx *Ctx, u telemtUser) {
	link := telemtWebLink(u.TLSLink())
	if link == "" {
		replyServiceError(ctx, "MTProto", "Не удалось получить ссылку", fmt.Errorf("у прокси %s нет fake-TLS ссылки: проверьте [general.links] в config.toml telemt", u.Username))
		return
	}

	text := fmt.Sprintf("MTProto прокси <b>%s</b>\n\n<code>%s</code>\n\nНажмите «Подключить» или отсканируйте QR. Ссылка содержит секрет — не пересылайте её.",
		htmlEscape(u.Username), htmlEscape(link))
	if !u.Enabled {
		text += "\n\n⛔ Прокси сейчас отключён."
	}
	m := tgbotapi.NewMessage(ctx.ChatID, text)
	m.ParseMode = "HTML"
	m.DisableWebPagePreview = true
	m.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonURL("Подключить", link)),
	)
	_, err := ctx.Bot.Send(m)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", "mtproto link")

	pngData, err := qrcode.Encode(link, qrcode.Medium, 1024)
	if err != nil {
		replyServiceError(ctx, "MTProto", "Не удалось сделать QR-код", err)
		return
	}
	photo := tgbotapi.NewPhoto(ctx.ChatID, tgbotapi.FileBytes{Name: "qrcode.png", Bytes: pngData})
	photo.Caption = u.Username
	_, err = ctx.Bot.Send(photo)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", "mtproto qr")
}
