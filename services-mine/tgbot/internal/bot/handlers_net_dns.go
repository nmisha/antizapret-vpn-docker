package bot

import (
	"context"
	"fmt"
	"html"
	"net"
	"os"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func RegisterNetHandlers(reg *CommandRegistry) {
	reg.Command(CommandSpec{Cmd: "/net_dns_resolve", Args: "[domain]", Desc: "DNS resolve (через заданный DNS-сервер)", Section: "Сеть", NeedAny: []Role{RoleNetUser}}, handleNetDNSResolve,
		RequireRole(RoleNetUser, "Недостаточно прав."),
	)
	reg.Alias("net_dns_resolve", "/net_dns_resolve")
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
	_, err := ctx.Bot.Send(msg)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", msg.Text)
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

	server := strings.TrimSpace(os.Getenv("NET_DNS_SERVER"))
	if server == "" {
		server = "10.224.0.1:53"
	}
	// Allow providing only IP, without port.
	if !strings.Contains(server, ":") {
		server = server + ":53"
	}

	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 4 * time.Second}
			return d.DialContext(ctx, network, server)
		},
	}

	ctxLookup, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	addrs, err := resolver.LookupIPAddr(ctxLookup, domain)
	if err != nil || len(addrs) == 0 {
		if err != nil {
			reply(ctx.Bot, ctx.ChatID, "Не удалось резолвнуть.")
			logAghErrorIfEnabledChat(ctx.ChatID, getChatUserLabel(ctx.ChatID), "dns resolve failed for "+domain+" via "+server+": "+err.Error())
		} else {
			reply(ctx.Bot, ctx.ChatID, "Не удалось резолвнуть: нет IP-адресов")
		}
		return
	}

	lines := []string{fmt.Sprintf("🔎 <b>%s</b>", html.EscapeString(domain))}
	for _, a := range addrs {
		ipStr := a.IP.String()
		tag := classifyIP(ipStr)
		lines = append(lines, fmt.Sprintf("• <code>%s</code> — %s", html.EscapeString(ipStr), html.EscapeString(tag)))
	}
	lines = append(lines, fmt.Sprintf("• DNS: <code>%s</code>", html.EscapeString(server)))

	m := tgbotapi.NewMessage(ctx.ChatID, strings.Join(lines, "\n"))
	m.ParseMode = "HTML"
	m.DisableWebPagePreview = true
	_, sendErr := ctx.Bot.Send(m)
	logSendErrorIfEnabled(ctx.ChatID, sendErr, "send message", m.Text)
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
