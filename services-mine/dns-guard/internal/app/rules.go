package app

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

type RiskRulesFile struct {
	Rules []RiskRule `json:"rules"`
}

type RiskRule struct {
	Domain    string `json:"domain"`
	Match     string `json:"match"`
	Risk      int    `json:"risk"`
	Reason    string `json:"reason"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at,omitempty"`
}

type compiledRule struct {
	domain  string
	match   string
	risk    int
	reason  string
	enabled bool
}

func loadRules() ([]compiledRule, error) {
	path := strings.TrimSpace(os.Getenv("DNS_GUARD_RULES"))
	if path == "" {
		path = "/config/risk-domains.json"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read rules: %w", err)
	}
	var root RiskRulesFile
	if err := json.Unmarshal(b, &root); err != nil {
		return nil, fmt.Errorf("parse rules: %w", err)
	}
	out := make([]compiledRule, 0, len(root.Rules))
	for _, r := range root.Rules {
		match := strings.ToLower(strings.TrimSpace(r.Match))
		if match == "" {
			match = "exact"
		}
		if match != "exact" && match != "suffix" {
			continue
		}
		domain := normalizeDomain(r.Domain)
		if domain == "" {
			continue
		}
		risk := r.Risk
		if risk < 0 {
			risk = 0
		}
		if risk > 9 {
			risk = 9
		}
		enabled := r.Enabled
		if r.CreatedAt == "" {
			r.CreatedAt = time.Now().UTC().Format(time.RFC3339)
		}
		out = append(out, compiledRule{
			domain:  domain,
			match:   match,
			risk:    risk,
			reason:  strings.TrimSpace(r.Reason),
			enabled: enabled,
		})
	}
	return out, nil
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

func normalizeDomain(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, ".")
	s = strings.TrimSuffix(s, ".")
	return s
}
