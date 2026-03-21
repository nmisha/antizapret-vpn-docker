package bot

import "strings"

// handleAccAdminPendingIfAny processes admin input for the accounts UI.
// Returns true if message consumed.
func handleAccAdminPendingIfAny(ctx *Ctx, text string) bool {
	p, ok := popAccAdminPending(ctx.TgID)
	if !ok {
		return false
	}

	if !ctx.User.Has(RoleAdmin) {
		reply(ctx.Bot, ctx.ChatID, "Недостаточно прав. Нужна роль Admin.")
		return true
	}
	if ctx.Accounts == nil {
		reply(ctx.Bot, ctx.ChatID, "AccountsStore не настроен.")
		return true
	}

	s := strings.TrimSpace(text)
	if s == "" {
		reply(ctx.Bot, ctx.ChatID, "Пустое значение.")
		return true
	}

	switch p.Kind {
	case accAdminAdd:
		parts := strings.SplitN(s, ";", 3)
		if len(parts) < 3 {
			reply(ctx.Bot, ctx.ChatID, "Формат: <name>;<login>;<password>")
			return true
		}
		name := strings.TrimSpace(parts[0])
		login := strings.TrimSpace(parts[1])
		pass := parts[2]
		msg, err := ctx.Accounts.Upsert(Account{Name: name, Login: login, Password: pass})
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
			return true
		}
		reply(ctx.Bot, ctx.ChatID, msg)
		showAccountsAdminList(ctx)
		return true

	case accAdminRename:
		msg, err := ctx.Accounts.Rename(p.Name, s)
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
			return true
		}
		reply(ctx.Bot, ctx.ChatID, msg)
		showAccountsAdminList(ctx)
		return true

	case accAdminSetLogin, accAdminSetPass:
		list, err := ctx.Accounts.List()
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Ошибка чтения accounts.json: "+err.Error())
			return true
		}
		key := strings.ToLower(strings.TrimSpace(p.Name))
		var cur *Account
		for i := range list {
			if strings.ToLower(list[i].Name) == key {
				cur = &list[i]
				break
			}
		}
		if cur == nil {
			reply(ctx.Bot, ctx.ChatID, "Учётная запись не найдена: "+p.Name)
			showAccountsAdminList(ctx)
			return true
		}
		upd := *cur
		if p.Kind == accAdminSetLogin {
			upd.Login = s
		} else {
			upd.Password = s
		}
		msg, err := ctx.Accounts.Upsert(upd)
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
			return true
		}
		reply(ctx.Bot, ctx.ChatID, msg)
		showAccountsAdminList(ctx)
		return true

	default:
		reply(ctx.Bot, ctx.ChatID, "Неизвестное ожидаемое действие.")
		return true
	}
}
