package bot

import "strings"

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
