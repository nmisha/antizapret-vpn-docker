package bot

func RegisterGuardNotifyHandlers(reg *CommandRegistry) {
	reg.Command(
		CommandSpec{
			Cmd:     "/guard_notify_status",
			Args:    "",
			Desc:    "показать статус DNS Guard уведомлений",
			Section: "DNS Guard",
		},
		handleGuardNotifyStatus,
		RequirePrivateWithOpenDM("Команда /guard_notify_status доступна только в личных сообщениях с ботом."),
	)

	reg.Command(
		CommandSpec{
			Cmd:     "/guard_notify_off",
			Args:    "",
			Desc:    "отключить DNS Guard уведомления",
			Section: "DNS Guard",
		},
		handleGuardNotifyOff,
		RequirePrivateWithOpenDM("Команда /guard_notify_off доступна только в личных сообщениях с ботом."),
	)

	reg.Command(
		CommandSpec{
			Cmd:     "/guard_notify_on",
			Args:    "",
			Desc:    "включить DNS Guard уведомления",
			Section: "DNS Guard",
		},
		handleGuardNotifyOn,
		RequirePrivateWithOpenDM("Команда /guard_notify_on доступна только в личных сообщениях с ботом."),
	)

	reg.Command(
		CommandSpec{
			Cmd:     "/guard_notme_notify_off",
			Args:    "",
			Desc:    "отключить admin-уведомления DNS Guard по другим пользователям",
			Section: "DNS Guard",
			NeedAny: []Role{RoleAdmin},
		},
		handleGuardNotMeNotifyOff,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequirePrivateWithOpenDM("Команда /guard_notme_notify_off доступна только в личных сообщениях с ботом."),
	)

	reg.Command(
		CommandSpec{
			Cmd:     "/guard_notme_notify_on",
			Args:    "",
			Desc:    "включить admin-уведомления DNS Guard по другим пользователям",
			Section: "DNS Guard",
			NeedAny: []Role{RoleAdmin},
		},
		handleGuardNotMeNotifyOn,
		RequireRole(RoleAdmin, "Недостаточно прав. Нужна роль Admin."),
		RequirePrivateWithOpenDM("Команда /guard_notme_notify_on доступна только в личных сообщениях с ботом."),
	)
}

func handleGuardNotifyStatus(ctx *Ctx, _ string) {
	if ctx.UsersStore == nil {
		reply(ctx.Bot, ctx.ChatID, "UsersStore не настроен.")
		return
	}

	user, ok, err := ctx.UsersStore.GetByID(ctx.TgID)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Не удалось прочитать настройку: "+err.Error())
		return
	}
	if !ok {
		reply(ctx.Bot, ctx.ChatID, "Пользователь не найден в users.json.")
		return
	}

	status := "включены"
	mode := "явно"
	if !user.GuardNotificationsEnabled() {
		status = "отключены"
	}
	if user.GuardNotifyEnabled == nil {
		mode = "по умолчанию"
	}

	msg := "Уведомления DNS Guard " + status + ". Режим: " + mode + "."
	if user.Has(RoleAdmin) {
		notMeStatus := "включены"
		notMeMode := "явно"
		if !user.GuardNotMeNotificationsEnabled() {
			notMeStatus = "отключены"
		}
		if user.GuardNotMeNotifyEnabled == nil {
			notMeMode = "по умолчанию"
		}
		msg += "\nAdmin-уведомления по другим пользователям " + notMeStatus + ". Режим: " + notMeMode + "."
	}

	reply(ctx.Bot, ctx.ChatID, msg)
}

func handleGuardNotifyOff(ctx *Ctx, _ string) {
	setGuardNotify(ctx, false, "Уведомления DNS Guard отключены.")
}

func handleGuardNotifyOn(ctx *Ctx, _ string) {
	setGuardNotify(ctx, true, "Уведомления DNS Guard включены.")
}

func handleGuardNotMeNotifyOff(ctx *Ctx, _ string) {
	setGuardNotMeNotify(ctx, false, "Admin-уведомления DNS Guard по другим пользователям отключены.")
}

func handleGuardNotMeNotifyOn(ctx *Ctx, _ string) {
	setGuardNotMeNotify(ctx, true, "Admin-уведомления DNS Guard по другим пользователям включены.")
}

func setGuardNotify(ctx *Ctx, enabled bool, okMessage string) {
	if ctx.UsersStore == nil {
		reply(ctx.Bot, ctx.ChatID, "UsersStore не настроен.")
		return
	}
	if _, err := ctx.UsersStore.SetGuardNotifyByID(ctx.TgID, enabled); err != nil {
		reply(ctx.Bot, ctx.ChatID, "Не удалось сохранить настройку: "+err.Error())
		return
	}
	reply(ctx.Bot, ctx.ChatID, okMessage)
}

func setGuardNotMeNotify(ctx *Ctx, enabled bool, okMessage string) {
	if ctx.UsersStore == nil {
		reply(ctx.Bot, ctx.ChatID, "UsersStore не настроен.")
		return
	}
	if _, err := ctx.UsersStore.SetGuardNotMeNotifyByID(ctx.TgID, enabled); err != nil {
		reply(ctx.Bot, ctx.ChatID, "Не удалось сохранить настройку: "+err.Error())
		return
	}
	reply(ctx.Bot, ctx.ChatID, okMessage)
}
