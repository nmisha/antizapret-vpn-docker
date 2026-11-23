package bot

import (
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func RegisterDomainHandlers(reg *CommandRegistry) {
	reg.Command(CommandSpec{Cmd: "/add", Args: "<domain>", Desc: "добавить домен в твою секцию", Section: secDomains, NeedAny: []Role{RoleDomainEditor}}, handleAdd,
		RequireNonEmptyArg("Укажи домен: /add example.com"),
	)
	reg.Alias("add", "/add")

	reg.Command(CommandSpec{Cmd: "/del", Args: "<domain>", Desc: "удалить домен из твоей секции", Section: secDomains, NeedAny: []Role{RoleDomainEditor}}, handleDel,
		RequireNonEmptyArg("Укажи домен: /del example.com"),
	)
	reg.Alias("del", "/del")

	reg.Command(CommandSpec{Cmd: "/list", Desc: "показать домены в твоей секции", Section: secDomains, NeedAny: []Role{RoleDomainEditor}}, handleList)
	reg.Alias("list", "/list")

	reg.Command(CommandSpec{Cmd: "/export", Desc: "экспортировать все домены", Section: secDomains, NeedAny: []Role{RoleDomainManager}}, handleExport)
	reg.Alias("export", "/export")
}

func handleAdd(ctx *Ctx, arg string) {
	d, err := normalizeDomain(arg)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Ошибка: "+err.Error())
		return
	}

	match, err := ctx.Domains.FindLongestContainingDomain(d)
	if err != nil {
		replyServiceError(ctx, "домены", "Не удалось прочитать список доменов", err)
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
			replyServiceError(ctx, "бот", "Не удалось отправить сообщение", err)
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
		replyServiceError(ctx, "домены", "Не удалось сохранить домен", err)
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
}

func handleList(ctx *Ctx, _ string) {
	list, err := ctx.Domains.List(ctx.User.Name)
	if err != nil {
		replyServiceError(ctx, "домены", "Не удалось прочитать список доменов", err)
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
		replyServiceError(ctx, "домены", "Не удалось экспортировать домены", err)
		return
	}
	if Trim(all) == "" {
		reply(ctx.Bot, ctx.ChatID, "Файл пуст.")
		return
	}
	reply(ctx.Bot, ctx.ChatID, all)
}
