package bot

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type riskNotificationEvent struct {
	ID                      string   `json:"id"`
	Type                    string   `json:"type"`
	ProfileKind             string   `json:"profile_kind"`
	ProfileName             string   `json:"profile_name"`
	ProfileIP               string   `json:"profile_ip,omitempty"`
	Risk                    int      `json:"risk"`
	Score15m                int      `json:"score_15m,omitempty"`
	Score24h                int      `json:"score_24h,omitempty"`
	TriggeredWindow         string   `json:"triggered_window,omitempty"`
	EstimatedBlockInSeconds int      `json:"estimated_block_in_seconds,omitempty"`
	Reason                  string   `json:"reason"`
	Domains                 []string `json:"domains"`
	MatchedRule             string   `json:"matched_rule,omitempty"`
	DetectedAt              string   `json:"detected_at"`
	Action                  string   `json:"action"`
	ActionResult            string   `json:"action_result,omitempty"`
}

func startRiskNotificationWorker(botAPI *tgbotapi.BotAPI, usersStore *UsersStore) {
	startRiskNotificationHTTPServer(botAPI, usersStore)
}

func startRiskNotificationHTTPServer(botAPI *tgbotapi.BotAPI, usersStore *UsersStore) {
	addr := envTrim("RISK_NOTIFICATIONS_HTTP_ADDR")
	if addr == "" {
		return
	}
	token := envTrim("RISK_NOTIFICATIONS_HTTP_TOKEN")
	mux := http.NewServeMux()
	mux.HandleFunc("/risk-notifications", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if token != "" && !matchBearerToken(r.Header.Get("Authorization"), token) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, "unauthorized")
			return
		}
		defer r.Body.Close()
		var evt riskNotificationEvent
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&evt); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, "invalid json")
			return
		}
		if strings.TrimSpace(evt.ProfileKind) == "" || strings.TrimSpace(evt.ProfileName) == "" || evt.Risk < 0 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, "invalid event payload")
			return
		}
		if err := deliverRiskNotification(botAPI, usersStore, evt); err != nil {
			log.Printf("risk notifications: api deliver failed: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, "delivery failed")
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, "accepted")
	})
	go func() {
		log.Printf("risk notifications: http server listening on %s", addr)
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Printf("risk notifications: http server stopped: %v", err)
		}
	}()
}

func matchBearerToken(headerValue, expected string) bool {
	headerValue = strings.TrimSpace(headerValue)
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return true
	}
	if !strings.HasPrefix(strings.ToLower(headerValue), "bearer ") {
		return false
	}
	return strings.TrimSpace(headerValue[len("Bearer "):]) == expected
}

func deliverRiskNotification(botAPI *tgbotapi.BotAPI, usersStore *UsersStore, evt riskNotificationEvent) error {
	owners, err := findNotificationOwners(usersStore, evt.ProfileKind, evt.ProfileName)
	if err != nil {
		return err
	}
	admins, err := usersStore.ListAdmins()
	if err != nil {
		return err
	}

	userMsg := formatRiskUserMessage(evt)
	adminMsg := formatRiskAdminMessage(evt, owners)

	ownerIDs := make(map[int64]struct{}, len(owners))
	for _, owner := range owners {
		if owner.TelegramID <= 0 {
			continue
		}
		ownerIDs[owner.TelegramID] = struct{}{}
		if !owner.GuardNotificationsEnabled() {
			continue
		}
		msg := tgbotapi.NewMessage(owner.TelegramID, userMsg)
		msg.ParseMode = "HTML"
		msg.DisableWebPagePreview = true
		if _, err := botAPI.Send(msg); err != nil {
			log.Printf("risk notifications: send user alert to %d failed: %v", owner.TelegramID, err)
		}
	}

	for _, admin := range admins {
		if admin.TelegramID <= 0 {
			continue
		}
		if _, ok := ownerIDs[admin.TelegramID]; ok {
			continue
		}
		if !admin.GuardNotMeNotificationsEnabled() {
			continue
		}
		if !admin.GuardNotificationsEnabled() {
			continue
		}
		msg := tgbotapi.NewMessage(admin.TelegramID, adminMsg)
		msg.ParseMode = "HTML"
		msg.DisableWebPagePreview = true
		if _, err := botAPI.Send(msg); err != nil {
			log.Printf("risk notifications: send admin alert to %d failed: %v", admin.TelegramID, err)
		}
	}
	return nil
}

func findNotificationOwners(usersStore *UsersStore, profileKind, profileName string) ([]User, error) {
	switch strings.ToLower(strings.TrimSpace(profileKind)) {
	case "wg", "awg":
		return usersStore.FindAllByWGProfile(profileName)
	case "ovpn":
		return usersStore.FindAllByOvpnProfile(profileName)
	default:
		return nil, nil
	}
}

func formatRiskUserMessage(evt riskNotificationEvent) string {
	title := "Обнаружена подозрительная DNS-активность."
	if evt.Action == "block" {
		title = "Профиль заблокирован из-за опасной DNS-активности."
	} else if evt.Action == "block_applied" {
		title = "Блокировка профиля фактически применена."
	} else if evt.Action == "block_failed" {
		title = "Попытка блокировки профиля завершилась ошибкой."
	} else if evt.Action == "block_pending" {
		title = "Обнаружена опасная DNS-активность, профиль может быть заблокирован."
	} else if evt.Risk >= 9 {
		title = "Обнаружена критическая DNS-активность."
	}
	lines := []string{
		html.EscapeString(title),
		"",
		fmt.Sprintf("Profile: <b>%s</b>", html.EscapeString(evt.ProfileName)),
		fmt.Sprintf("Type: <code>%s</code>", html.EscapeString(strings.ToUpper(evt.ProfileKind))),
		fmt.Sprintf("Risk Score: <b>%d</b>", evt.Risk),
		fmt.Sprintf("Reason: %s", html.EscapeString(nonEmptyString(evt.Reason, "domain risk match"))),
	}
	if evt.Score15m > 0 || evt.Score24h > 0 {
		lines = append(lines,
			fmt.Sprintf("Score 15m: <b>%d</b>", evt.Score15m),
			fmt.Sprintf("Score 24h: <b>%d</b>", evt.Score24h),
		)
	}
	if len(evt.Domains) > 0 {
		lines = append(lines, "Last domain:")
		for _, d := range limitStrings(evt.Domains, 5) {
			lines = append(lines, "• <code>"+html.EscapeString(d)+"</code>")
		}
	}
	if evt.TriggeredWindow != "" {
		lines = append(lines, fmt.Sprintf("Triggered window: <code>%s</code>", html.EscapeString(evt.TriggeredWindow)))
	}
	if evt.EstimatedBlockInSeconds > 0 {
		lines = append(lines, fmt.Sprintf("При текущей динамике блокировка может наступить примерно через <b>%s</b>.", html.EscapeString(formatETASeconds(evt.EstimatedBlockInSeconds))))
	}
	if evt.ActionResult != "" {
		lines = append(lines, fmt.Sprintf("Status: %s", html.EscapeString(evt.ActionResult)))
	}
	// lines = append(lines, "", "Если это ожидаемое поведение, свяжитесь с администратором.")
	return strings.Join(lines, "\n")
}

func formatRiskAdminMessage(evt riskNotificationEvent, owners []User) string {
	ownerLine := "Owners: not found"
	if len(owners) > 0 {
		parts := make([]string, 0, len(owners))
		for _, owner := range owners {
			parts = append(parts, fmt.Sprintf("<b>%s</b> (<code>%d</code>)", html.EscapeString(owner.Name), owner.TelegramID))
		}
		ownerLine = "Owners: " + strings.Join(parts, ", ")
	}
	lines := []string{
		"<b>DNS Guard alert</b>",
		ownerLine,
		fmt.Sprintf("Profile: <b>%s</b>", html.EscapeString(evt.ProfileName)),
		fmt.Sprintf("Type: <code>%s</code>", html.EscapeString(strings.ToUpper(evt.ProfileKind))),
		fmt.Sprintf("IP: <code>%s</code>", html.EscapeString(nonEmptyString(evt.ProfileIP, "-"))),
		fmt.Sprintf("Risk score: <b>%d</b>", evt.Risk),
		fmt.Sprintf("Reason: %s", html.EscapeString(nonEmptyString(evt.Reason, "manual domain risk match"))),
		fmt.Sprintf("Action: <code>%s</code>", html.EscapeString(nonEmptyString(evt.Action, "notify"))),
	}
	if evt.Score15m > 0 || evt.Score24h > 0 {
		lines = append(lines,
			fmt.Sprintf("Score 15m: <b>%d</b>", evt.Score15m),
			fmt.Sprintf("Score 24h: <b>%d</b>", evt.Score24h),
		)
	}
	if evt.TriggeredWindow != "" {
		lines = append(lines, fmt.Sprintf("Triggered window: <code>%s</code>", html.EscapeString(evt.TriggeredWindow)))
	}
	if evt.EstimatedBlockInSeconds > 0 {
		lines = append(lines, fmt.Sprintf("Estimated block ETA: <b>%s</b>", html.EscapeString(formatETASeconds(evt.EstimatedBlockInSeconds))))
	}
	if evt.ActionResult != "" {
		lines = append(lines, fmt.Sprintf("Action result: <code>%s</code>", html.EscapeString(evt.ActionResult)))
	}
	// if evt.MatchedRule != "" {
	// 	lines = append(lines, fmt.Sprintf("Rule: <code>%s</code>", html.EscapeString(evt.MatchedRule)))
	// }
	if len(evt.Domains) > 0 {
		lines = append(lines, "Last domain:")
		for _, d := range limitStrings(evt.Domains, 8) {
			lines = append(lines, "• <code>"+html.EscapeString(d)+"</code>")
		}
	}
	return strings.Join(lines, "\n")
}

func nonEmptyString(v, fallback string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}

func limitStrings(list []string, n int) []string {
	if len(list) <= n {
		return list
	}
	return list[:n]
}

func formatETASeconds(seconds int) string {
	if seconds <= 0 {
		return ""
	}
	if seconds < 60 {
		return fmt.Sprintf("%d sec", seconds)
	}
	if seconds < 3600 {
		minutes := seconds / 60
		rest := seconds % 60
		if rest == 0 {
			return fmt.Sprintf("%d min", minutes)
		}
		return fmt.Sprintf("%d min %d sec", minutes, rest)
	}
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	if minutes == 0 {
		return fmt.Sprintf("%d h", hours)
	}
	return fmt.Sprintf("%d h %d min", hours, minutes)
}

