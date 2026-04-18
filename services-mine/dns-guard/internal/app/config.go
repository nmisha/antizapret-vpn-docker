package app

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	Enabled              bool                `json:"enabled"`
	PollIntervalSeconds  int                 `json:"poll_interval_seconds"`
	QueryLogPath         string              `json:"querylog_path"`
	StatePath            string              `json:"state_path"`
	NotificationAPIURL   string              `json:"notification_api_url"`
	NotificationAPIToken string              `json:"notification_api_token"`
	NotificationCooldown int                 `json:"notification_cooldown_seconds"`
	DebugLogEnabled      bool                `json:"debug_log_enabled"`
	TrackSkippedEvents   bool                `json:"track_skipped_events"`
	MinRuleRisk          int                 `json:"min_rule_risk"`
	MaxRuleRisk          int                 `json:"max_rule_risk"`
	ScoreNotifyAt        int                 `json:"score_notify_at"`
	ScoreBlockAt15m      int                 `json:"score_block_at_15m"`
	ScoreBlockAt24h      int                 `json:"score_block_at_24h"`
	BlockDelaySeconds    int                 `json:"block_delay_seconds"`
	PredictBlockETA      bool                `json:"predict_block_eta"`
	Subnets              map[string][]string `json:"subnets"`
	IgnoreIPs            []string            `json:"ignore_ips"`
	ProfileWhitelist     map[string][]string `json:"profile_whitelist"`
	WG                   GuardAPIConfig      `json:"wg"`
	AWG                  GuardAPIConfig      `json:"awg"`
	OVPN                 OVPNAPIConfig       `json:"ovpn"`
}

type GuardAPIConfig struct {
	Enabled  bool   `json:"enabled"`
	Host     string `json:"host"`
	Port     string `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Block    bool   `json:"block"`
}

type OVPNAPIConfig struct {
	Enabled  bool   `json:"enabled"`
	Host     string `json:"host"`
	Port     string `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Block    bool   `json:"block"`
}

func loadConfig() (Config, error) {
	path := strings.TrimSpace(os.Getenv("DNS_GUARD_CONFIG"))
	if path == "" {
		path = "/config/config.json"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if v := strings.TrimSpace(os.Getenv("DNS_GUARD_QUERYLOG")); v != "" {
		cfg.QueryLogPath = v
	}
	if v := strings.TrimSpace(os.Getenv("DNS_GUARD_STATE")); v != "" {
		cfg.StatePath = v
	}
	if v := strings.TrimSpace(os.Getenv("DNS_GUARD_NOTIFICATION_URL")); v != "" {
		cfg.NotificationAPIURL = v
	}
	if v := strings.TrimSpace(os.Getenv("DNS_GUARD_NOTIFICATION_TOKEN")); v != "" {
		cfg.NotificationAPIToken = v
	}
	overrideGuardAPIFromEnv(&cfg.WG, "WG")
	overrideGuardAPIFromEnv(&cfg.AWG, "AWG")
	overrideOVPNAPIFromEnv(&cfg.OVPN)
	if !cfg.Enabled {
		// keep zero-value false only when explicitly set? default to enabled for backward compatibility
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(b, &raw); err == nil {
			if _, ok := raw["enabled"]; !ok {
				cfg.Enabled = true
			}
		}
	}
	if cfg.PollIntervalSeconds <= 0 {
		cfg.PollIntervalSeconds = 60
	}
	if cfg.NotificationCooldown <= 0 {
		cfg.NotificationCooldown = int((6 * time.Hour).Seconds())
	}
	if cfg.MinRuleRisk < 0 {
		cfg.MinRuleRisk = 0
	}
	if cfg.MaxRuleRisk <= 0 {
		cfg.MaxRuleRisk = 9
	}
	if cfg.MaxRuleRisk < cfg.MinRuleRisk {
		cfg.MaxRuleRisk = cfg.MinRuleRisk
	}
	if cfg.ScoreNotifyAt <= 0 {
		cfg.ScoreNotifyAt = 12
	}
	if cfg.ScoreBlockAt15m <= 0 {
		cfg.ScoreBlockAt15m = 18
	}
	if cfg.ScoreBlockAt24h <= 0 {
		cfg.ScoreBlockAt24h = 36
	}
	if cfg.BlockDelaySeconds <= 0 {
		cfg.BlockDelaySeconds = 30
	}
	if !cfg.PredictBlockETA {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(b, &raw); err == nil {
			if _, ok := raw["predict_block_eta"]; !ok {
				cfg.PredictBlockETA = true
			}
		}
	}
	if cfg.QueryLogPath == "" || cfg.StatePath == "" || strings.TrimSpace(cfg.NotificationAPIURL) == "" {
		return Config{}, fmt.Errorf("querylog_path, state_path and notification_api_url are required")
	}
	return cfg, nil
}

func overrideGuardAPIFromEnv(dst *GuardAPIConfig, prefix string) {
	if v := strings.TrimSpace(os.Getenv(prefix + "_HOST")); v != "" {
		dst.Host = v
	}
	if v := strings.TrimSpace(os.Getenv(prefix + "_PORT")); v != "" {
		dst.Port = v
	}
	if v := strings.TrimSpace(os.Getenv(prefix + "_USERNAME")); v != "" {
		dst.Username = v
	}
	if v := strings.TrimSpace(os.Getenv(prefix + "_PASSWORD")); v != "" {
		dst.Password = v
	}
}

func overrideOVPNAPIFromEnv(dst *OVPNAPIConfig) {
	if v := strings.TrimSpace(os.Getenv("OVPN_HOST")); v != "" {
		dst.Host = v
	}
	if v := strings.TrimSpace(os.Getenv("OVPN_PORT")); v != "" {
		dst.Port = v
	}
	if v := strings.TrimSpace(os.Getenv("OVPN_USERNAME")); v != "" {
		dst.Username = v
	}
	if v := strings.TrimSpace(os.Getenv("OVPN_PASSWORD")); v != "" {
		dst.Password = v
	}
}
