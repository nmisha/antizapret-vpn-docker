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
	reg.Command(CommandSpec{Cmd: "/net_dns_resolve", Args: "[domain]", Desc: "DNS resolve через заданный DNS-сервер", Section: "Сеть", NeedAny: []Role{RoleNetUser}}, handleNetDNSResolve,
		RequireRole(RoleNetUser, "Недостаточно прав."),
	)
	reg.Alias("net_dns_resolve", "/net_dns_resolve")

	reg.Command(CommandSpec{Cmd: "/net_get_iperf3", Desc: "готовые iperf3 команды для near/far node", Section: "Сеть", NeedAny: []Role{RoleNetUser}}, handleNetGetIperf3,
		RequireRole(RoleNetUser, "Недостаточно прав."),
	)
	reg.Alias("net_get_iperf3", "/net_get_iperf3")
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
			tgbotapi.NewInlineKeyboardButtonData("Отмена", "ui:cancel"),
		),
	)
	msg := tgbotapi.NewMessage(ctx.ChatID, "Введи доменное имя для DNS resolve, например: example.com\n\n/cancel - отмена")
	msg.ReplyMarkup = kb
	_, err := ctx.Bot.Send(msg)
	logSendErrorIfEnabled(ctx.ChatID, err, "send message", msg.Text)
}

func netDNSResolveAndReply(ctx *Ctx, domain string) {
	domain = normalizeDomainInput(domain)
	if domain == "" {
		reply(ctx.Bot, ctx.ChatID, "Пустое имя. Попробуй ещё раз: /net_dns_resolve")
		return
	}

	addrs, server, err := netDNSLookup(domain)
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
		lines = append(lines, fmt.Sprintf("• <code>%s</code> — %s", html.EscapeString(ipStr), html.EscapeString(classifyIP(ipStr))))
	}

	m := tgbotapi.NewMessage(ctx.ChatID, strings.Join(lines, "\n"))
	m.ParseMode = "HTML"
	m.DisableWebPagePreview = true
	_, sendErr := ctx.Bot.Send(m)
	logSendErrorIfEnabled(ctx.ChatID, sendErr, "send message", m.Text)
}

func handleNetGetIperf3(ctx *Ctx, _ string) {
	nearAddrs, _, err := netDNSLookup("az-local.antizapret")
	if err != nil || len(nearAddrs) == 0 {
		reply(ctx.Bot, ctx.ChatID, "Не удалось резолвнуть az-local.antizapret.")
		return
	}
	farAddrs, _, err := netDNSLookup("az-world.antizapret")
	if err != nil || len(farAddrs) == 0 {
		reply(ctx.Bot, ctx.ChatID, "Не удалось резолвнуть az-world.antizapret.")
		return
	}

	nearIP := nearAddrs[0].IP.String()
	farIP := farAddrs[0].IP.String()

	msg := strings.Join([]string{
		"<b>NEAR-Node</b>",
		"Upload test",
		"<code>iperf3 -c " + html.EscapeString(nearIP) + " -i1 -t10 -P10</code>",
		"Download test",
		"<code>iperf3 -c " + html.EscapeString(nearIP) + " -i1 -t10 -P10 -R</code>",
		"",
		"<b>FAR-Node</b>",
		"Upload test",
		"<code>iperf3 -c " + html.EscapeString(farIP) + " -i1 -t10 -P10</code>",
		"Download test",
		"<code>iperf3 -c " + html.EscapeString(farIP) + " -i1 -t10 -P10 -R</code>",
	}, "\n")
	replyHTML(ctx.Bot, ctx.ChatID, msg)
}

func netDNSLookup(domain string) ([]net.IPAddr, string, error) {
	server := strings.TrimSpace(os.Getenv("NET_DNS_SERVER"))
	if server == "" {
		server = "14.16.0.1:53"
	}
	if !strings.Contains(server, ":") {
		server += ":53"
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
	return addrs, server, err
}

func normalizeDomainInput(domain string) string {
	domain = strings.TrimSpace(domain)
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimPrefix(domain, "https://")
	if i := strings.Index(domain, "/"); i >= 0 {
		domain = domain[:i]
	}
	domain = strings.TrimSpace(domain)
	domain = strings.TrimSuffix(domain, ".")
	return domain
}

func classifyIP(ipStr string) string {
	if ipStr == "0.0.0.0" {
		return "Blocked"
	}
	if strings.HasPrefix(ipStr, "14.16.") {
		return "NEAR-Node"
	}
	if strings.HasPrefix(ipStr, "14.18.") {
		return "FAR-Node"
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return "Unknown"
	}
	if ip.IsPrivate() {
		return "Private Address"
	}
	return "Direct Link"
}
