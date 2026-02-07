package main

import (
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func RegisterDomainHandlers(r *Router) {
	r.Handle("/add", handleAdd,
		RequireRole(RoleDomainEditor, "Недостаточно прав. Нужна роль DomainEditor (или Admin)."),
		RequireNonEmptyArg("Укажи домен: /add example.com"),
	)
	r.Alias("add", "/add")

	r.Handle("/del", handleDel,
		RequireRole(RoleDomainEditor, "Недостаточно прав. Нужна роль DomainEditor (или Admin)."),
		RequireNonEmptyArg("Укажи домен: /del example.com"),
	)
	r.Alias("del", "/del")

	r.Handle("/list", handleList,
		RequireRole(RoleDomainEditor, "Недостаточно прав. Нужна роль DomainEditor (или Admin)."),
	)
	r.Alias("list", "/list")

	r.Handle("/export", handleExport,
		RequireRole(RoleDomainManager, "Недостаточно прав. Нужна роль DomainManager (или Admin)."),
	)
	r.Alias("export", "/export")
}

func handleAdd(ctx *Ctx, arg string) {
	d, err := normalizeDomain(arg)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
		return
	}

	match, err := ctx.Domains.FindLongestContainingDomain(d)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка чтения списка: "+err.Error())
		return
	}

	if match != nil {
		msgText := fmt.Sprintf(
			"Домен *%s* является поддоменом домена *%s* (секция: #%s).\nВыбери действие:",
			d, match.ParentDomain, match.ParentSection,
		)

		canReplace := (match.ParentSection == ctx.User.Name) || ctx.User.Has(RoleDomainManager)
		canSub := ctx.User.Has(RoleDomainManager)

		kb := buildAddDecisionKeyboard(canReplace, canSub)
		m := tgbotapi.NewMessage(ctx.ChatID, msgText)
		m.ParseMode = "Markdown"
		m.ReplyMarkup = kb
		sent, err := ctx.Bot.Send(m)
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Ошибка отправки: "+err.Error())
			return
		}

		setPending(ctx.TgID, PendingAdd{
			Candidate:     d,
			ParentDomain:  match.ParentDomain,
			ParentSection: match.ParentSection,
			TargetSection: ctx.User.Name,
			ChatID:        ctx.ChatID,
			MessageID:     sent.MessageID,
		})
		return
	}

	added, err := ctx.Domains.AddDomain(ctx.User.Name, d)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка сохранения: "+err.Error())
		return
	}
	if !added {
		reply(ctx.Bot, ctx.ChatID, "Уже есть в твоей секции: "+d)
		return
	}
	reply(ctx.Bot, ctx.ChatID, "Добавлено в секцию #"+ctx.User.Name+": "+d)
}

func handleDel(ctx *Ctx, arg string) {
	d, err := normalizeDomain(arg)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
		return
	}

	// Confirmation for any destructive action.
	// Actual deletion is performed in handleConfirmCallback.
	sendConfirm(ctx, "Удалить домен \""+d+"\"?", "domain:del", d)
	return
}

func handleList(ctx *Ctx, _ string) {
	list, err := ctx.Domains.List(ctx.User.Name)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка чтения: "+err.Error())
		return
	}
	if len(list) == 0 {
		reply(ctx.Bot, ctx.ChatID, "В твоей секции #"+ctx.User.Name+" пока нет доменов.")
		return
	}
	reply(ctx.Bot, ctx.ChatID, formatSection(ctx.User.Name, list))
}

func handleExport(ctx *Ctx, _ string) {
	all, err := ctx.Domains.ExportAll()
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка экспорта: "+err.Error())
		return
	}
	if Trim(all) == "" {
		reply(ctx.Bot, ctx.ChatID, "Файл пуст.")
		return
	}
	reply(ctx.Bot, ctx.ChatID, all)
}
