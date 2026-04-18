package app

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	PollIntervalSeconds  int                 `json:"poll_interval_seconds"`
	QueryLogPath         string              `json:"querylog_path"`
	StatePath            string              `json:"state_path"`
	NotificationInboxDir string              `json:"notification_inbox_dir"`
	NotificationAPIURL   string              `json:"notification_api_url"`
	NotificationAPIToken string              `json:"notification_api_token"`
	NotificationCooldown int                 `json:"notification_cooldown_seconds"`
	RiskNotifyFrom       int                 `json:"risk_notify_from"`
	RiskBlockAt          int                 `json:"risk_block_at"`
	MatchMode            string              `json:"match_mode"`
	Subnets              map[string][]string `json:"subnets"`
	IgnoreIPs            []string            `json:"ignore_ips"`
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
	if cfg.PollIntervalSeconds <= 0 {
		cfg.PollIntervalSeconds = 60
	}
	if cfg.NotificationCooldown <= 0 {
		cfg.NotificationCooldown = int((6 * time.Hour).Seconds())
	}
	if cfg.RiskNotifyFrom <= 0 {
		cfg.RiskNotifyFrom = 6
	}
	if cfg.RiskBlockAt <= 0 {
		cfg.RiskBlockAt = 9
	}
	if cfg.MatchMode == "" {
		cfg.MatchMode = "max"
	}
	if cfg.NotificationInboxDir == "" {
		cfg.NotificationInboxDir = "/tgbot/data/notifications/inbox"
	}
	if cfg.QueryLogPath == "" || cfg.StatePath == "" {
		return Config{}, fmt.Errorf("querylog_path and state_path are required")
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
