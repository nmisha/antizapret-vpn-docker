package app

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type RiskRulesFile struct {
	Rules    []RiskRule    `json:"rules"`
	Excludes []RuleMatcher `json:"excludes,omitempty"`
}

type RiskRule struct {
	Domain    string   `json:"domain"`
	Domains   []string `json:"domains,omitempty"`
	Match     string   `json:"match"`
	Risk      int      `json:"risk"`
	Reason    string   `json:"reason"`
	Enabled   bool     `json:"enabled"`
	CreatedAt string   `json:"created_at,omitempty"`
}

type RuleMatcher struct {
	Domain  string   `json:"domain"`
	Domains []string `json:"domains,omitempty"`
	Match   string   `json:"match"`
	Reason  string   `json:"reason,omitempty"`
	Enabled bool     `json:"enabled"`
}

type compiledRule struct {
	domain  string
	match   string
	risk    int
	reason  string
	enabled bool
}

type compiledMatcher struct {
	domain  string
	match   string
	reason  string
	enabled bool
}

type compiledRuleset struct {
	Rules    []compiledRule
	Excludes []compiledMatcher
}

type rulesFileSnapshot struct {
	Path    string
	Size    int64
	ModTime time.Time
}

type rulesReloadResult struct {
	Rules    compiledRuleset
	Snapshot rulesFileSnapshot
	Changed  bool
}

func loadRules(cfg Config) (compiledRuleset, error) {
	path := rulesPath()
	rules, _, err := loadRulesFromPath(path, cfg)
	return rules, err
}

func rulesPath() string {
	path := strings.TrimSpace(os.Getenv("DNS_GUARD_RULES"))
	if path == "" {
		path = "/config/risk-domains.json"
	}
	return path
}

func statRulesFile(path string) (rulesFileSnapshot, error) {
	info, err := os.Stat(path)
	if err != nil {
		return rulesFileSnapshot{}, fmt.Errorf("stat rules: %w", err)
	}
	return rulesFileSnapshot{
		Path:    path,
		Size:    info.Size(),
		ModTime: info.ModTime().UTC(),
	}, nil
}

func loadRulesFromPath(path string, cfg Config) (compiledRuleset, rulesFileSnapshot, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return compiledRuleset{}, rulesFileSnapshot{}, fmt.Errorf("read rules: %w", err)
	}
	var root RiskRulesFile
	if err := json.Unmarshal(b, &root); err != nil {
		return compiledRuleset{}, rulesFileSnapshot{}, fmt.Errorf("parse rules: %w", err)
	}
	out := compiledRuleset{
		Rules:    make([]compiledRule, 0, len(root.Rules)),
		Excludes: make([]compiledMatcher, 0, len(root.Excludes)),
	}
	for _, r := range root.Rules {
		match := strings.ToLower(strings.TrimSpace(r.Match))
		if match == "" {
			match = "exact"
		}
		if match != "exact" && match != "suffix" {
			continue
		}
		risk := r.Risk
		if risk < cfg.MinRuleRisk {
			risk = cfg.MinRuleRisk
		}
		if risk > cfg.MaxRuleRisk {
			risk = cfg.MaxRuleRisk
		}
		enabled := r.Enabled
		if r.CreatedAt == "" {
			r.CreatedAt = time.Now().UTC().Format(time.RFC3339)
		}
		domains := normalizeRuleDomains(r)
		if len(domains) == 0 {
			continue
		}
		for _, domain := range domains {
			out.Rules = append(out.Rules, compiledRule{
				domain:  domain,
				match:   match,
				risk:    risk,
				reason:  strings.TrimSpace(r.Reason),
				enabled: enabled,
			})
		}
	}
	for _, r := range root.Excludes {
		match := strings.ToLower(strings.TrimSpace(r.Match))
		if match == "" {
			match = "exact"
		}
		if match != "exact" && match != "suffix" {
			continue
		}
		domains := normalizeMatcherDomains(r.Domain, r.Domains)
		if len(domains) == 0 {
			continue
		}
		for _, domain := range domains {
			out.Excludes = append(out.Excludes, compiledMatcher{
				domain:  domain,
				match:   match,
				reason:  strings.TrimSpace(r.Reason),
				enabled: r.Enabled,
			})
		}
	}
	snapshot, err := statRulesFile(path)
	if err != nil {
		return compiledRuleset{}, rulesFileSnapshot{}, err
	}
	return out, snapshot, nil
}

func refreshRulesIfChanged(cfg Config, current compiledRuleset, prev rulesFileSnapshot) (rulesReloadResult, error) {
	path := rulesPath()
	snapshot, err := statRulesFile(path)
	if err != nil {
		return rulesReloadResult{}, err
	}
	if prev.Path != "" && prev.Path == snapshot.Path && prev.Size == snapshot.Size && prev.ModTime.Equal(snapshot.ModTime) {
		return rulesReloadResult{
			Rules:    current,
			Snapshot: prev,
			Changed:  false,
		}, nil
	}
	rules, loadedSnapshot, err := loadRulesFromPath(path, cfg)
	if err != nil {
		return rulesReloadResult{}, err
	}
	return rulesReloadResult{
		Rules:    rules,
		Snapshot: loadedSnapshot,
		Changed:  true,
	}, nil
}

func normalizeRuleDomains(r RiskRule) []string {
	return normalizeMatcherDomains(r.Domain, r.Domains)
}

func normalizeMatcherDomains(domain string, domains []string) []string {
	candidates := make([]string, 0, 1+len(domains))
	if strings.TrimSpace(domain) != "" {
		candidates = append(candidates, domain)
	}
	candidates = append(candidates, domains...)
	seen := map[string]struct{}{}
	out := make([]string, 0, len(candidates))
	for _, raw := range candidates {
		domain := normalizeDomain(raw)
		if domain == "" {
			continue
		}
		if _, ok := seen[domain]; ok {
			continue
		}
		seen[domain] = struct{}{}
		out = append(out, domain)
	}
	return out
}

func countEnabledRules(rules []compiledRule) int {
	total := 0
	for _, r := range rules {
		if r.enabled {
			total++
		}
	}
	return total
}

func countEnabledMatchers(matchers []compiledMatcher) int {
	total := 0
	for _, m := range matchers {
		if m.enabled {
			total++
		}
	}
	return total
}

func matchRule(domain string, rules []compiledRule) (compiledRule, bool) {
	normalized := normalizeDomain(domain)
	var best compiledRule
	found := false
	for _, r := range rules {
		if !r.enabled {
			continue
		}
		ok := false
		switch r.match {
		case "exact":
			ok = normalized == r.domain
		case "suffix":
			ok = normalized == r.domain || strings.HasSuffix(normalized, "."+r.domain)
		}
		if !ok {
			continue
		}
		if !found || r.risk > best.risk || (r.risk == best.risk && len(r.domain) > len(best.domain)) {
			best = r
			found = true
		}
	}
	return best, found
}

func matchExclude(domain string, excludes []compiledMatcher) (compiledMatcher, bool) {
	normalized := normalizeDomain(domain)
	var best compiledMatcher
	found := false
	for _, r := range excludes {
		if !r.enabled {
			continue
		}
		ok := false
		switch r.match {
		case "exact":
			ok = normalized == r.domain
		case "suffix":
			ok = normalized == r.domain || strings.HasSuffix(normalized, "."+r.domain)
		}
		if !ok {
			continue
		}
		if !found || len(r.domain) > len(best.domain) {
			best = r
			found = true
		}
	}
	return best, found
}

func normalizeDomain(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, ".")
	s = strings.TrimSuffix(s, ".")
	return s
}

func rulesSignature(set compiledRuleset) string {
	if len(set.Rules) == 0 && len(set.Excludes) == 0 {
		return ""
	}
	parts := make([]string, 0, len(set.Rules)+len(set.Excludes))
	for _, r := range set.Rules {
		parts = append(parts, strings.Join([]string{
			"rule",
			r.domain,
			r.match,
			strconv.Itoa(r.risk),
			r.reason,
			strconv.FormatBool(r.enabled),
		}, "|"))
	}
	for _, r := range set.Excludes {
		parts = append(parts, strings.Join([]string{
			"exclude",
			r.domain,
			r.match,
			r.reason,
			strconv.FormatBool(r.enabled),
		}, "|"))
	}
	return strings.Join(parts, "\n")
}
