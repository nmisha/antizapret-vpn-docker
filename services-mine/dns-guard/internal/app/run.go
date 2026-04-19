package app

import (
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

const maxRecentEventKeys = 512
const skippedEventsResetInterval = 36 * time.Hour

func Run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	configSnapshot, err := statConfigFile(configPath())
	if err != nil {
		return err
	}
	if !cfg.Enabled {
		log.Printf("dns-guard disabled by config")
		return nil
	}
	rules, err := loadRules(cfg)
	if err != nil {
		return err
	}
	rulesSnapshot, err := statRulesFile(rulesPath())
	if err != nil {
		return err
	}
	state, err := loadState(cfg.StatePath)
	if err != nil {
		return err
	}
	querySource, err := newQueryLogSource(cfg)
	if err != nil {
		return err
	}

	if err := querySource.Sync(&state.Cursor); err != nil {
		return err
	}
	if err := saveState(cfg.StatePath, state); err != nil {
		return err
	}
	startAPIServer(cfg)
	log.Printf("dns-guard started: querylog=%s rules=%d enabled_rules=%d notify=%s notify_score=%d block15m=%d block24h=%d min_rule_risk=%d max_rule_risk=%d",
		querySource.Description(), len(rules), countEnabledRules(rules), cfg.NotificationAPIURL, cfg.ScoreNotifyAt, cfg.ScoreBlockAt15m, cfg.ScoreBlockAt24h, cfg.MinRuleRisk, cfg.MaxRuleRisk)
	pollTicker := time.NewTicker(time.Duration(cfg.PollIntervalSeconds) * time.Second)
	blockTicker := time.NewTicker(1 * time.Second)
	defer pollTicker.Stop()
	defer blockTicker.Stop()

	for {
		select {
		case <-blockTicker.C:
			if !cfg.Enabled {
				continue
			}
			if changed := processPendingBlocks(cfg, state, time.Now().UTC()); changed {
				if err := saveState(cfg.StatePath, state); err != nil {
					log.Printf("dns-guard save state error: %v", err)
				}
			}
		case <-pollTicker.C:
			if freshCfg, freshSnapshot, changed, err := refreshConfigIfChanged(cfg, configSnapshot); err != nil {
				log.Printf("dns-guard reload config error: %v", err)
			} else {
				prevSourceDesc := querySource.Description()
				cfg = freshCfg
				configSnapshot = freshSnapshot
				if querySource, err = newQueryLogSource(cfg); err != nil {
					log.Printf("dns-guard querylog source reload error: %v", err)
					continue
				}
				if changed && querySource.Description() != prevSourceDesc {
					if err := querySource.Sync(&state.Cursor); err != nil {
						log.Printf("dns-guard querylog source sync error: %v", err)
						continue
					}
				}
				if changed {
					log.Printf("dns-guard config reloaded: notify_score=%d block15m=%d block24h=%d cooldown=%d debug=%t track_skipped=%t ignore_ips=%d whitelist=%d",
						cfg.ScoreNotifyAt, cfg.ScoreBlockAt15m, cfg.ScoreBlockAt24h, cfg.NotificationCooldown,
						cfg.DebugLogEnabled, cfg.TrackSkippedEvents, len(cfg.IgnoreIPs), countProfileWhitelist(cfg.ProfileWhitelist))
					writeDebugLog(cfg, "config_reloaded path=%s size=%d mod_time=%s notify_score=%d block15m=%d block24h=%d cooldown=%d debug=%t track_skipped=%t ignore_ips=%d whitelist=%d",
						configSnapshot.Path, configSnapshot.Size, configSnapshot.ModTime.Format(time.RFC3339Nano),
						cfg.ScoreNotifyAt, cfg.ScoreBlockAt15m, cfg.ScoreBlockAt24h, cfg.NotificationCooldown,
						cfg.DebugLogEnabled, cfg.TrackSkippedEvents, len(cfg.IgnoreIPs), countProfileWhitelist(cfg.ProfileWhitelist))
				}
			}
			if !cfg.Enabled {
				continue
			}
			if reloadResult, err := refreshRulesIfChanged(cfg, rules, rulesSnapshot); err != nil {
				log.Printf("dns-guard reload rules error: %v", err)
			} else {
				rules = reloadResult.Rules
				rulesSnapshot = reloadResult.Snapshot
				if reloadResult.Changed {
					log.Printf("dns-guard rules reloaded: rules=%d enabled_rules=%d", len(rules), countEnabledRules(rules))
					writeDebugLog(cfg, "rules_reloaded path=%s size=%d mod_time=%s rules=%d enabled_rules=%d signature=%q",
						reloadResult.Snapshot.Path,
						reloadResult.Snapshot.Size,
						reloadResult.Snapshot.ModTime.Format(time.RFC3339Nano),
						len(rules),
						countEnabledRules(rules),
						truncateForLog(rulesSignature(rules), 512),
					)
				}
			}
			if err := runOnce(cfg, querySource, rules, state, time.Now().UTC()); err != nil {
				log.Printf("dns-guard cycle error: %v", err)
			}
			if err := saveState(cfg.StatePath, state); err != nil {
				log.Printf("dns-guard save state error: %v", err)
			}
		}
	}
}

func runOnce(cfg Config, source QueryLogSource, rules []compiledRule, state *State, cycleNow time.Time) error {
	cleanupState(state, cycleNow)
	cleanupSkippedEvents(state, cycleNow, cfg.TrackSkippedEvents)
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
	err = source.Process(&state.Cursor, func(entry QueryLogEntry) error {
		defer advanceCursorEvent(entry, &state.Cursor)

		ip := strings.TrimSpace(entry.IP)
		if ip == "" {
			incrementSkippedEvent(state, cfg, "empty_ip")
			return nil
		}
		if _, ok := ignoreIPs[ip]; ok {
			incrementSkippedEvent(state, cfg, "ignored_ip")
			return nil
		}
		if !ipAllowed(ip, allowedSubnets) {
			return nil
		}
		rule, matched := matchRule(entry.QH, rules)
		if !matched {
			return nil
		}
		if shouldSkipEvent(entry, state.Cursor) {
			incrementSkippedEvent(state, cfg, "duplicate_event")
			return nil
		}

		ref, err := resolveProfile(ip, cfg)
		if err != nil {
			log.Printf("resolve profile for %s failed: %v", ip, err)
			writeDebugLog(cfg, "resolve_error ip=%s domain=%s qt=%s rule=%s kind=%s err=%q", ip, normalizeDomain(entry.QH), strings.ToUpper(strings.TrimSpace(entry.QT)), rule.domain, ref.Kind, err.Error())
			incrementSkippedEvent(state, cfg, "resolve_error")
			return nil
		}
		if ref.Name == "" {
			log.Printf("dns-guard matched rule=%s domain=%s but profile was not resolved for ip=%s kind=%s", rule.domain, normalizeDomain(entry.QH), ip, ref.Kind)
			writeDebugLog(cfg, "profile_not_found ip=%s domain=%s qt=%s rule=%s kind=%s", ip, normalizeDomain(entry.QH), strings.ToUpper(strings.TrimSpace(entry.QT)), rule.domain, ref.Kind)
			incrementSkippedEvent(state, cfg, "profile_not_found")
			return nil
		}
		if profileWhitelisted(ref, cfg.ProfileWhitelist) {
			writeDebugLog(cfg, "profile_whitelisted kind=%s profile=%s ip=%s domain=%s qt=%s rule=%s",
				ref.Kind, ref.Name, ref.IP, normalizeDomain(entry.QH), strings.ToUpper(strings.TrimSpace(entry.QT)), rule.domain)
			incrementSkippedEvent(state, cfg, "profile_whitelisted")
			return nil
		}

		profileKey := ref.Kind + ":" + strings.ToLower(strings.TrimSpace(ref.Name))
		ps := state.Profiles[profileKey]
		if ps.LastBlockAt != "" {
			log.Printf("dns-guard profile activity resumed after block: profile=%s blocked_at=%s; resetting profile state", profileKey, ps.LastBlockAt)
			ps = ProfileRiskState{}
		}
		if ps.Buckets == nil {
			ps.Buckets = map[string]int{}
		}
		ps.LastRuleRisk = max(ps.LastRuleRisk, rule.risk)
		ps.LastMatchedDomain = normalizeDomain(entry.QH)
		ps.LastMatchedReason = rule.reason
		ps.LastProfileID = ref.ID
		ps.LastProfileIP = ref.IP

		eventTime := normalizeEventTime(entry.T, cycleNow)
		addScoreBucket(&ps, eventTime, rule.risk)
		cleanupProfileBuckets(&ps, cycleNow)

		score15m, score24h := calculateScores(ps, cycleNow)
		effectiveScore := score15m
		if score24h > effectiveScore {
			effectiveScore = score24h
		}
		writeDebugLog(cfg, "rule_matched profile=%s profile_ip=%s kind=%s domain=%s matched_rule=%s rule_risk=%d score15m=%d score24h=%d effective_score=%d",
			ref.Name, ref.IP, ref.Kind, normalizeDomain(entry.QH), rule.domain, rule.risk, score15m, score24h, effectiveScore)

		if effectiveScore >= cfg.ScoreNotifyAt && shouldNotifyScore(ps, effectiveScore, cfg.NotificationCooldown) {
			estimatedBlockInSeconds := 0
			if cfg.PredictBlockETA {
				estimatedBlockInSeconds = estimateTimeToBlockSeconds(ps, cycleNow, cfg)
			}
			evt := NotificationEvent{
				ID:                      fmt.Sprintf("risk-%s-%s-%d", ref.Kind, sanitizeFilename(ref.Name), time.Now().UnixNano()),
				Type:                    "risk_notification",
				ProfileKind:             ref.Kind,
				ProfileName:             ref.Name,
				ProfileIP:               ref.IP,
				Risk:                    effectiveScore,
				Score15m:                score15m,
				Score24h:                score24h,
				EstimatedBlockInSeconds: estimatedBlockInSeconds,
				Reason:                  nonEmpty(rule.reason, "manual domain risk match"),
				Domains:                 []string{normalizeDomain(entry.QH)},
				MatchedRule:             rule.domain,
				DetectedAt:              nonEmpty(entry.T, time.Now().UTC().Format(time.RFC3339)),
				Action:                  "notify",
			}
			if err := deliverNotificationEvent(cfg, evt); err != nil {
				log.Printf("deliver notification event warning: %v", err)
			} else {
				ps.LastNotifyAt = time.Now().UTC().Format(time.RFC3339)
				ps.LastNotifiedScore = effectiveScore
				ps.LastNotificationEvent = evt.ID
				ps.LastAction = "notify"
			}
		}

		blockWindow, blockScore := evaluateBlockThresholds(cfg, score15m, score24h)
		if blockWindow != "" && canScheduleBlock(ref.Kind, cfg) && ps.LastBlockAt == "" && ps.PendingBlockAt == "" {
			scheduledAt := cycleNow.Add(time.Duration(cfg.BlockDelaySeconds) * time.Second).UTC()
			evt := NotificationEvent{
				ID:              fmt.Sprintf("block-%s-%s-%d", ref.Kind, sanitizeFilename(ref.Name), time.Now().UnixNano()),
				Type:            "risk_notification",
				ProfileKind:     ref.Kind,
				ProfileName:     ref.Name,
				ProfileIP:       ref.IP,
				Risk:            blockScore,
				Score15m:        score15m,
				Score24h:        score24h,
				TriggeredWindow: blockWindow,
				Reason:          nonEmpty(rule.reason, "manual domain risk match"),
				Domains:         []string{normalizeDomain(entry.QH)},
				MatchedRule:     rule.domain,
				DetectedAt:      nonEmpty(entry.T, time.Now().UTC().Format(time.RFC3339)),
				Action:          "block",
				ActionResult:    fmt.Sprintf("block decision applied immediately; technical execution delay %ds until %s", cfg.BlockDelaySeconds, scheduledAt.Format(time.RFC3339)),
			}
			if err := deliverNotificationEvent(cfg, evt); err != nil {
				log.Printf("deliver block notification warning: %v", err)
			}
			ps.PendingBlockAt = scheduledAt.Format(time.RFC3339)
			ps.PendingBlockWindow = blockWindow
			ps.PendingBlockScore = blockScore
			ps.LastNotificationEvent = evt.ID
			ps.LastAction = "block"
		}

		state.Profiles[profileKey] = ps
		return nil
	})
	if err != nil {
		return err
	}
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

func maybeBlockProfile(ref ProfileRef, cfg Config, ps *ProfileRiskState) (string, error) {
	switch ref.Kind {
	case "wg":
		if !cfg.WG.Enabled || !cfg.WG.Block {
			return "", nil
		}
		if ps.LastBlockAt != "" {
			return "", nil
		}
		if err := disableWGProfile(ref, cfg.WG); err != nil {
			return "", err
		}
		ps.LastBlockAt = time.Now().UTC().Format(time.RFC3339)
		return "wg profile disabled", nil
	case "awg":
		if !cfg.AWG.Enabled || !cfg.AWG.Block {
			return "", nil
		}
		if ps.LastBlockAt != "" {
			return "", nil
		}
		if err := disableWGProfile(ref, cfg.AWG); err != nil {
			return "", err
		}
		ps.LastBlockAt = time.Now().UTC().Format(time.RFC3339)
		return "awg profile disabled", nil
	case "ovpn":
		if !cfg.OVPN.Enabled || !cfg.OVPN.Block {
			return "", nil
		}
		if ps.LastBlockAt != "" {
			return "", nil
		}
		if err := revokeOVPNProfile(ref, cfg.OVPN); err != nil {
			return "", err
		}
		writeDebugLog(cfg, "ovpn_revoked profile=%s ip=%s", ref.Name, ref.IP)
		ps.LastBlockAt = time.Now().UTC().Format(time.RFC3339)
		if cfg.OVPN.RestartServer {
			if err := restartOVPNServer(cfg.OVPN); err != nil {
				log.Printf("ovpn restart after revoke failed for %s: %v", ref.Name, err)
				writeDebugLog(cfg, "ovpn_restart_failed profile=%s ip=%s err=%q", ref.Name, ref.IP, err.Error())
				return fmt.Sprintf("ovpn certificate revoked; server restart failed: %v", err), nil
			}
			writeDebugLog(cfg, "ovpn_restarted profile=%s ip=%s", ref.Name, ref.IP)
			return "ovpn certificate revoked; server restarted with SIGUSR1", nil
		}
		return "ovpn certificate revoked", nil
	default:
		return "", nil
	}
}

func canScheduleBlock(kind string, cfg Config) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "wg":
		return cfg.WG.Enabled && cfg.WG.Block
	case "awg":
		return cfg.AWG.Enabled && cfg.AWG.Block
	case "ovpn":
		return cfg.OVPN.Enabled && cfg.OVPN.Block
	default:
		return false
	}
}

func profileWhitelisted(ref ProfileRef, whitelist map[string][]string) bool {
	kind := strings.ToLower(strings.TrimSpace(ref.Kind))
	name := strings.ToLower(strings.TrimSpace(ref.Name))
	if kind == "" || name == "" || len(whitelist) == 0 {
		return false
	}
	for _, candidate := range whitelist[kind] {
		if strings.ToLower(strings.TrimSpace(candidate)) == name {
			return true
		}
	}
	return false
}

func countProfileWhitelist(whitelist map[string][]string) int {
	total := 0
	for _, profiles := range whitelist {
		total += len(profiles)
	}
	return total
}

func shouldNotifyScore(ps ProfileRiskState, score int, cooldownSeconds int) bool {
	if score <= 0 {
		return false
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

func evaluateBlockThresholds(cfg Config, score15m, score24h int) (string, int) {
	window := ""
	score := 0
	if score15m >= cfg.ScoreBlockAt15m {
		window = "15m"
		score = score15m
	}
	if score24h >= cfg.ScoreBlockAt24h && score24h >= score {
		window = "24h"
		score = score24h
	}
	return window, score
}

func normalizeEventTime(src string, fallback time.Time) time.Time {
	t := parseEventTime(src)
	if t.IsZero() {
		return fallback.UTC()
	}
	return t.UTC()
}

func minuteBucketKey(t time.Time) string {
	return t.UTC().Truncate(time.Minute).Format(time.RFC3339)
}

func addScoreBucket(ps *ProfileRiskState, eventTime time.Time, risk int) {
	if ps.Buckets == nil {
		ps.Buckets = map[string]int{}
	}
	ps.Buckets[minuteBucketKey(eventTime)] += risk
}

func calculateScores(ps ProfileRiskState, now time.Time) (int, int) {
	now = now.UTC()
	cut15 := now.Add(-15 * time.Minute)
	cut24 := now.Add(-24 * time.Hour)
	score15 := 0
	score24 := 0
	for key, score := range ps.Buckets {
		ts, err := time.Parse(time.RFC3339, key)
		if err != nil {
			continue
		}
		if ts.Before(cut24) {
			continue
		}
		score24 += score
		if !ts.Before(cut15) {
			score15 += score
		}
	}
	return score15, score24
}

func estimateTimeToBlockSeconds(ps ProfileRiskState, now time.Time, cfg Config) int {
	now = now.UTC()
	points := make([]bucketPoint, 0, len(ps.Buckets))
	for key, score := range ps.Buckets {
		ts, err := time.Parse(time.RFC3339, key)
		if err != nil {
			continue
		}
		points = append(points, bucketPoint{ts: ts.UTC(), score: score})
	}
	if len(points) == 0 {
		return 0
	}
	score15m, score24h := calculateScores(ps, now)
	bestETA := 0
	if eta := estimateWindowETASeconds(points, now, 15*time.Minute, score15m, cfg.ScoreBlockAt15m); eta > 0 {
		bestETA = eta
	}
	if eta := estimateWindowETASeconds(points, now, 24*time.Hour, score24h, cfg.ScoreBlockAt24h); eta > 0 && (bestETA == 0 || eta < bestETA) {
		bestETA = eta
	}
	return bestETA
}

type bucketPoint struct {
	ts    time.Time
	score int
}

func estimateWindowETASeconds(points []bucketPoint, now time.Time, window time.Duration, currentScore, threshold int) int {
	if currentScore >= threshold {
		return 0
	}
	lookback := 5 * time.Minute
	if window < lookback {
		lookback = window
	}
	cutoff := now.Add(-lookback)
	recentScore := 0
	for _, p := range points {
		if !p.ts.Before(cutoff) && !p.ts.After(now) {
			recentScore += p.score
		}
	}
	if recentScore <= 0 {
		return 0
	}
	ratePerSecond := float64(recentScore) / lookback.Seconds()
	if ratePerSecond <= 0 {
		return 0
	}
	remaining := float64(threshold - currentScore)
	eta := int(remaining / ratePerSecond)
	if eta <= 0 {
		return 1
	}
	return eta
}

func cleanupProfileBuckets(ps *ProfileRiskState, now time.Time) {
	if ps.Buckets == nil {
		ps.Buckets = map[string]int{}
		return
	}
	cutoff := now.UTC().Add(-24 * time.Hour)
	for key := range ps.Buckets {
		ts, err := time.Parse(time.RFC3339, key)
		if err != nil || ts.Before(cutoff) {
			delete(ps.Buckets, key)
		}
	}
}

func cleanupState(state *State, now time.Time) {
	for key, ps := range state.Profiles {
		cleanupProfileBuckets(&ps, now)
		state.Profiles[key] = ps
		if len(ps.Buckets) == 0 && ps.PendingBlockAt == "" && ps.LastBlockAt == "" {
			delete(state.Profiles, key)
		}
	}
}

func incrementSkippedEvent(state *State, cfg Config, reason string) {
	if !cfg.TrackSkippedEvents {
		return
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return
	}
	if state.SkippedEvents == nil {
		state.SkippedEvents = map[string]int{}
	}
	state.SkippedEvents[reason]++
}

func cleanupSkippedEvents(state *State, now time.Time, enabled bool) {
	if !enabled {
		state.SkippedEvents = map[string]int{}
		state.SkippedEventsResetAt = ""
		return
	}
	if state.SkippedEvents == nil {
		state.SkippedEvents = map[string]int{}
	}
	lastReset := parseEventTime(state.SkippedEventsResetAt)
	if lastReset.IsZero() {
		state.SkippedEventsResetAt = now.UTC().Format(time.RFC3339)
		return
	}
	if now.UTC().Sub(lastReset) < skippedEventsResetInterval {
		return
	}
	state.SkippedEvents = map[string]int{}
	state.SkippedEventsResetAt = now.UTC().Format(time.RFC3339)
}

func syncCursorToEOF(path string, cursor *CursorState) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat querylog for startup sync: %w", err)
	}
	cursor.Offset = info.Size()
	cursor.FileSize = info.Size()
	cursor.FileModTime = info.ModTime().UTC().Format(time.RFC3339Nano)
	cursor.LastSeenTime = ""
	cursor.RecentEventKeys = nil
	return nil
}

func processPendingBlocks(cfg Config, state *State, now time.Time) bool {
	changed := false
	for profileKey, ps := range state.Profiles {
		if ps.PendingBlockAt == "" || ps.LastBlockAt != "" {
			continue
		}
		dueAt, err := time.Parse(time.RFC3339, ps.PendingBlockAt)
		if err != nil || now.UTC().Before(dueAt) {
			continue
		}
		ref, ok := pendingProfileRef(profileKey, ps)
		if !ok {
			ps.PendingBlockAt = ""
			ps.PendingBlockWindow = ""
			ps.PendingBlockScore = 0
			state.Profiles[profileKey] = ps
			changed = true
			continue
		}
		actionResult := ""
		action := "block_applied"
		if result, err := maybeBlockProfile(ref, cfg, &ps); err != nil {
			log.Printf("block profile %s failed: %v", profileKey, err)
			action = "block_failed"
			actionResult = err.Error()
		} else {
			actionResult = strings.TrimSpace(result)
		}
		if evt := buildAppliedBlockNotification(ref, ps, action, actionResult); evt != nil {
			if err := deliverNotificationEvent(cfg, *evt); err != nil {
				log.Printf("deliver applied block notification warning: %v", err)
			}
		}
		ps.PendingBlockAt = ""
		ps.PendingBlockWindow = ""
		ps.PendingBlockScore = 0
		if ps.LastBlockAt != "" {
			ps.LastAction = "block"
		}
		state.Profiles[profileKey] = ps
		changed = true
	}
	return changed
}

func pendingProfileRef(profileKey string, ps ProfileRiskState) (ProfileRef, bool) {
	parts := strings.SplitN(profileKey, ":", 2)
	if len(parts) != 2 {
		return ProfileRef{}, false
	}
	return ProfileRef{
		Kind: parts[0],
		Name: parts[1],
		ID:   ps.LastProfileID,
		IP:   ps.LastProfileIP,
	}, true
}

func buildAppliedBlockNotification(ref ProfileRef, ps ProfileRiskState, action, actionResult string) *NotificationEvent {
	action = strings.TrimSpace(action)
	actionResult = strings.TrimSpace(actionResult)
	if action == "" {
		return nil
	}
	domains := []string{}
	if d := strings.TrimSpace(ps.LastMatchedDomain); d != "" {
		domains = append(domains, d)
	}
	return &NotificationEvent{
		ID:              fmt.Sprintf("%s-%s-%s-%d", action, ref.Kind, sanitizeFilename(ref.Name), time.Now().UnixNano()),
		Type:            "risk_notification",
		ProfileKind:     ref.Kind,
		ProfileName:     ref.Name,
		ProfileIP:       ref.IP,
		Risk:            ps.PendingBlockScore,
		TriggeredWindow: ps.PendingBlockWindow,
		Reason:          nonEmpty(ps.LastMatchedReason, "manual domain risk match"),
		Domains:         domains,
		MatchedRule:     ps.LastMatchedDomain,
		DetectedAt:      time.Now().UTC().Format(time.RFC3339),
		Action:          action,
		ActionResult:    actionResult,
	}
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
