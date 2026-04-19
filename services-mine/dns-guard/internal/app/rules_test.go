package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRefreshRulesIfChangedSkipsUnchangedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "risk-domains.json")
	if err := os.WriteFile(path, []byte(`{"rules":[{"domain":"a.example","match":"exact","risk":3,"reason":"a","enabled":true}]}`), 0644); err != nil {
		t.Fatalf("write rules: %v", err)
	}
	t.Setenv("DNS_GUARD_RULES", path)

	cfg := Config{MinRuleRisk: 0, MaxRuleRisk: 20}
	rules, snapshot, err := loadRulesFromPath(path, cfg)
	if err != nil {
		t.Fatalf("load initial rules: %v", err)
	}

	result, err := refreshRulesIfChanged(cfg, rules, snapshot)
	if err != nil {
		t.Fatalf("refresh unchanged rules: %v", err)
	}
	if result.Changed {
		t.Fatalf("expected unchanged rules result")
	}
	if len(result.Rules.Rules) != 1 || result.Rules.Rules[0].domain != "a.example" {
		t.Fatalf("unexpected cached rules: %#v", result.Rules)
	}
}

func TestRefreshRulesIfChangedReloadsModifiedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "risk-domains.json")
	if err := os.WriteFile(path, []byte(`{"rules":[{"domain":"a.example","match":"exact","risk":3,"reason":"a","enabled":true}]}`), 0644); err != nil {
		t.Fatalf("write initial rules: %v", err)
	}
	t.Setenv("DNS_GUARD_RULES", path)

	cfg := Config{MinRuleRisk: 0, MaxRuleRisk: 20}
	rules, snapshot, err := loadRulesFromPath(path, cfg)
	if err != nil {
		t.Fatalf("load initial rules: %v", err)
	}

	time.Sleep(1100 * time.Millisecond)
	if err := os.WriteFile(path, []byte(`{"rules":[{"domain":"b.example","match":"exact","risk":7,"reason":"b","enabled":true}]}`), 0644); err != nil {
		t.Fatalf("write updated rules: %v", err)
	}

	result, err := refreshRulesIfChanged(cfg, rules, snapshot)
	if err != nil {
		t.Fatalf("refresh changed rules: %v", err)
	}
	if !result.Changed {
		t.Fatalf("expected changed rules result")
	}
	if len(result.Rules.Rules) != 1 || result.Rules.Rules[0].domain != "b.example" || result.Rules.Rules[0].risk != 7 {
		t.Fatalf("unexpected reloaded rules: %#v", result.Rules)
	}
}

func TestLoadRulesFromPathLoadsExcludes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "risk-domains.json")
	raw := `{
		"rules":[{"domain":"example.com","match":"suffix","risk":5,"reason":"r","enabled":true}],
		"excludes":[
			{"domain":"safe.example.com","match":"exact","reason":"safe exact","enabled":true},
			{"domains":["cdn.example.com"],"match":"suffix","reason":"safe suffix","enabled":true}
		]
	}`
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatalf("write rules: %v", err)
	}

	cfg := Config{MinRuleRisk: 0, MaxRuleRisk: 20}
	rules, _, err := loadRulesFromPath(path, cfg)
	if err != nil {
		t.Fatalf("load rules: %v", err)
	}
	if len(rules.Rules) != 1 {
		t.Fatalf("expected 1 risk rule, got %d", len(rules.Rules))
	}
	if len(rules.Excludes) != 2 {
		t.Fatalf("expected 2 excludes, got %d", len(rules.Excludes))
	}
}

func TestMatchExcludePrefersMostSpecificDomain(t *testing.T) {
	excludes := []compiledMatcher{
		{domain: "example.com", match: "suffix", enabled: true},
		{domain: "safe.example.com", match: "exact", enabled: true},
	}
	match, ok := matchExclude("safe.example.com", excludes)
	if !ok {
		t.Fatal("expected exclude match")
	}
	if match.domain != "safe.example.com" || match.match != "exact" {
		t.Fatalf("unexpected exclude match: %#v", match)
	}
}
