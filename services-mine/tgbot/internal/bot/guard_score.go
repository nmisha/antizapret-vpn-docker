package bot

import (
	"fmt"
	"html"
	"strings"
)

func sendDNSGuardRiskScore(ctx *Ctx, kind, profileName string, adminView bool) {
	risk, err := fetchDNSGuardProfileRisk(kind, profileName)
	if err != nil {
		reply(ctx.Bot, ctx.ChatID, "Не удалось получить risk score:\n"+truncate(err.Error(), 3500))
		return
	}
	replyHTML(ctx.Bot, ctx.ChatID, formatDNSGuardRiskScoreMessage(risk, adminView))
}

func formatDNSGuardRiskScoreMessage(risk *dnsGuardProfileRisk, adminView bool) string {
	lines := []string{
		"<b>DNS Guard Risk Score</b>",
		fmt.Sprintf("Профиль: <b>%s</b>", html.EscapeString(risk.ProfileName)),
		fmt.Sprintf("Тип: <code>%s</code>", html.EscapeString(strings.ToUpper(risk.ProfileKind))),
	}
	if adminView && strings.TrimSpace(risk.LastProfileIP) != "" {
		lines = append(lines, fmt.Sprintf("IP: <code>%s</code>", html.EscapeString(risk.LastProfileIP)))
	}

	if adminView {
		lines = append(lines,
			fmt.Sprintf("Notify: <b>%d</b> / <b>%d</b> (<b>%.1f%%</b>)", risk.EffectiveScore, risk.NotifyThreshold, risk.PercentToNotify),
			fmt.Sprintf("15m block: <b>%d</b> / <b>%d</b> (<b>%.1f%%</b>)", risk.Score15m, risk.BlockThreshold15m, risk.PercentToBlock15m),
			fmt.Sprintf("24h block: <b>%d</b> / <b>%d</b> (<b>%.1f%%</b>)", risk.Score24h, risk.BlockThreshold24h, risk.PercentToBlock24h),
			fmt.Sprintf("Critical: <b>%.1f%%</b> (<code>%s</code>)", risk.PercentToCritical, html.EscapeString(risk.CriticalWindow)),
		)
	} else {
		lines = append(lines,
			fmt.Sprintf("Notify: <b>%.1f%%</b>", risk.PercentToNotify),
			fmt.Sprintf("15m block: <b>%.1f%%</b>", risk.PercentToBlock15m),
			fmt.Sprintf("24h block: <b>%.1f%%</b>", risk.PercentToBlock24h),
			fmt.Sprintf("Critical: <b>%.1f%%</b>", risk.PercentToCritical),
		)
	}

	if strings.TrimSpace(risk.LastMatchedDomain) != "" {
		lines = append(lines, fmt.Sprintf("Последний домен: <code>%s</code>", html.EscapeString(risk.LastMatchedDomain)))
	}
	if adminView && strings.TrimSpace(risk.LastMatchedReason) != "" {
		lines = append(lines, fmt.Sprintf("Причина: %s", html.EscapeString(risk.LastMatchedReason)))
	}
	if strings.TrimSpace(risk.LastAction) != "" {
		lines = append(lines, fmt.Sprintf("Последнее действие: <code>%s</code>", html.EscapeString(risk.LastAction)))
	}
	if strings.TrimSpace(risk.PendingBlockAt) != "" {
		lines = append(lines, fmt.Sprintf("Pending block at: <code>%s</code>", html.EscapeString(risk.PendingBlockAt)))
	}
	if strings.TrimSpace(risk.LastBlockAt) != "" {
		lines = append(lines, fmt.Sprintf("Blocked at: <code>%s</code>", html.EscapeString(risk.LastBlockAt)))
	}
	return strings.Join(lines, "\n")
}
