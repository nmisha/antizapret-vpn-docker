package app

import (
	"fmt"
	"log"
	"net"
	"strings"
	"time"
)

const maxRecentEventKeys = 512

func Run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	rules, err := loadRules()
	if err != nil {
		return err
	}
	state, err := loadState(cfg.StatePath)
	if err != nil {
		return err
	}

	notifyTarget := cfg.NotificationInboxDir
	if strings.TrimSpace(cfg.NotificationAPIURL) != "" {
		notifyTarget = cfg.NotificationAPIURL
	}
	log.Printf("dns-guard started: querylog=%s rules=%d notify=%s", cfg.QueryLogPath, len(rules), notifyTarget)
	ticker := time.NewTicker(time.Duration(cfg.PollIntervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		if freshRules, err := loadRules(); err != nil {
			log.Printf("dns-guard reload rules error: %v", err)
		} else {
			prevSig := rulesSignature(rules)
			nextSig := rulesSignature(freshRules)
			rules = freshRules
			if prevSig != nextSig {
				log.Printf("dns-guard rules reloaded: rules=%d", len(rules))
			}
		}
		if err := runOnce(cfg, rules, state); err != nil {
			log.Printf("dns-guard cycle error: %v", err)
		}
		if err := saveState(cfg.StatePath, state); err != nil {
			log.Printf("dns-guard save state error: %v", err)
		}
		<-ticker.C
	}
}

func runOnce(cfg Config, rules []compiledRule, state *State) error {
	readResult, err := readNewEntries(cfg.QueryLogPath, state.Cursor)
	if err != nil {
		return err
	}
	entries := readResult.Entries
	if len(entries) == 0 {
		state.Cursor.Offset = readResult.NewOffset
		state.Cursor.FileSize = readResult.FileSize
		state.Cursor.FileModTime = readResult.FileModTime.Format(time.RFC3339Nano)
		return nil
	}

	allowedSubnets, err := parseAllowedSubnets(cfg.Subnets)
	if err != nil {
		return err
	}
	ignoreIPs := make(map[string]struct{}, len(cfg.IgnoreIPs))
	for _, ip := range cfg.IgnoreIPs {
		ip = strings.TrimSpace(ip)
		if ip != "" {
			ignoreIPs[ip] = struct{}{}
		}
	}

	for _, entry := range entries {
		ip := strings.TrimSpace(entry.IP)
		if ip == "" {
			continue
		}
		if _, ok := ignoreIPs[ip]; ok {
			continue
		}
		if !ipAllowed(ip, allowedSubnets) {
			continue
		}
		rule, matched := matchRule(entry.QH, rules)
		if !matched {
			continue
		}
		if shouldSkipEvent(entry, state.Cursor) {
			continue
		}

		ref, err := resolveProfile(ip, cfg)
		if err != nil {
			log.Printf("resolve profile for %s failed: %v", ip, err)
			continue
		}
		if ref.Name == "" {
			continue
		}

		profileKey := ref.Kind + ":" + strings.ToLower(strings.TrimSpace(ref.Name))
		ps := state.Profiles[profileKey]
		ps.LastRisk = max(ps.LastRisk, rule.risk)
		ps.LastMatchedDomain = normalizeDomain(entry.QH)
		ps.LastMatchedReason = rule.reason

		action := "notify"
		actionResult := ""
		if rule.risk >= cfg.RiskBlockAt {
			blockErr := maybeBlockProfile(ref, cfg, &ps)
			action = "block"
			if blockErr != nil {
				actionResult = "block_failed: " + truncateForLog(blockErr.Error(), 300)
				log.Printf("block profile %s failed: %v", profileKey, blockErr)
			} else if ps.LastBlockAt != "" {
				actionResult = "blocked"
			}
		}

		if shouldNotify(ps, rule.risk, cfg.NotificationCooldown) && rule.risk >= cfg.RiskNotifyFrom {
			evt := NotificationEvent{
				ID:           fmt.Sprintf("risk-%s-%s-%d", ref.Kind, sanitizeFilename(ref.Name), time.Now().UnixNano()),
				Type:         "risk_notification",
				ProfileKind:  ref.Kind,
				ProfileName:  ref.Name,
				ProfileIP:    ref.IP,
				Risk:         rule.risk,
				Reason:       nonEmpty(rule.reason, "manual domain risk match"),
				Domains:      []string{normalizeDomain(entry.QH)},
				MatchedRule:  rule.domain,
				DetectedAt:   nonEmpty(entry.T, time.Now().UTC().Format(time.RFC3339)),
				Action:       action,
				ActionResult: actionResult,
			}
			if err := deliverNotificationEvent(cfg, evt); err != nil {
				log.Printf("deliver notification event warning: %v", err)
			} else {
				ps.LastNotifyAt = time.Now().UTC().Format(time.RFC3339)
				ps.LastNotifiedRisk = rule.risk
				ps.LastNotificationEvent = evt.ID
				ps.LastAction = action
			}
		}

		state.Profiles[profileKey] = ps
		advanceCursorEvent(entry, &state.Cursor)
	}

	state.Cursor.Offset = readResult.NewOffset
	state.Cursor.FileSize = readResult.FileSize
	state.Cursor.FileModTime = readResult.FileModTime.Format(time.RFC3339Nano)
	return nil
}

func parseAllowedSubnets(src map[string][]string) ([]*net.IPNet, error) {
	out := make([]*net.IPNet, 0, 8)
	for _, list := range src {
		for _, cidr := range list {
			cidr = strings.TrimSpace(cidr)
			if cidr == "" {
				continue
			}
			_, n, err := net.ParseCIDR(cidr)
			if err != nil {
				return nil, fmt.Errorf("parse cidr %q: %w", cidr, err)
			}
			out = append(out, n)
		}
	}
	return out, nil
}

func ipAllowed(ipStr string, nets []*net.IPNet) bool {
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return false
	}
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func maybeBlockProfile(ref ProfileRef, cfg Config, ps *ProfileRiskState) error {
	switch ref.Kind {
	case "wg":
		if !cfg.WG.Enabled || !cfg.WG.Block {
			return nil
		}
		if ps.LastBlockAt != "" {
			return nil
		}
		if err := disableWGProfile(ref, cfg.WG); err != nil {
			return err
		}
		ps.LastBlockAt = time.Now().UTC().Format(time.RFC3339)
		return nil
	case "awg":
		if !cfg.AWG.Enabled || !cfg.AWG.Block {
			return nil
		}
		if ps.LastBlockAt != "" {
			return nil
		}
		if err := disableWGProfile(ref, cfg.AWG); err != nil {
			return err
		}
		ps.LastBlockAt = time.Now().UTC().Format(time.RFC3339)
		return nil
	default:
		return nil
	}
}

func shouldNotify(ps ProfileRiskState, risk int, cooldownSeconds int) bool {
	if risk <= 0 {
		return false
	}
	if ps.LastNotifiedRisk == 0 || risk > ps.LastNotifiedRisk {
		return true
	}
	if ps.LastNotifyAt == "" {
		return true
	}
	last, err := time.Parse(time.RFC3339, ps.LastNotifyAt)
	if err != nil {
		return true
	}
	return time.Since(last) >= time.Duration(cooldownSeconds)*time.Second
}

func nonEmpty(v, fallback string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func truncateForLog(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func shouldSkipEvent(entry QueryLogEntry, cursor CursorState) bool {
	eventTime := parseEventTime(entry.T)
	lastSeen := parseEventTime(cursor.LastSeenTime)
	if !lastSeen.IsZero() && eventTime.Before(lastSeen) {
		return true
	}
	key := eventKey(entry)
	if key == "" {
		return false
	}
	if !eventTime.IsZero() && eventTime.Equal(lastSeen) && containsString(cursor.RecentEventKeys, key) {
		return true
	}
	if lastSeen.IsZero() && containsString(cursor.RecentEventKeys, key) {
		return true
	}
	return false
}

func advanceCursorEvent(entry QueryLogEntry, cursor *CursorState) {
	eventTime := parseEventTime(entry.T)
	key := eventKey(entry)
	lastSeen := parseEventTime(cursor.LastSeenTime)

	switch {
	case !eventTime.IsZero() && (lastSeen.IsZero() || eventTime.After(lastSeen)):
		cursor.LastSeenTime = eventTime.UTC().Format(time.RFC3339Nano)
		cursor.RecentEventKeys = nil
		if key != "" {
			cursor.RecentEventKeys = append(cursor.RecentEventKeys, key)
		}
	case !eventTime.IsZero() && eventTime.Equal(lastSeen):
		if key != "" {
			cursor.RecentEventKeys = appendUniqueBounded(cursor.RecentEventKeys, key, maxRecentEventKeys)
		}
	default:
		if key != "" {
			cursor.RecentEventKeys = appendUniqueBounded(cursor.RecentEventKeys, key, maxRecentEventKeys)
		}
	}
}

func parseEventTime(v string) time.Time {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, v)
	if err == nil {
		return t.UTC()
	}
	t, err = time.Parse(time.RFC3339, v)
	if err == nil {
		return t.UTC()
	}
	return time.Time{}
}

func eventKey(entry QueryLogEntry) string {
	t := strings.TrimSpace(entry.T)
	ip := strings.TrimSpace(entry.IP)
	qh := normalizeDomain(entry.QH)
	qt := strings.TrimSpace(strings.ToUpper(entry.QT))
	if t == "" && ip == "" && qh == "" && qt == "" {
		return ""
	}
	return t + "|" + ip + "|" + qh + "|" + qt
}

func containsString(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

func appendUniqueBounded(list []string, value string, maxLen int) []string {
	if value == "" {
		return list
	}
	if containsString(list, value) {
		return list
	}
	list = append(list, value)
	if len(list) <= maxLen {
		return list
	}
	return append([]string(nil), list[len(list)-maxLen:]...)
}
