package bot

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func htmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	)
	return r.Replace(s)
}

func helpForUser(u User) string {
	if gCmdRegistry == nil {
		// fallback
		return "📚 Справка недоступна: registry не инициализирован."
	}

	blocks := gCmdRegistry.SectionsForUser(u)

	var b strings.Builder
	b.WriteString("📚 <b>Справка</b>\n")

	for _, bl := range blocks {
		if len(bl.Items) == 0 {
			continue
		}
		b.WriteString("\n<b>")
		b.WriteString(htmlEscape(bl.Title))
		b.WriteString("</b>\n")
		for _, it := range bl.Items {
			line := it.Cmd
			if it.Args != "" {
				line += " " + it.Args
			}
			if it.Desc != "" {
				line += " — " + it.Desc
			}
			line = htmlEscape(line)
			b.WriteString("• ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n💡 Подсказка: команды и роли не зависят от регистра.\n")
	return b.String()
}

// Up to this many commands /help shows one full list; more get an index of groups.
const helpFullListMax = 20

// Telegram rejects messages over 4096 characters; keep a margin for HTML.
const helpScreenMaxRunes = 4000

const helpCbPrefix = "help:"

type helpGroupView struct {
	helpGroup
	Blocks []helpBlock
	Count  int
}

// helpGroupsForUser splits the commands visible to u into helpGroups. Sections missing from
// helpGroups land in a trailing "Прочее" group, so nothing disappears from /help.
func helpGroupsForUser(reg *CommandRegistry, u User) []helpGroupView {
	bySection := map[string][]CommandSpec{}
	for _, s := range reg.VisibleSpecsForUser(u) {
		bySection[sectionOf(s)] = append(bySection[sectionOf(s)], s)
	}
	grouped := map[string]bool{}
	other := helpGroup{Key: "other", Title: "Прочее"}
	for _, g := range helpGroups {
		for _, sec := range g.Sections {
			grouped[sec] = true
		}
	}
	for _, s := range reg.Specs() {
		if sec := sectionOf(s); !grouped[sec] && !containsString(other.Sections, sec) {
			other.Sections = append(other.Sections, sec)
		}
	}

	out := make([]helpGroupView, 0, len(helpGroups)+1)
	for _, g := range append(append([]helpGroup{}, helpGroups...), other) {
		v := helpGroupView{helpGroup: g}
		for _, sec := range g.Sections {
			items := bySection[sec]
			if len(items) == 0 {
				continue
			}
			// Menus and other commands without arguments first, then the ones that take arguments.
			sort.SliceStable(items, func(i, j int) bool {
				if (items[i].Args == "") != (items[j].Args == "") {
					return items[i].Args == ""
				}
				return items[i].Cmd < items[j].Cmd
			})
			v.Blocks = append(v.Blocks, helpBlock{Title: sec, Items: items})
			v.Count += len(items)
		}
		if v.Count > 0 {
			out = append(out, v)
		}
	}
	return out
}

func sectionOf(s CommandSpec) string {
	if sec := strings.TrimSpace(s.Section); sec != "" {
		return sec
	}
	return "Прочее"
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func writeHelpItems(b *strings.Builder, items []CommandSpec) {
	for _, it := range items {
		line := it.Cmd
		if it.Args != "" {
			line += " " + it.Args
		}
		if it.Desc != "" {
			line += " — " + it.Desc
		}
		b.WriteString("• ")
		b.WriteString(htmlEscape(line))
		b.WriteString("\n")
	}
}

func helpGroupText(v helpGroupView) string {
	var b strings.Builder
	b.WriteString("📚 <b>" + htmlEscape(v.Title) + "</b>\n")
	for _, bl := range v.Blocks {
		if len(v.Blocks) > 1 {
			b.WriteString("\n<b>" + htmlEscape(bl.Title) + "</b>\n")
		} else {
			b.WriteString("\n")
		}
		writeHelpItems(&b, bl.Items)
	}
	return b.String()
}

func helpIndexText(groups []helpGroupView, note string) string {
	keys := make([]string, 0, len(groups))
	for _, g := range groups {
		keys = append(keys, g.Key)
	}
	var b strings.Builder
	if note != "" {
		b.WriteString(htmlEscape(note) + "\n\n")
	}
	b.WriteString("📚 <b>Справка</b>\nВыбери раздел.\n\n")
	b.WriteString("Без кнопок: /help &lt;раздел&gt; (" + htmlEscape(strings.Join(keys, ", ")) + "), всё одним списком: /help all.\n")
	b.WriteString("\n💡 Подсказка: команды и роли не зависят от регистра.\n")
	return b.String()
}

func helpIndexKeyboard(groups []helpGroupView) tgbotapi.InlineKeyboardMarkup {
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, (len(groups)+1)/2)
	for i := 0; i < len(groups); i += 2 {
		row := []tgbotapi.InlineKeyboardButton{helpGroupButton(groups[i])}
		if i+1 < len(groups) {
			row = append(row, helpGroupButton(groups[i+1]))
		}
		rows = append(rows, row)
	}
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func helpGroupButton(g helpGroupView) tgbotapi.InlineKeyboardButton {
	return tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("%s (%d)", g.Title, g.Count), helpCbPrefix+g.Key)
}

var helpBackKeyboard = tgbotapi.NewInlineKeyboardMarkup(
	tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("← Разделы", helpCbPrefix+"index")),
)

// showHelp answers /help [group]: the full list for short help, otherwise an index of groups
// or the requested group. In a callback (ctx.MessageID set) it edits the menu in place.
func showHelp(ctx *Ctx, arg string) {
	if gCmdRegistry == nil {
		reply(ctx.Bot, ctx.ChatID, helpForUser(ctx.User))
		return
	}
	key := strings.ToLower(strings.TrimSpace(arg))
	groups := helpGroupsForUser(gCmdRegistry, ctx.User)
	total := 0
	for _, g := range groups {
		total += g.Count
	}

	if key == "all" || (key == "" && total <= helpFullListMax) {
		replyHTMLChunks(ctx.Bot, ctx.ChatID, helpForUser(ctx.User))
		return
	}
	note := ""
	if key != "" && key != "index" {
		for _, g := range groups {
			if g.Key == key {
				showOrEditHTML(ctx, helpGroupText(g), helpBackKeyboard)
				return
			}
		}
		note = fmt.Sprintf("Раздел %q не найден.", shortLabel(arg, 100))
	}
	showOrEditHTML(ctx, helpIndexText(groups, note), helpIndexKeyboard(groups))
}

func handleHelpCallback(ctx *Ctx, data string) {
	showHelp(ctx, strings.TrimPrefix(data, helpCbPrefix))
}

// HelpProblems checks the registry against helpGroups: sections outside any group and
// screens of the admin's help that would not fit into one Telegram message.
func HelpProblems(reg *CommandRegistry) []string {
	var out []string
	admin := User{Roles: map[Role]bool{RoleAdmin: true}}
	groups := helpGroupsForUser(reg, admin)
	for _, g := range groups {
		if g.Key == "other" {
			for _, bl := range g.Blocks {
				out = append(out, fmt.Sprintf("section %q is not in helpGroups", bl.Title))
			}
		}
		if n := utf8.RuneCountInString(helpGroupText(g)); n > helpScreenMaxRunes {
			out = append(out, fmt.Sprintf("help group %q is %d characters, limit %d", g.Key, n, helpScreenMaxRunes))
		}
	}
	if n := utf8.RuneCountInString(helpIndexText(groups, "")); n > helpScreenMaxRunes {
		out = append(out, fmt.Sprintf("help index is %d characters, limit %d", n, helpScreenMaxRunes))
	}
	return out
}
