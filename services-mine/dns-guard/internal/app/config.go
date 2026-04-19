package app

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	Enabled                     bool                `json:"enabled"`
	PollIntervalSeconds         int                 `json:"poll_interval_seconds"`
	HTTPListenAddr              string              `json:"http_listen_addr"`
	HTTPAPIToken                string              `json:"http_api_token"`
	QueryLogSource              string              `json:"querylog_source"`
	QueryLogPath                string              `json:"querylog_path"`
	StatePath                   string              `json:"state_path"`
	NotificationAPIURL          string              `json:"notification_api_url"`
	NotificationAPIToken        string              `json:"notification_api_token"`
	NotificationCooldown        int                 `json:"notification_cooldown_seconds"`
	DebugLogEnabled             bool                `json:"debug_log_enabled"`
	TrackSkippedEvents          bool                `json:"track_skipped_events"`
	SkippedEventsResetHours     int                 `json:"skipped_events_reset_hours"`
	HistoryCatchupEnabled       bool                `json:"history_catchup_enabled"`
	HistoryCatchupMaxAgeMinutes int                 `json:"history_catchup_max_age_minutes"`
	HistoryCatchupMaxRecords    int                 `json:"history_catchup_max_records"`
	MinRuleRisk                 int                 `json:"min_rule_risk"`
	MaxRuleRisk                 int                 `json:"max_rule_risk"`
	ScoreNotifyAt               int                 `json:"score_notify_at"`
	ScoreBlockAt15m             int                 `json:"score_block_at_15m"`
	ScoreBlockAt24h             int                 `json:"score_block_at_24h"`
	BlockDelaySeconds           int                 `json:"block_delay_seconds"`
	PredictBlockETA             bool                `json:"predict_block_eta"`
	Subnets                     map[string][]string `json:"subnets"`
	IgnoreIPs                   []string            `json:"ignore_ips"`
	ProfileWhitelist            map[string][]string `json:"profile_whitelist"`
	AdGuard                     AdGuardAPIConfig    `json:"adguard"`
	WG                          GuardAPIConfig      `json:"wg"`
	AWG                         GuardAPIConfig      `json:"awg"`
	OVPN                        OVPNAPIConfig       `json:"ovpn"`
}

type AdGuardAPIConfig struct {
	Scheme         string `json:"scheme"`
	Host           string `json:"host"`
	Port           string `json:"port"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	PageLimit      int    `json:"page_limit"`
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
	Enabled       bool   `json:"enabled"`
	Host          string `json:"host"`
	Port          string `json:"port"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	Block         bool   `json:"block"`
	RestartServer bool   `json:"restart_server"`
}

type configFileSnapshot struct {
	Path    string
	Size    int64
	ModTime time.Time
}

func loadConfig() (Config, error) {
	path := configPath()
	cfg, _, err := loadConfigFromPath(path)
	return cfg, err
}

func configPath() string {
	path := strings.TrimSpace(os.Getenv("DNS_GUARD_CONFIG"))
	if path == "" {
		path = "/config/config.json"
	}
	return path
}

func statConfigFile(path string) (configFileSnapshot, error) {
	info, err := os.Stat(path)
	if err != nil {
		return configFileSnapshot{}, fmt.Errorf("stat config: %w", err)
	}
	return configFileSnapshot{
		Path:    path,
		Size:    info.Size(),
		ModTime: info.ModTime().UTC(),
	}, nil
}

func loadConfigFromPath(path string) (Config, configFileSnapshot, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, configFileSnapshot{}, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, configFileSnapshot{}, fmt.Errorf("parse config: %w", err)
	}
	if v := strings.TrimSpace(os.Getenv("DNS_GUARD_QUERYLOG")); v != "" {
		cfg.QueryLogPath = v
	}
	if v := strings.TrimSpace(os.Getenv("DNS_GUARD_HTTP_ADDR")); v != "" {
		cfg.HTTPListenAddr = v
	}
	if v := strings.TrimSpace(os.Getenv("DNS_GUARD_HTTP_TOKEN")); v != "" {
		cfg.HTTPAPIToken = v
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
	overrideAdGuardAPIFromEnv(&cfg.AdGuard)
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
	if cfg.SkippedEventsResetHours <= 0 {
		cfg.SkippedEventsResetHours = 36
	}
	if cfg.HistoryCatchupMaxAgeMinutes <= 0 {
		cfg.HistoryCatchupMaxAgeMinutes = 10
	}
	if cfg.HistoryCatchupMaxRecords <= 0 {
		cfg.HistoryCatchupMaxRecords = 1000
	}
	if cfg.AdGuard.Scheme == "" {
		cfg.AdGuard.Scheme = "http"
	}
	if cfg.AdGuard.TimeoutSeconds <= 0 {
		cfg.AdGuard.TimeoutSeconds = 15
	}
	if cfg.AdGuard.PageLimit <= 0 {
		cfg.AdGuard.PageLimit = 500
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
	switch queryLogSource(cfg) {
	case "api":
		if strings.TrimSpace(cfg.AdGuard.Host) == "" || strings.TrimSpace(cfg.AdGuard.Port) == "" || strings.TrimSpace(cfg.AdGuard.Username) == "" || strings.TrimSpace(cfg.AdGuard.Password) == "" {
			return Config{}, configFileSnapshot{}, fmt.Errorf("adguard.host, adguard.port, adguard.username and adguard.password are required for querylog_source=api")
		}
	case "file":
		if strings.TrimSpace(cfg.QueryLogPath) == "" {
			return Config{}, configFileSnapshot{}, fmt.Errorf("querylog_path is required for querylog_source=file")
		}
	default:
		return Config{}, configFileSnapshot{}, fmt.Errorf("unsupported querylog_source %q", cfg.QueryLogSource)
	}
	if cfg.StatePath == "" || strings.TrimSpace(cfg.NotificationAPIURL) == "" {
		return Config{}, configFileSnapshot{}, fmt.Errorf("state_path and notification_api_url are required")
	}
	snapshot, err := statConfigFile(path)
	if err != nil {
		return Config{}, configFileSnapshot{}, err
	}
	return cfg, snapshot, nil
}

func refreshConfigIfChanged(current Config, prev configFileSnapshot) (Config, configFileSnapshot, bool, error) {
	path := configPath()
	snapshot, err := statConfigFile(path)
	if err != nil {
		return Config{}, configFileSnapshot{}, false, err
	}
	if prev.Path != "" && prev.Path == snapshot.Path && prev.Size == snapshot.Size && prev.ModTime.Equal(snapshot.ModTime) {
		return current, prev, false, nil
	}
	cfg, loadedSnapshot, err := loadConfigFromPath(path)
	if err != nil {
		return Config{}, configFileSnapshot{}, false, err
	}
	return cfg, loadedSnapshot, true, nil
}

func queryLogSource(cfg Config) string {
	src := strings.ToLower(strings.TrimSpace(cfg.QueryLogSource))
	switch src {
	case "api", "file":
		return src
	case "":
		if strings.TrimSpace(cfg.QueryLogPath) != "" {
			return "file"
		}
		if strings.TrimSpace(cfg.AdGuard.Host) != "" {
			return "api"
		}
		return "file"
	default:
		return src
	}
}

func overrideAdGuardAPIFromEnv(dst *AdGuardAPIConfig) {
	if v := strings.TrimSpace(os.Getenv("DNS_GUARD_AGH_SCHEME")); v != "" {
		dst.Scheme = v
	}
	if v := strings.TrimSpace(os.Getenv("DNS_GUARD_AGH_HOST")); v != "" {
		dst.Host = v
	}
	if v := strings.TrimSpace(os.Getenv("DNS_GUARD_AGH_PORT")); v != "" {
		dst.Port = v
	}
	if v := strings.TrimSpace(os.Getenv("DNS_GUARD_AGH_USERNAME")); v != "" {
		dst.Username = v
	}
	if v := strings.TrimSpace(os.Getenv("DNS_GUARD_AGH_PASSWORD")); v != "" {
		dst.Password = v
	}
	if v := strings.TrimSpace(os.Getenv("DNS_GUARD_AGH_TIMEOUT_SECONDS")); v != "" {
		if n, err := parsePositiveInt(v); err == nil {
			dst.TimeoutSeconds = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("DNS_GUARD_AGH_PAGE_LIMIT")); v != "" {
		if n, err := parsePositiveInt(v); err == nil {
			dst.PageLimit = n
		}
	}
}

func parsePositiveInt(v string) (int, error) {
	n := 0
	for _, ch := range v {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("not a positive integer")
		}
		n = n*10 + int(ch-'0')
	}
	if n <= 0 {
		return 0, fmt.Errorf("not a positive integer")
	}
	return n, nil
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
