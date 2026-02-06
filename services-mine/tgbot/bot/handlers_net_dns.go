package main

import (
	"fmt"
	"html"
	"net"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func RegisterNetHandlers(r *Router) {
	r.Handle("/net_dns_resolve", handleNetDNSResolve, RequireRole(RoleNetUser, "Недостаточно прав.")) // works in private & groups
	r.Alias("net_dns_resolve", "/net_dns_resolve")
}

func handleNetDNSResolve(ctx *Ctx, arg string) {
	domain := strings.TrimSpace(arg)
	if domain == "" {
		setConv(ctx.ChatID, ctx.TgID, ConvState{Mode: ConvNetDNSAwaitDomain})
		netDNSStartUI(ctx)
		return
	}
	netDNSResolveAndReply(ctx, domain)
}

func netDNSStartUI(ctx *Ctx) {
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✖ Отмена", "ui:cancel"),
		),
	)
	msg := tgbotapi.NewMessage(ctx.ChatID, "🌐 Введи доменное имя для DNS resolve (например: example.com)\n\n/cancel — отмена")
	msg.ReplyMarkup = kb
	ctx.Bot.Send(msg)
}

func netDNSResolveAndReply(ctx *Ctx, domain string) {
	domain = strings.TrimSpace(domain)
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimPrefix(domain, "https://")
	// remove path if user pasted URL
	if i := strings.Index(domain, "/"); i >= 0 {
		domain = domain[:i]
	}
	domain = strings.TrimSpace(domain)
	domain = strings.TrimSuffix(domain, ".")
	if domain == "" {
		reply(ctx.Bot, ctx.ChatID, "Пустое имя. Попробуй ещё раз: /net_dns_resolve")
		return
	}

	ips, err := net.LookupIP(domain)
	if err != nil || len(ips) == 0 {
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Не удалось резолвнуть: "+err.Error())
		} else {
			reply(ctx.Bot, ctx.ChatID, "Не удалось резолвнуть: нет IP-адресов")
		}
		return
	}

	lines := []string{fmt.Sprintf("🔎 <b>%s</b>", html.EscapeString(domain))}
	for _, ip := range ips {
		// Prefer IPv4, but show all.
		ipStr := ip.String()
		tag := classifyIP(ipStr)
		lines = append(lines, fmt.Sprintf("• <code>%s</code> — %s", html.EscapeString(ipStr), html.EscapeString(tag)))
	}

	m := tgbotapi.NewMessage(ctx.ChatID, strings.Join(lines, "\n"))
	m.ParseMode = "HTML"
	m.DisableWebPagePreview = true
	ctx.Bot.Send(m)
}

func classifyIP(ipStr string) string {
	// MSK / NL by prefix for 10.224 / 10.226
	if strings.HasPrefix(ipStr, "10.224.") {
		return "MSK-Node"
	}
	if strings.HasPrefix(ipStr, "10.226.") {
		return "NL-Node"
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return "Unknown"
	}
	// net.IP.IsPrivate works for both v4/v6
	if ip.IsPrivate() {
		return "Private Address"
	}
	return "Direct Link"
}
