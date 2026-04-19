package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRefreshConfigIfChangedSkipsUnchangedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{
		"enabled": true,
		"score_notify_at": 10,
		"wg": {"enabled": true},
		"awg": {"enabled": true},
		"ovpn": {"enabled": true}
	}`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("DNS_GUARD_CONFIG", path)
	t.Setenv("DNS_GUARD_QUERYLOG", "/adguard/querylog.json")
	t.Setenv("DNS_GUARD_STATE", "/state/state.json")
	t.Setenv("DNS_GUARD_NOTIFICATION_URL", "http://example")

	cfg, snapshot, err := loadConfigFromPath(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	nextCfg, nextSnapshot, changed, err := refreshConfigIfChanged(cfg, snapshot)
	if err != nil {
		t.Fatalf("refresh config: %v", err)
	}
	if changed {
		t.Fatal("expected unchanged config result")
	}
	if nextCfg.ScoreNotifyAt != cfg.ScoreNotifyAt {
		t.Fatalf("unexpected config change: %d vs %d", nextCfg.ScoreNotifyAt, cfg.ScoreNotifyAt)
	}
	if nextSnapshot != snapshot {
		t.Fatal("expected same snapshot for unchanged config")
	}
}

func TestRefreshConfigIfChangedReloadsModifiedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{
		"enabled": true,
		"score_notify_at": 10,
		"wg": {"enabled": true},
		"awg": {"enabled": true},
		"ovpn": {"enabled": true}
	}`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("DNS_GUARD_CONFIG", path)
	t.Setenv("DNS_GUARD_QUERYLOG", "/adguard/querylog.json")
	t.Setenv("DNS_GUARD_STATE", "/state/state.json")
	t.Setenv("DNS_GUARD_NOTIFICATION_URL", "http://example")

	cfg, snapshot, err := loadConfigFromPath(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	time.Sleep(1100 * time.Millisecond)
	if err := os.WriteFile(path, []byte(`{
		"enabled": true,
		"score_notify_at": 7,
		"wg": {"enabled": true},
		"awg": {"enabled": true},
		"ovpn": {"enabled": true}
	}`), 0644); err != nil {
		t.Fatalf("rewrite config: %v", err)
	}

	nextCfg, _, changed, err := refreshConfigIfChanged(cfg, snapshot)
	if err != nil {
		t.Fatalf("refresh config: %v", err)
	}
	if !changed {
		t.Fatal("expected changed config result")
	}
	if nextCfg.ScoreNotifyAt != 7 {
		t.Fatalf("expected reloaded notify score 7, got %d", nextCfg.ScoreNotifyAt)
	}
}

func TestLoadConfigAppliesSkippedEventsResetDefaultAndOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{
		"enabled": true,
		"querylog_source": "file",
		"querylog_path": "/adguard/querylog.json",
		"state_path": "/state/state.json",
		"notification_api_url": "http://example",
		"wg": {"enabled": true},
		"awg": {"enabled": true},
		"ovpn": {"enabled": true}
	}`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, _, err := loadConfigFromPath(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.SkippedEventsResetHours != 36 {
		t.Fatalf("expected default skipped reset 36h, got %d", cfg.SkippedEventsResetHours)
	}

	if err := os.WriteFile(path, []byte(`{
		"enabled": true,
		"querylog_source": "file",
		"querylog_path": "/adguard/querylog.json",
		"state_path": "/state/state.json",
		"notification_api_url": "http://example",
		"skipped_events_reset_hours": 12,
		"wg": {"enabled": true},
		"awg": {"enabled": true},
		"ovpn": {"enabled": true}
	}`), 0644); err != nil {
		t.Fatalf("rewrite config: %v", err)
	}
	cfg, _, err = loadConfigFromPath(path)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if cfg.SkippedEventsResetHours != 12 {
		t.Fatalf("expected overridden skipped reset 12h, got %d", cfg.SkippedEventsResetHours)
	}
}
