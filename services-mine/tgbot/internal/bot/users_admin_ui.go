package bot

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bot users UI (users.json): list, card, roles, profile prefixes, rename, delete, join requests.
// Users are addressed by telegram_id in callback data: it is short and survives renames.
const usrCbPrefix = "usr:"

var botUserNameRe = regexp.MustCompile(`^[\p{L}\p{N}_.-]{1,32}$`)

func RegisterUsersAdminHandlers(reg *CommandRegistry) {
	reg.Command(CommandSpec{Cmd: "/users_admin", Desc: "UI: пользователи бота, роли, префиксы профилей, заявки", Section: "Админ: пользователи и роли", NeedAny: []Role{RoleAdmin}}, handleUsersAdmin,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequirePrivateWithOpenDM("Управление пользователями доступно только в личных сообщениях с ботом."),
	)
	reg.Alias("users_admin", "/users_admin")

	reg.Command(CommandSpec{Cmd: "/prefix_add", Args: "<name> <wg|ovpn|mtproto> <prefix>", Desc: "добавить префикс профилей пользователю", Section: "Админ: пользователи и роли", NeedAny: []Role{RoleAdmin}}, handlePrefixAdd,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequirePrivateWithOpenDM("Команда /prefix_add доступна только в личных сообщениях с ботом."),
		RequireNonEmptyArg("Формат: /prefix_add <name> <wg|ovpn|mtproto> <prefix>"),
	)
	reg.Alias("prefix_add", "/prefix_add")

	reg.Command(CommandSpec{Cmd: "/prefix_del", Args: "<name> <wg|ovpn|mtproto> <prefix>", Desc: "убрать префикс профилей у пользователя", Section: "Админ: пользователи и роли", NeedAny: []Role{RoleAdmin}}, handlePrefixDel,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequirePrivateWithOpenDM("Команда /prefix_del доступна только в личных сообщениях с ботом."),
		RequireNonEmptyArg("Формат: /prefix_del <name> <wg|ovpn|mtproto> <prefix>"),
	)
	reg.Alias("prefix_del", "/prefix_del")

	reg.Command(CommandSpec{Cmd: "/user_disable", Args: "<name>", Desc: "отключить пользователя от бота и уведомлений", Section: "Админ: пользователи и роли", NeedAny: []Role{RoleAdmin}}, handleUserDisable,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequirePrivateWithOpenDM("Команда /user_disable доступна только в личных сообщениях с ботом."),
		RequireNonEmptyArg("Формат: /user_disable <name>"),
	)
	reg.Alias("user_disable", "/user_disable")

	reg.Command(CommandSpec{Cmd: "/user_enable", Args: "<name>", Desc: "снова включить пользователя", Section: "Админ: пользователи и роли", NeedAny: []Role{RoleAdmin}}, handleUserEnable,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequirePrivateWithOpenDM("Команда /user_enable доступна только в личных сообщениях с ботом."),
		RequireNonEmptyArg("Формат: /user_enable <name>"),
	)
	reg.Alias("user_enable", "/user_enable")
}

func handleUserDisable(ctx *Ctx, arg string) { handleUserSetDisabledByName(ctx, arg, true) }
func handleUserEnable(ctx *Ctx, arg string)  { handleUserSetDisabledByName(ctx, arg, false) }

func handleUserSetDisabledByName(ctx *Ctx, arg string, disabled bool) {
	u, ok, err := ctx.UsersStore.GetByName(strings.TrimSpace(arg))
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка чтения users.json: "+err.Error())
		return
	}
	if !ok {
		reply(ctx.Bot, ctx.ChatID, "Пользователь не найден: "+arg)
		return
	}
	msg, err := setUserDisabled(ctx, u, disabled)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
		return
	}
	reply(ctx.Bot, ctx.ChatID, msg)
}

// setUserDisabled switches bot access. Disabling is silent and drops the user's unfinished UI state;
// enabling tells the user that access is back.
func setUserDisabled(ctx *Ctx, u User, disabled bool) (string, error) {
	if disabled && u.TelegramID == ctx.TgID {
		return "", fmt.Errorf("нельзя отключить самого себя")
	}
	if u.Disabled == disabled {
		if disabled {
			return "Пользователь " + u.Name + " уже отключён.", nil
		}
		return "Пользователь " + u.Name + " уже активен.", nil
	}
	if _, err := ctx.UsersStore.SetDisabled(u.Name, disabled); err != nil {
		return "", err
	}

	id := u.TelegramID
	if disabled {
		clearUserUIState(id)
		return fmt.Sprintf("⛔ Пользователь %s (tg_id=%d) отключён: бот отвечает ему как незнакомцу, уведомления не отправляются. VPN-профили и прокси продолжают работать.", u.Name, id), nil
	}

	msg := fmt.Sprintf("✅ Пользователь %s (tg_id=%d) снова активен.", u.Name, id)
	if _, err := ctx.Bot.Send(tgbotapi.NewMessage(id, "✅ Доступ к боту снова открыт. Список команд: /help")); err != nil {
		msg += "\nНе удалось уведомить пользователя: " + err.Error()
	}
	return msg, nil
}

func handleUsersAdmin(ctx *Ctx, _ string) {
	showUsersAdminList(ctx)
}

// parsePrefixArgs splits "<name> <kind> <prefix...>"; the prefix may contain spaces.
func parsePrefixArgs(arg string) (string, profileKind, string, error) {
	f := strings.Fields(arg)
	if len(f) < 3 {
		return "", "", "", fmt.Errorf("нужно три аргумента")
	}
	kind, err := parseProfileKind(f[1])
	if err != nil {
		return "", "", "", err
	}
	return f[0], kind, strings.Join(f[2:], " "), nil
}

func handlePrefixAdd(ctx *Ctx, arg string) {
	name, kind, prefix, err := parsePrefixArgs(arg)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error()+"\nФормат: /prefix_add <name> <wg|ovpn|mtproto> <prefix>")
		return
	}
	u, err := ctx.UsersStore.AddProfilePrefix(name, kind, prefix)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
		return
	}
	reply(ctx.Bot, ctx.ChatID, fmt.Sprintf("Добавлен префикс %s %q пользователю %s. Сейчас: %s",
		kind.Label(), prefix, u.Name, strings.Join(*kind.prefixes(&u), ", ")))
}

func handlePrefixDel(ctx *Ctx, arg string) {
	name, kind, prefix, err := parsePrefixArgs(arg)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error()+"\nФормат: /prefix_del <name> <wg|ovpn|mtproto> <prefix>")
		return
	}
	u, removed, err := ctx.UsersStore.RemoveProfilePrefix(name, kind, prefix)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
		return
	}
	rest := strings.Join(*kind.prefixes(&u), ", ")
	if rest == "" {
		rest = "(нет)"
	}
	reply(ctx.Bot, ctx.ChatID, fmt.Sprintf("Убран префикс %s %q у пользователя %s. Сейчас: %s", kind.Label(), removed, u.Name, rest))
}

func validateBotUserName(raw string) (string, error) {
	name := normalizeName(raw)
	if !botUserNameRe.MatchString(name) {
		return "", fmt.Errorf("имя: буквы, цифры, _ . -, до 32 символов, без пробелов")
	}
	return name, nil
}

// usrShow edits the menu message the button belongs to, or sends a new one.
func usrShow(ctx *Ctx, text string, kb tgbotapi.InlineKeyboardMarkup) {
	if ctx.MessageID != 0 {
		edit := tgbotapi.NewEditMessageTextAndMarkup(ctx.ChatID, ctx.MessageID, text, kb)
		edit.ParseMode = "HTML"
		edit.DisableWebPagePreview = true
		_, err := ctx.Bot.Send(edit)
		if err == nil || strings.Contains(err.Error(), "message is not modified") {
			return
		}
		// e.g. the message is too old to edit: fall back to a new one
	}
	m := tgbotapi.NewMessage(ctx.ChatID, text)
	m.ParseMode = "HTML"
	m.DisableWebPagePreview = true
	m.ReplyMarkup = kb
	_, err := ctx.Bot.Send(m)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", "users admin")
}

func usrCb(parts ...string) string {
	return usrCbPrefix + strings.Join(parts, ":")
}

func idStr(id int64) string { return strconv.FormatInt(id, 10) }

func shortLabel(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max-1]) + "…"
}

func showUsersAdminList(ctx *Ctx) {
	users, err := ctx.UsersStore.ListUsers()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка чтения users.json: "+err.Error())
		return
	}
	sort.Slice(users, func(i, j int) bool { return users[i].Name < users[j].Name })

	label := func(u User) string {
		l := u.Name
		if u.Disabled {
			l = "⛔ " + l
		}
		if u.HasExact(RoleAdmin) {
			l += " 👑"
		}
		if u.TelegramID == ctx.TgID {
			l += " (me)"
		}
		return l
	}
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(users)/2+3)
	for i := 0; i < len(users); i += 2 {
		row := []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(label(users[i]), usrCb("u", idStr(users[i].TelegramID))),
		}
		if i+1 < len(users) {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(label(users[i+1]), usrCb("u", idStr(users[i+1].TelegramID))))
		}
		rows = append(rows, row)
	}
	rows = append(rows,
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➕ Добавить по ID", usrCb("add")),
			tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("🙈 Игнор (%d)", len(getSettingsCached().IgnoredJoinIDs)), usrCb("ign")),
		),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("✖ Закрыть", "ui:cancel")),
	)
	usrShow(ctx, fmt.Sprintf("👥 Пользователи бота: %d", len(users)), tgbotapi.NewInlineKeyboardMarkup(rows...))
}

func formatPrefixList(list []string) string {
	if len(list) == 0 {
		return "—"
	}
	return htmlEscape(strings.Join(list, ", "))
}

func showUserCard(ctx *Ctx, id int64) {
	u, ok, err := ctx.UsersStore.GetByID(id)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка чтения users.json: "+err.Error())
		return
	}
	if !ok {
		reply(ctx.Bot, ctx.ChatID, "Пользователь не найден (возможно, удалён).")
		showUsersAdminList(ctx)
		return
	}

	roles := strings.Join(u.RolesRaw, ", ")
	if roles == "" {
		roles = "(нет)"
	}
	var b strings.Builder
	status := "активен"
	if u.Disabled {
		status = "⛔ отключён (бот отвечает шуткой, уведомления не отправляются)"
	}
	fmt.Fprintf(&b, "👤 <b>%s</b>\ntg_id: <code>%d</code>\nСтатус: %s\nРоли: %s\n\n<b>Префиксы профилей</b>\n",
		htmlEscape(u.Name), u.TelegramID, status, htmlEscape(roles))
	for _, k := range profileKinds {
		fmt.Fprintf(&b, "%s: %s\n", k.Label(), formatPrefixList(*k.prefixes(&u)))
	}

	sid := idStr(id)
	prefixRow := make([]tgbotapi.InlineKeyboardButton, 0, len(profileKinds))
	for _, k := range profileKinds {
		prefixRow = append(prefixRow, tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("%s (%d)", k.Label(), len(*k.prefixes(&u))), usrCb("p", string(k), sid)))
	}
	toggle := tgbotapi.NewInlineKeyboardButtonData("⛔ Отключить", usrCb("dis", sid))
	if u.Disabled {
		toggle = tgbotapi.NewInlineKeyboardButtonData("✅ Включить", usrCb("en", sid))
	}
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🎭 Роли", usrCb("r", sid)),
			tgbotapi.NewInlineKeyboardButtonData("✏️ Переименовать", usrCb("ren", sid)),
		),
		prefixRow,
		tgbotapi.NewInlineKeyboardRow(
			toggle,
			tgbotapi.NewInlineKeyboardButtonData("🗑 Удалить", usrCb("del", sid)),
		),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("← Назад", usrCb("list"))),
	)
	usrShow(ctx, b.String(), kb)
}

func showUserRoles(ctx *Ctx, id int64) {
	u, ok, err := ctx.UsersStore.GetByID(id)
	if err != nil || !ok {
		reply(ctx.Bot, ctx.ChatID, "Пользователь не найден.")
		return
	}
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(roleCatalog)/2+2)
	var row []tgbotapi.InlineKeyboardButton
	for _, rc := range roleCatalog {
		mark := "▫️ "
		if u.HasExact(rc.Role) {
			mark = "✅ "
		}
		row = append(row, tgbotapi.NewInlineKeyboardButtonData(mark+string(rc.Role), usrCb("rt", idStr(id), string(rc.Role))))
		if len(row) == 2 {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("← Назад", usrCb("u", idStr(id)))))
	usrShow(ctx, fmt.Sprintf("🎭 Роли <b>%s</b>: нажми, чтобы выдать или снять.\nAdmin включает все остальные роли. Описание: /roles_catalog", htmlEscape(u.Name)),
		tgbotapi.NewInlineKeyboardMarkup(rows...))
}

func toggleUserRole(ctx *Ctx, id int64, roleRaw string) {
	role, err := normalizeRoleString(roleRaw)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
		return
	}
	u, ok, err := ctx.UsersStore.GetByID(id)
	if err != nil || !ok {
		reply(ctx.Bot, ctx.ChatID, "Пользователь не найден.")
		return
	}
	if u.HasExact(role) {
		if role == RoleAdmin && id == ctx.TgID {
			reply(ctx.Bot, ctx.ChatID, "Нельзя снять Admin с самого себя: можно потерять доступ к боту.")
			return
		}
		_, err = ctx.UsersStore.RevokeByName(u.Name, role)
	} else {
		_, err = ctx.UsersStore.GrantByName(u.Name, role)
	}
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
		return
	}
	showUserRoles(ctx, id)
}

func showUserPrefixes(ctx *Ctx, kind profileKind, id int64) {
	u, ok, err := ctx.UsersStore.GetByID(id)
	if err != nil || !ok {
		reply(ctx.Bot, ctx.ChatID, "Пользователь не найден.")
		return
	}
	list := *kind.prefixes(&u)
	sid := idStr(id)

	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(list)+2)
	for _, p := range list {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✖ "+shortLabel(p, 40), usrCb("pd", string(kind), sid, prefixToken(p)))))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("➕ Добавить", usrCb("pa", string(kind), sid)),
		tgbotapi.NewInlineKeyboardButtonData("← Назад", usrCb("u", sid)),
	))

	text := fmt.Sprintf("<b>%s</b>: префиксы пользователя <b>%s</b>\n%s\n\nПрофиль доступен пользователю, если его имя начинается с одного из префиксов (регистр не важен). "+
		"Чтобы префикс не захватил чужие имена, добавляйте разделитель: <code>vasya_</code>.\nНажми на префикс, чтобы убрать его.",
		kind.Label(), htmlEscape(u.Name), formatPrefixList(list))
	usrShow(ctx, text, tgbotapi.NewInlineKeyboardMarkup(rows...))
}

func showIgnoredJoins(ctx *Ctx) {
	ids := getSettingsCached().IgnoredJoinIDs
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(ids)+1)
	for _, id := range ids {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("↩️ Снять игнор: "+idStr(id), usrCb("unign", idStr(id)))))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("← Назад", usrCb("list"))))
	text := "🙈 Игнор-лист заявок: о сообщениях этих Telegram ID админам не сообщается."
	if len(ids) == 0 {
		text = "🙈 Игнор-лист заявок пуст."
	}
	usrShow(ctx, text, tgbotapi.NewInlineKeyboardMarkup(rows...))
}

func handleUsersAdminCallback(ctx *Ctx, data string) {
	if !ctx.IsPrivate {
		reply(ctx.Bot, ctx.ChatID, "Управление пользователями доступно только в личных сообщениях с ботом.")
		return
	}
	if !ctx.User.Has(RoleAdmin) {
		reply(ctx.Bot, ctx.ChatID, "Недостаточно прав. Нужна роль Admin.")
		return
	}

	parts := strings.Split(strings.TrimPrefix(data, usrCbPrefix), ":")
	arg := func(i int) string {
		if i < len(parts) {
			return parts[i]
		}
		return ""
	}
	argID := func(i int) (int64, bool) {
		id, err := strconv.ParseInt(arg(i), 10, 64)
		if err != nil || id <= 0 {
			reply(ctx.Bot, ctx.ChatID, "Неверный формат.")
			return 0, false
		}
		return id, true
	}
	argKind := func(i int) (profileKind, bool) {
		k, err := parseProfileKind(arg(i))
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Неверный формат.")
			return "", false
		}
		return k, true
	}

	switch parts[0] {
	case "list":
		showUsersAdminList(ctx)

	case "u":
		if id, ok := argID(1); ok {
			showUserCard(ctx, id)
		}

	case "r":
		if id, ok := argID(1); ok {
			showUserRoles(ctx, id)
		}

	case "rt":
		if id, ok := argID(1); ok {
			toggleUserRole(ctx, id, arg(2))
		}

	case "p":
		if k, ok := argKind(1); ok {
			if id, ok := argID(2); ok {
				showUserPrefixes(ctx, k, id)
			}
		}

	case "pa":
		k, ok := argKind(1)
		if !ok {
			return
		}
		id, ok := argID(2)
		if !ok {
			return
		}
		setUsrPending(ctx.TgID, usrPending{Kind: usrPendingPrefix, TargetID: id, Profile: k})
		reply(ctx.Bot, ctx.ChatID, "Отправь новый префикс "+k.Label()+" одним сообщением (например vasya_).\n/cancel — отмена.")

	case "pd":
		k, ok := argKind(1)
		if !ok {
			return
		}
		id, ok := argID(2)
		if !ok {
			return
		}
		u, found, err := ctx.UsersStore.GetByID(id)
		if err != nil || !found {
			reply(ctx.Bot, ctx.ChatID, "Пользователь не найден.")
			return
		}
		if _, _, err := ctx.UsersStore.RemoveProfilePrefixByToken(u.Name, k, arg(3)); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
			return
		}
		showUserPrefixes(ctx, k, id)

	case "ren":
		id, ok := argID(1)
		if !ok {
			return
		}
		setUsrPending(ctx.TgID, usrPending{Kind: usrPendingRename, TargetID: id})
		reply(ctx.Bot, ctx.ChatID, "Отправь новое имя одним сообщением (буквы, цифры, _ . -, до 32 символов). Секция доменов переименуется вместе с пользователем.\n/cancel — отмена.")

	case "del":
		id, ok := argID(1)
		if !ok {
			return
		}
		if id == ctx.TgID {
			reply(ctx.Bot, ctx.ChatID, "Нельзя удалить самого себя.")
			return
		}
		u, found, err := ctx.UsersStore.GetByID(id)
		if err != nil || !found {
			reply(ctx.Bot, ctx.ChatID, "Пользователь не найден.")
			return
		}
		sendConfirm(ctx, fmt.Sprintf("Удалить пользователя %s (tg_id=%d)? Доступ к боту пропадёт. VPN-профили и прокси не удаляются.", u.Name, u.TelegramID), "usr:del", idStr(id))

	case "dis":
		id, ok := argID(1)
		if !ok {
			return
		}
		if id == ctx.TgID {
			reply(ctx.Bot, ctx.ChatID, "Нельзя отключить самого себя.")
			return
		}
		u, found, err := ctx.UsersStore.GetByID(id)
		if err != nil || !found {
			reply(ctx.Bot, ctx.ChatID, "Пользователь не найден.")
			return
		}
		sendConfirm(ctx, fmt.Sprintf("Отключить %s (tg_id=%d) от бота? Он перестанет получать ответы и уведомления, роли и префиксы сохранятся. VPN-профили и прокси продолжат работать.", u.Name, u.TelegramID), "usr:dis", idStr(id))

	case "en":
		id, ok := argID(1)
		if !ok {
			return
		}
		u, found, err := ctx.UsersStore.GetByID(id)
		if err != nil || !found {
			reply(ctx.Bot, ctx.ChatID, "Пользователь не найден.")
			return
		}
		msg, err := setUserDisabled(ctx, u, false)
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
			return
		}
		reply(ctx.Bot, ctx.ChatID, msg)
		showUserCard(ctx, id)

	case "add":
		setUsrPending(ctx.TgID, usrPending{Kind: usrPendingAdd})
		reply(ctx.Bot, ctx.ChatID, "Отправь имя и Telegram ID через пробел, например: vasya 123456789\n/cancel — отмена.")

	case "ign":
		showIgnoredJoins(ctx)

	case "unign":
		id, ok := argID(1)
		if !ok {
			return
		}
		if err := setJoinIgnored(id, false); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
			return
		}
		showIgnoredJoins(ctx)

	case "j":
		handleJoinRequestCallback(ctx, arg(1), arg(2), arg(3))

	default:
		reply(ctx.Bot, ctx.ChatID, "Неизвестный callback.")
	}
}

// createBotUser adds a user and opens their card. welcome notifies the person who sent a join request.
func createBotUser(ctx *Ctx, rawName string, id int64, welcome bool) {
	name, err := validateBotUserName(rawName)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error()+". Отменено.")
		return
	}
	msg, err := ctx.UsersStore.Add(name, id)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
		return
	}
	_ = setJoinIgnored(id, false)
	reply(ctx.Bot, ctx.ChatID, msg+"\nВыдай роли и префиксы в карточке ниже.")
	if welcome {
		w := tgbotapi.NewMessage(id, "✅ Доступ к боту открыт. Список команд: /help")
		if _, err := ctx.Bot.Send(w); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Не удалось отправить приветствие пользователю: "+err.Error())
		}
	}
	ctx.MessageID = 0
	showUserCard(ctx, id)
}

func renameUserWithSection(ctx *Ctx, oldName, newName string) (string, error) {
	msg1, err := ctx.UsersStore.Rename(oldName, newName)
	if err != nil {
		return "", err
	}
	if ctx.Domains == nil {
		return msg1, nil
	}
	msg2, err := ctx.Domains.RenameSection(oldName, newName)
	if err != nil {
		return msg1 + "\n\nОшибка при переименовании секции доменов: " + err.Error(), nil
	}
	return msg1 + "\n" + msg2, nil
}

func handleUsrPendingIfAny(ctx *Ctx, text string) bool {
	// Pending actions originate in the private chat; ignore input from other chats.
	if !ctx.IsPrivate {
		return false
	}
	p, ok := popUsrPending(ctx.TgID)
	if !ok {
		return false
	}
	if !ctx.User.Has(RoleAdmin) {
		return false
	}

	input := strings.TrimSpace(text)
	if strings.EqualFold(input, "/cancel") {
		reply(ctx.Bot, ctx.ChatID, "Ок, отменил.")
		return true
	}
	if input == "" {
		reply(ctx.Bot, ctx.ChatID, "Пустое значение, отменено.")
		return true
	}

	switch p.Kind {
	case usrPendingAdd:
		if p.TargetID != 0 {
			createBotUser(ctx, input, p.TargetID, true)
			return true
		}
		f := strings.Fields(input)
		if len(f) != 2 {
			reply(ctx.Bot, ctx.ChatID, "Формат: <name> <tg_id>. Отменено.")
			return true
		}
		id, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil || id <= 0 {
			reply(ctx.Bot, ctx.ChatID, "tg_id должен быть положительным числом. Отменено.")
			return true
		}
		createBotUser(ctx, f[0], id, false)
		return true

	case usrPendingRename:
		u, found, err := ctx.UsersStore.GetByID(p.TargetID)
		if err != nil || !found {
			reply(ctx.Bot, ctx.ChatID, "Пользователь не найден.")
			return true
		}
		name, err := validateBotUserName(input)
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error()+". Отменено.")
			return true
		}
		msg, err := renameUserWithSection(ctx, u.Name, name)
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
			return true
		}
		reply(ctx.Bot, ctx.ChatID, msg)
		showUserCard(ctx, p.TargetID)
		return true

	case usrPendingPrefix:
		u, found, err := ctx.UsersStore.GetByID(p.TargetID)
		if err != nil || !found {
			reply(ctx.Bot, ctx.ChatID, "Пользователь не найден.")
			return true
		}
		if _, err := ctx.UsersStore.AddProfilePrefix(u.Name, p.Profile, input); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
			return true
		}
		showUserPrefixes(ctx, p.Profile, p.TargetID)
		return true

	default:
		reply(ctx.Bot, ctx.ChatID, "Неизвестное ожидаемое действие.")
		return true
	}
}

// handleUsrDeleteConfirm runs after the generic "confirm:usr:del:<id>" button.
func handleUsrDeleteConfirm(ctx *Ctx, payload string) {
	if !ctx.User.Has(RoleAdmin) {
		reply(ctx.Bot, ctx.ChatID, "Недостаточно прав. Нужна роль Admin.")
		return
	}
	id, err := strconv.ParseInt(strings.TrimSpace(payload), 10, 64)
	if err != nil || id <= 0 {
		reply(ctx.Bot, ctx.ChatID, "Неверный формат.")
		return
	}
	if id == ctx.TgID {
		reply(ctx.Bot, ctx.ChatID, "Нельзя удалить самого себя.")
		return
	}
	u, found, err := ctx.UsersStore.GetByID(id)
	if err != nil || !found {
		reply(ctx.Bot, ctx.ChatID, "Пользователь не найден.")
		return
	}
	msg, err := ctx.UsersStore.DeleteByName(u.Name)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
		return
	}
	clearUserUIState(id)
	reply(ctx.Bot, ctx.ChatID, msg)
	ctx.MessageID = 0
	showUsersAdminList(ctx)
}

// handleUsrDisableConfirm runs after the generic "confirm:usr:dis:<id>" button.
func handleUsrDisableConfirm(ctx *Ctx, payload string) {
	if !ctx.User.Has(RoleAdmin) {
		reply(ctx.Bot, ctx.ChatID, "Недостаточно прав. Нужна роль Admin.")
		return
	}
	id, err := strconv.ParseInt(strings.TrimSpace(payload), 10, 64)
	if err != nil || id <= 0 {
		reply(ctx.Bot, ctx.ChatID, "Неверный формат.")
		return
	}
	u, found, err := ctx.UsersStore.GetByID(id)
	if err != nil || !found {
		reply(ctx.Bot, ctx.ChatID, "Пользователь не найден.")
		return
	}
	msg, err := setUserDisabled(ctx, u, true)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
		return
	}
	reply(ctx.Bot, ctx.ChatID, msg)
	ctx.MessageID = 0
	showUserCard(ctx, id)
}
