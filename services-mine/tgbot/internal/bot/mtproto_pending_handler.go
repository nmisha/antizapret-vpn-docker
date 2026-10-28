package bot

import (
	"strings"
	"time"
)

func handleMtpPendingIfAny(ctx *Ctx, text string) bool {
	// Pending actions originate in the private chat. Other chats must neither
	// consume the input nor receive proxy secrets.
	if !ctx.IsPrivate {
		return false
	}
	p, ok := popMtpPending(ctx.TgID)
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

	client, err := newTelemtClientFromEnv()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, err.Error())
		return true
	}

	switch p.Kind {
	case mtpPendingAdd:
		if _, ok := getMtpAdminScope(ctx.TgID); !ok {
			reply(ctx.Bot, ctx.ChatID, "Действие недоступно: кнопка устарела, открой список прокси заново.")
			return true
		}
		// Keep the case as typed (existing names are mixed-case); prefixes match case-insensitively.
		name := input
		if !telemtUserNameRe.MatchString(name) || len(name) > mtpMaxNewName {
			reply(ctx.Bot, ctx.ChatID, "Недопустимое имя: латиница, цифры, _ . -, до 32 символов. Отменено.")
			return true
		}
		// telemt names are case-sensitive: refuse "vasya" next to "Vasya", they would look like one proxy.
		existing, err := client.listUsers()
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Не удалось получить список прокси:\n"+truncate(err.Error(), 3500))
			return true
		}
		for _, e := range existing {
			if strings.EqualFold(e.Username, name) {
				reply(ctx.Bot, ctx.ChatID, "Прокси с таким именем уже есть: "+e.Username+". Отменено.")
				return true
			}
		}
		u, err := client.createUser(name)
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Не удалось создать прокси:\n"+truncate(err.Error(), 3500))
			return true
		}
		if u.Username == "" {
			u.Username = name
		}
		reply(ctx.Bot, ctx.ChatID, "OK: MTProto прокси "+name+" создан с лимитами по умолчанию.")
		u = refreshMtpUserLinks(client, u)
		sendMtpLink(ctx, u)
		if _, err := validateMtpAdminAccess(ctx, client, name); err == nil {
			sendMtpUserActions(ctx, u, true)
		}
		return true

	case mtpPendingLimit:
		f, ok := mtpLimitFieldByKey(p.Field)
		if !ok {
			reply(ctx.Bot, ctx.ChatID, "Неизвестное поле лимита.")
			return true
		}
		if _, err := validateMtpAdminAccess(ctx, client, p.Name); err != nil {
			reply(ctx.Bot, ctx.ChatID, "Действие недоступно: "+truncate(err.Error(), 3500))
			return true
		}
		v, err := parseMtpLimitValue(f, input, time.Now())
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error()+". Отменено, нажми кнопку лимита ещё раз.")
			return true
		}
		u, err := client.patchUser(p.Name, map[string]any{f.JSON: v})
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Не удалось изменить лимит:\n"+truncate(err.Error(), 3500))
			return true
		}
		sendMtpLimitsMenu(ctx, u)
		return true

	default:
		reply(ctx.Bot, ctx.ChatID, "Неизвестное ожидаемое действие.")
		return true
	}
}
