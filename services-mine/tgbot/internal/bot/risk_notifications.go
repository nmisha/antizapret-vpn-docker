package bot

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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
	inboxDir := envTrim("RISK_NOTIFICATIONS_INBOX_DIR")
	if inboxDir == "" {
		inboxDir = "/data/notifications/inbox"
	}
	sentDir := envTrim("RISK_NOTIFICATIONS_SENT_DIR")
	if sentDir == "" {
		sentDir = "/data/notifications/sent"
	}
	errorDir := envTrim("RISK_NOTIFICATIONS_ERROR_DIR")
	if errorDir == "" {
		errorDir = "/data/notifications/error"
	}
	pollInterval := 10 * time.Second
	if v := envTrim("RISK_NOTIFICATIONS_POLL_SECONDS"); v != "" {
		if n, err := time.ParseDuration(strings.TrimSpace(v) + "s"); err == nil && n > 0 {
			pollInterval = n
		}
	}
	if err := os.MkdirAll(inboxDir, 0755); err != nil {
		log.Printf("risk notifications: create inbox dir failed: %v", err)
		return
	}
	if err := os.MkdirAll(sentDir, 0755); err != nil {
		log.Printf("risk notifications: create sent dir failed: %v", err)
		return
	}
	if err := os.MkdirAll(errorDir, 0755); err != nil {
		log.Printf("risk notifications: create error dir failed: %v", err)
		return
	}
	startRiskNotificationHTTPServer(botAPI, usersStore)
	go func() {
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()
		for {
			processRiskNotificationQueue(botAPI, usersStore, inboxDir, sentDir, errorDir)
			<-ticker.C
		}
	}()
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

func processRiskNotificationQueue(botAPI *tgbotapi.BotAPI, usersStore *UsersStore, inboxDir, sentDir, errorDir string) {
	entries, err := os.ReadDir(inboxDir)
	if err != nil {
		log.Printf("risk notifications: read inbox failed: %v", err)
		return
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		srcPath := filepath.Join(inboxDir, entry.Name())
		evt, err := loadRiskNotificationEvent(srcPath)
		if err != nil {
			log.Printf("risk notifications: parse %s failed: %v", entry.Name(), err)
			moveRiskNotificationFile(srcPath, filepath.Join(errorDir, entry.Name()))
			continue
		}
		if err := deliverRiskNotification(botAPI, usersStore, evt); err != nil {
			log.Printf("risk notifications: deliver %s failed: %v", entry.Name(), err)
			moveRiskNotificationFile(srcPath, filepath.Join(errorDir, entry.Name()))
			continue
		}
		moveRiskNotificationFile(srcPath, filepath.Join(sentDir, entry.Name()))
	}
}

func loadRiskNotificationEvent(path string) (riskNotificationEvent, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return riskNotificationEvent{}, err
	}
	var evt riskNotificationEvent
	if err := json.Unmarshal(b, &evt); err != nil {
		return riskNotificationEvent{}, err
	}
	if strings.TrimSpace(evt.ProfileKind) == "" || strings.TrimSpace(evt.ProfileName) == "" || evt.Risk < 0 {
		return riskNotificationEvent{}, fmt.Errorf("invalid event payload")
	}
	return evt, nil
}

func deliverRiskNotification(botAPI *tgbotapi.BotAPI, usersStore *UsersStore, evt riskNotificationEvent) error {
	owner, ownerFound, err := findNotificationOwner(usersStore, evt.ProfileKind, evt.ProfileName)
	if err != nil {
		return err
	}
	admins, err := usersStore.ListAdmins()
	if err != nil {
		return err
	}

	userMsg := formatRiskUserMessage(evt)
	adminMsg := formatRiskAdminMessage(evt, owner, ownerFound)

	if ownerFound && owner.TelegramID > 0 {
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
		if ownerFound && admin.TelegramID == owner.TelegramID {
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

func findNotificationOwner(usersStore *UsersStore, profileKind, profileName string) (User, bool, error) {
	switch strings.ToLower(strings.TrimSpace(profileKind)) {
	case "wg", "awg":
		return usersStore.FindByWGProfile(profileName)
	case "ovpn":
		return usersStore.FindByOvpnProfile(profileName)
	default:
		return User{}, false, nil
	}
}

func formatRiskUserMessage(evt riskNotificationEvent) string {
	title := "Обнаружена подозрительная DNS-активность."
	if evt.Action == "block_pending" {
		title = "Обнаружена опасная DNS-активность, профиль может быть заблокирован."
	} else if evt.Risk >= 9 {
		title = "Обнаружена критическая DNS-активность."
	}
	lines := []string{
		html.EscapeString(title),
		"",
		fmt.Sprintf("Профиль: <b>%s</b>", html.EscapeString(evt.ProfileName)),
		fmt.Sprintf("Тип: <code>%s</code>", html.EscapeString(strings.ToUpper(evt.ProfileKind))),
		fmt.Sprintf("Риск-скор: <b>%d</b>", evt.Risk),
		fmt.Sprintf("Причина: %s", html.EscapeString(nonEmptyString(evt.Reason, "manual domain risk match"))),
	}
	if evt.Score15m > 0 || evt.Score24h > 0 {
		lines = append(lines,
			fmt.Sprintf("Скор за 15 минут: <b>%d</b>", evt.Score15m),
			fmt.Sprintf("Скор за 24 часа: <b>%d</b>", evt.Score24h),
		)
	}
	if len(evt.Domains) > 0 {
		lines = append(lines, "Домены:")
		for _, d := range limitStrings(evt.Domains, 5) {
			lines = append(lines, "• <code>"+html.EscapeString(d)+"</code>")
		}
	}
	if evt.TriggeredWindow != "" {
		lines = append(lines, fmt.Sprintf("Окно срабатывания: <code>%s</code>", html.EscapeString(evt.TriggeredWindow)))
	}
	if evt.EstimatedBlockInSeconds > 0 {
		lines = append(lines, fmt.Sprintf("При текущей динамике блокировка может наступить примерно через <b>%s</b>.", html.EscapeString(formatETASeconds(evt.EstimatedBlockInSeconds))))
	}
	if evt.ActionResult != "" {
		lines = append(lines, fmt.Sprintf("Статус: %s", html.EscapeString(evt.ActionResult)))
	}
	lines = append(lines, "", "Если это ожидаемое поведение, свяжитесь с администратором.")
	return strings.Join(lines, "\n")
}

func formatRiskAdminMessage(evt riskNotificationEvent, owner User, ownerFound bool) string {
	ownerLine := "Пользователь: не найден"
	if ownerFound {
		ownerLine = fmt.Sprintf("Пользователь: <b>%s</b> (<code>%d</code>)", html.EscapeString(owner.Name), owner.TelegramID)
	}
	lines := []string{
		"<b>DNS Guard alert</b>",
		ownerLine,
		fmt.Sprintf("Профиль: <b>%s</b>", html.EscapeString(evt.ProfileName)),
		fmt.Sprintf("Тип: <code>%s</code>", html.EscapeString(strings.ToUpper(evt.ProfileKind))),
		fmt.Sprintf("IP: <code>%s</code>", html.EscapeString(nonEmptyString(evt.ProfileIP, "-"))),
		fmt.Sprintf("Риск-скор: <b>%d</b>", evt.Risk),
		fmt.Sprintf("Причина: %s", html.EscapeString(nonEmptyString(evt.Reason, "manual domain risk match"))),
		fmt.Sprintf("Действие: <code>%s</code>", html.EscapeString(nonEmptyString(evt.Action, "notify"))),
	}
	if evt.Score15m > 0 || evt.Score24h > 0 {
		lines = append(lines,
			fmt.Sprintf("Скор за 15 минут: <b>%d</b>", evt.Score15m),
			fmt.Sprintf("Скор за 24 часа: <b>%d</b>", evt.Score24h),
		)
	}
	if evt.TriggeredWindow != "" {
		lines = append(lines, fmt.Sprintf("Окно срабатывания: <code>%s</code>", html.EscapeString(evt.TriggeredWindow)))
	}
	if evt.EstimatedBlockInSeconds > 0 {
		lines = append(lines, fmt.Sprintf("Оценка до блокировки: <b>%s</b>", html.EscapeString(formatETASeconds(evt.EstimatedBlockInSeconds))))
	}
	if evt.ActionResult != "" {
		lines = append(lines, fmt.Sprintf("Результат: <code>%s</code>", html.EscapeString(evt.ActionResult)))
	}
	if evt.MatchedRule != "" {
		lines = append(lines, fmt.Sprintf("Правило: <code>%s</code>", html.EscapeString(evt.MatchedRule)))
	}
	if len(evt.Domains) > 0 {
		lines = append(lines, "Домены:")
		for _, d := range limitStrings(evt.Domains, 8) {
			lines = append(lines, "• <code>"+html.EscapeString(d)+"</code>")
		}
	}
	return strings.Join(lines, "\n")
}

func moveRiskNotificationFile(srcPath, dstPath string) {
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		log.Printf("risk notifications: create dir failed: %v", err)
		return
	}
	if err := os.Rename(srcPath, dstPath); err != nil {
		log.Printf("risk notifications: move %s -> %s failed: %v", srcPath, dstPath, err)
	}
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
