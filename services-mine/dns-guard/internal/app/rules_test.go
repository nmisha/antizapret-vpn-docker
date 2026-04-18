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
	if len(result.Rules) != 1 || result.Rules[0].domain != "a.example" {
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
	if len(result.Rules) != 1 || result.Rules[0].domain != "b.example" || result.Rules[0].risk != 7 {
		t.Fatalf("unexpected reloaded rules: %#v", result.Rules)
	}
}
