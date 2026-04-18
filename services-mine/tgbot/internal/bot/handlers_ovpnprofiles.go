package bot

import (
	"fmt"
	"sort"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const ovpnCbPrefix = "ovpn:"

func RegisterOvpnProfilesHandlers(reg *CommandRegistry) {
	reg.Command(CommandSpec{Cmd: "/ovpn_profiles", Desc: "UI: мои OpenVPN профили", Section: "OpenVPN", NeedAny: []Role{RoleOvpnUserControl}}, handleOvpnProfiles,
		RequireRole(RoleOvpnUserControl, "Недостаточно прав. Нужна роль OvpnUserControl (или Admin)."),
		RequirePrivateWithOpenDM("Управление OpenVPN профилями доступно только в личных сообщениях с ботом."),
	)
	reg.Alias("ovpn_profiles", "/ovpn_profiles")

	reg.Command(CommandSpec{Cmd: "/ovpn_profiles_admin", Desc: "UI: OpenVPN профили (admin)", Section: "Админ: OpenVPN", NeedAny: []Role{RoleAdmin}}, handleOvpnProfilesAdmin,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequirePrivateWithOpenDM("Управление OpenVPN профилями доступно только в личных сообщениях с ботом."),
	)
	reg.Alias("ovpn_profiles_admin", "/ovpn_profiles_admin")
}

func handleOvpnProfiles(ctx *Ctx, _ string) {
	client, err := newOvpnUIClientFromEnv()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, err.Error())
		return
	}
	profiles, err := client.listProfiles()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Не удалось получить список OpenVPN профилей:\n"+truncate(err.Error(), 3500))
		return
	}
	filtered := filterOvpnProfilesByUserPrefixes(profiles, ctx.User.OvpnProfiles)
	if len(filtered) == 0 {
		reply(ctx.Bot, ctx.ChatID, "Не найдено ни одного OpenVPN профиля по вашим правилам.")
		return
	}
	sendOvpnProfilesList(ctx, filtered, "ovpn:u:p:")
}

func handleOvpnProfilesAdmin(ctx *Ctx, _ string) {
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("My", "ovpn:a:scope:my"),
			tgbotapi.NewInlineKeyboardButtonData("User", "ovpn:a:scope:user"),
			tgbotapi.NewInlineKeyboardButtonData("All", "ovpn:a:scope:all"),
		),
	)
	m := tgbotapi.NewMessage(ctx.ChatID, "Select scope:")
	m.ReplyMarkup = kb
	_, err := ctx.Bot.Send(m)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", "inline send")
}

func sendOvpnProfilesList(ctx *Ctx, profiles []ovpnProfile, pickPrefix string) {
	sort.Slice(profiles, func(i, j int) bool {
		return strings.ToLower(profiles[i].Name) < strings.ToLower(profiles[j].Name)
	})
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, (len(profiles)+1)/2)
	for i := 0; i < len(profiles); i += 2 {
		row := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(profiles[i].Name, pickPrefix+profiles[i].Name),
		}
		if i+1 < len(profiles) {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(profiles[i+1].Name, pickPrefix+profiles[i+1].Name))
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

func sendOvpnUsersList(ctx *Ctx) {
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
			tgbotapi.NewInlineKeyboardButtonData(label, "ovpn:a:user:"+u.Name),
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

func getOvpnProfilesForScope(ctx *Ctx, scope wgAdminScope) ([]ovpnProfile, error) {
	client, err := newOvpnUIClientFromEnv()
	if err != nil {
		return nil, err
	}
	profiles, err := client.listProfiles()
	if err != nil {
		return nil, err
	}
	switch scope.Mode {
	case wgScopeAll:
		return profiles, nil
	case wgScopeMy:
		return filterOvpnProfilesByUserPrefixes(profiles, ctx.User.OvpnProfiles), nil
	case wgScopeUser:
		u, ok, err := ctx.UsersStore.GetByName(scope.UserName)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("пользователь не найден: %s", scope.UserName)
		}
		return filterOvpnProfilesByUserPrefixes(profiles, u.OvpnProfiles), nil
	default:
		return nil, fmt.Errorf("unknown scope")
	}
}

func sendOvpnProfileActions(ctx *Ctx, profileName, scope string) {
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Risk score", scope+":act:score:"+profileName),
			tgbotapi.NewInlineKeyboardButtonData("Profile", scope+":act:conf:"+profileName),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Back", scope+":back"),
			tgbotapi.NewInlineKeyboardButtonData("Cancel", "ui:cancel"),
		),
	)
	m := tgbotapi.NewMessage(ctx.ChatID, "Select action:")
	m.ReplyMarkup = kb
	_, err := ctx.Bot.Send(m)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", "inline send")
}

func sendOvpnConfigAsFile(ctx *Ctx, client *ovpnUIClient, profileName string) {
	data, filename, err := client.downloadProfile(profileName)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Не удалось получить OpenVPN конфигурацию:\n"+truncate(err.Error(), 3500))
		return
	}
	doc := tgbotapi.NewDocument(ctx.ChatID, tgbotapi.FileBytes{Name: filename, Bytes: data})
	doc.Caption = "OpenVPN profile configuration"
	_, err = ctx.Bot.Send(doc)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", "inline send")
}

func filterOvpnProfilesByUserPrefixes(profiles []ovpnProfile, allowed []string) []ovpnProfile {
	if len(allowed) == 0 {
		return nil
	}
	out := make([]ovpnProfile, 0, len(profiles))
	for _, p := range profiles {
		name := strings.ToLower(strings.TrimSpace(p.Name))
		for _, prefix := range allowed {
			pp := strings.ToLower(strings.TrimSpace(prefix))
			if pp != "" && strings.HasPrefix(name, pp) {
				out = append(out, p)
				break
			}
		}
	}
	return out
}
