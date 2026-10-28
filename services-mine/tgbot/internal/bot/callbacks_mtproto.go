package bot

import (
	"strings"
)

func handleMtprotoCallback(ctx *Ctx, data string) {
	if !ctx.IsPrivate {
		reply(ctx.Bot, ctx.ChatID, "MTProto прокси доступны только в личных сообщениях боту.")
		return
	}
	client, err := newTelemtClientFromEnv()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, err.Error())
		return
	}

	switch {
	case data == "mtp:u:back":
		handleMtproto(ctx, "")

	case strings.HasPrefix(data, "mtp:u:p:"):
		u, err := validateMtpUserAccess(ctx, client, strings.TrimPrefix(data, "mtp:u:p:"))
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Доступ к прокси недоступен: "+truncate(err.Error(), 3500))
			return
		}
		sendMtpUserActions(ctx, u, false)

	case strings.HasPrefix(data, "mtp:u:x:"):
		act, name, ok := strings.Cut(strings.TrimPrefix(data, "mtp:u:x:"), ":")
		if !ok {
			reply(ctx.Bot, ctx.ChatID, "Неверный формат.")
			return
		}
		u, err := validateMtpUserAccess(ctx, client, name)
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Доступ к прокси недоступен: "+truncate(err.Error(), 3500))
			return
		}
		switch act {
		case "link":
			sendMtpLink(ctx, u)
		case "rot":
			sendConfirm(ctx, "Выпустить новый секрет для "+name+"? Старая ссылка перестанет работать на всех устройствах.", "mtp:u:rotate", name)
		default:
			reply(ctx.Bot, ctx.ChatID, "Неизвестное действие.")
		}

	case strings.HasPrefix(data, "mtp:a:"):
		if !ctx.User.Has(RoleAdmin) {
			reply(ctx.Bot, ctx.ChatID, "Недостаточно прав. Нужна роль Admin.")
			return
		}
		handleMtprotoAdminCallback(ctx, client, strings.TrimPrefix(data, "mtp:a:"))

	default:
		reply(ctx.Bot, ctx.ChatID, "Неизвестный callback.")
	}
}

func handleMtprotoAdminCallback(ctx *Ctx, client *telemtClient, data string) {
	showScope := func(scope wgAdminScope) {
		setMtpAdminScope(ctx.TgID, scope)
		users, err := getMtpUsersForScope(ctx, client, scope)
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Ошибка: "+truncate(err.Error(), 3500))
			return
		}
		sendMtpUsersList(ctx, users, "mtp:a:p:", true)
	}

	switch {
	case data == "scope:my":
		showScope(wgAdminScope{Mode: wgScopeMy})
	case data == "scope:all":
		showScope(wgAdminScope{Mode: wgScopeAll})
	case data == "scope:user":
		sendMtpBotUsersList(ctx)
	case strings.HasPrefix(data, "user:"):
		showScope(wgAdminScope{Mode: wgScopeUser, UserName: normalizeName(strings.TrimPrefix(data, "user:"))})

	case data == "back":
		scope, ok := getMtpAdminScope(ctx.TgID)
		if !ok {
			handleMtprotoAdmin(ctx, "")
			return
		}
		showScope(scope)

	case data == "add":
		if _, ok := getMtpAdminScope(ctx.TgID); !ok {
			reply(ctx.Bot, ctx.ChatID, "Кнопка устарела, открой список прокси заново.")
			return
		}
		setMtpPending(ctx.TgID, mtpPending{Kind: mtpPendingAdd})
		reply(ctx.Bot, ctx.ChatID, "Отправь имя нового MTProto прокси одним сообщением: латиница, цифры, _ . -, до 32 символов. "+
			"Пользователь бота увидит прокси, если имя начинается с одного из его префиксов mtproto_profiles."+
			mtpScopePrefixesHint(ctx)+"\n/cancel — отмена.")

	case strings.HasPrefix(data, "p:"):
		u, err := validateMtpAdminAccess(ctx, client, strings.TrimPrefix(data, "p:"))
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Действие недоступно: "+truncate(err.Error(), 3500))
			return
		}
		sendMtpUserActions(ctx, u, true)

	case strings.HasPrefix(data, "l:"):
		key, name, ok := strings.Cut(strings.TrimPrefix(data, "l:"), ":")
		f, known := mtpLimitFieldByKey(key)
		if !ok || !known {
			reply(ctx.Bot, ctx.ChatID, "Неверный формат.")
			return
		}
		if _, err := validateMtpAdminAccess(ctx, client, name); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Действие недоступно: "+truncate(err.Error(), 3500))
			return
		}
		setMtpPending(ctx.TgID, mtpPending{Kind: mtpPendingLimit, Name: name, Field: f.Key})
		reply(ctx.Bot, ctx.ChatID, f.Label+" для "+name+": отправь значение одним сообщением.\nФормат: "+f.Hint+
			"\nnone — убрать индивидуальный лимит (будет значение по умолчанию из config.toml).\n/cancel — отмена.")

	case strings.HasPrefix(data, "x:"):
		act, name, ok := strings.Cut(strings.TrimPrefix(data, "x:"), ":")
		if !ok {
			reply(ctx.Bot, ctx.ChatID, "Неверный формат.")
			return
		}
		u, err := validateMtpAdminAccess(ctx, client, name)
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Действие недоступно: "+truncate(err.Error(), 3500))
			return
		}
		switch act {
		case "link":
			sendMtpLink(ctx, u)
		case "lim":
			sendMtpLimitsMenu(ctx, u)
		case "on", "off":
			nu, err := client.setEnabled(name, act == "on")
			if err != nil {
				reply(ctx.Bot, ctx.ChatID, "Не удалось изменить состояние:\n"+truncate(err.Error(), 3500))
				return
			}
			sendMtpUserActions(ctx, nu, true)
		case "rq":
			if err := client.resetQuota(name); err != nil {
				reply(ctx.Bot, ctx.ChatID, "Не удалось сбросить квоту:\n"+truncate(err.Error(), 3500))
				return
			}
			reply(ctx.Bot, ctx.ChatID, "OK: счётчик квоты сброшен для "+name)
		case "rot":
			sendConfirm(ctx, "Выпустить новый секрет для "+name+"? Старая ссылка перестанет работать на всех устройствах.", "mtp:a:rotate", name)
		case "del":
			sendConfirm(ctx, "Удалить MTProto прокси "+name+"? Активные сессии будут разорваны.", "mtp:a:delete", name)
		default:
			reply(ctx.Bot, ctx.ChatID, "Неизвестное действие.")
		}

	default:
		reply(ctx.Bot, ctx.ChatID, "Неизвестный callback.")
	}
}

// mtpScopePrefixesHint lists the prefixes of the bot user selected in the admin scope.
func mtpScopePrefixesHint(ctx *Ctx) string {
	scope, _ := getMtpAdminScope(ctx.TgID)
	var u User
	switch scope.Mode {
	case wgScopeMy:
		u = ctx.User
	case wgScopeUser:
		found, ok, err := ctx.UsersStore.GetByName(scope.UserName)
		if err != nil || !ok {
			return ""
		}
		u = found
	default:
		return ""
	}
	if len(u.MtprotoProfilesRaw) == 0 {
		return "\nУ " + u.Name + " нет префиксов mtproto_profiles: созданный прокси будет виден только в scope All."
	}
	return "\nПрефиксы " + u.Name + ": " + strings.Join(u.MtprotoProfilesRaw, ", ")
}

// refreshMtpUserLinks re-reads the user when a create/rotate response came without links.
func refreshMtpUserLinks(client *telemtClient, u telemtUser) telemtUser {
	if u.TLSLink() != "" || u.Username == "" {
		return u
	}
	if fresh, err := client.getUser(u.Username); err == nil {
		return fresh
	}
	return u
}

// handleMtpConfirm performs confirmed destructive actions; kind is "mtp:<u|a>:<rotate|delete>".
func handleMtpConfirm(ctx *Ctx, kind, name string) {
	client, err := newTelemtClientFromEnv()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, err.Error())
		return
	}
	admin := strings.HasPrefix(kind, "mtp:a:")
	if admin {
		_, err = validateMtpAdminAccess(ctx, client, name)
	} else {
		_, err = validateMtpUserAccess(ctx, client, name)
	}
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Действие недоступно: "+truncate(err.Error(), 3500))
		return
	}

	switch {
	case strings.HasSuffix(kind, ":rotate"):
		u, err := client.rotateSecret(name)
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Не удалось сменить секрет:\n"+truncate(err.Error(), 3500))
			return
		}
		reply(ctx.Bot, ctx.ChatID, "OK: новый секрет выпущен, старая ссылка больше не работает.")
		sendMtpLink(ctx, refreshMtpUserLinks(client, u))
	case kind == "mtp:a:delete":
		if err := client.deleteUser(name); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Не удалось удалить:\n"+truncate(err.Error(), 3500))
			return
		}
		reply(ctx.Bot, ctx.ChatID, "OK: deleted "+name)
	default:
		reply(ctx.Bot, ctx.ChatID, "Неизвестное подтверждение: "+kind)
	}
}
