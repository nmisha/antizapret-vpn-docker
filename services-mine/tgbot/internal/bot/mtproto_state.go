package bot

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"
)

var mtpScopeMu sync.Mutex
var mtpAdminScopes = map[int64]wgAdminScope{}

func setMtpAdminScope(tgID int64, s wgAdminScope) {
	mtpScopeMu.Lock()
	defer mtpScopeMu.Unlock()
	mtpAdminScopes[tgID] = s
}

func getMtpAdminScope(tgID int64) (wgAdminScope, bool) {
	mtpScopeMu.Lock()
	defer mtpScopeMu.Unlock()
	s, ok := mtpAdminScopes[tgID]
	return s, ok
}

func clearMtpAdminScope(tgID int64) {
	mtpScopeMu.Lock()
	defer mtpScopeMu.Unlock()
	delete(mtpAdminScopes, tgID)
}

type mtpPendingKind string

const (
	mtpPendingAdd   mtpPendingKind = "add"
	mtpPendingLimit mtpPendingKind = "limit"
)

type mtpPending struct {
	Kind  mtpPendingKind
	Name  string // telemt username (limit)
	Field string // limit field key (limit)
}

var mtpPendMu sync.Mutex
var mtpPendings = map[int64]mtpPending{}

func setMtpPending(tgID int64, p mtpPending) {
	privatePendingMu.Lock()
	defer privatePendingMu.Unlock()
	clearPrivatePendings(tgID)
	mtpPendMu.Lock()
	defer mtpPendMu.Unlock()
	mtpPendings[tgID] = p
}

func popMtpPending(tgID int64) (mtpPending, bool) {
	mtpPendMu.Lock()
	defer mtpPendMu.Unlock()
	p, ok := mtpPendings[tgID]
	if ok {
		delete(mtpPendings, tgID)
	}
	return p, ok
}

// Per-user limits editable from the bot. Key is the short code used in callback data.
type mtpLimitField struct {
	Key   string
	Label string
	JSON  string // PatchUserRequest field
	Hint  string
}

var mtpLimitFields = []mtpLimitField{
	{Key: "ips", Label: "Устройства (IP)", JSON: "max_unique_ips", Hint: "целое число, например 2"},
	{Key: "conns", Label: "TCP-соединения", JSON: "max_tcp_conns", Hint: "целое число, например 30 (один клиент держит ~8 соединений)"},
	{Key: "exp", Label: "Срок", JSON: "expiration_rfc3339", Hint: "дата 2026-12-31, RFC3339 2026-12-31T23:59:59Z или срок от сейчас: 30d"},
	{Key: "quota", Label: "Квота трафика", JSON: "data_quota_bytes", Hint: "размер: 500M, 10G, 1T (или байты)"},
	{Key: "up", Label: "Скорость ↑", JSON: "rate_limit_up_bps", Hint: "бит/с: 10M = 10 Мбит/с, 512K"},
	{Key: "down", Label: "Скорость ↓", JSON: "rate_limit_down_bps", Hint: "бит/с: 10M = 10 Мбит/с, 512K"},
}

func mtpLimitFieldByKey(key string) (mtpLimitField, bool) {
	for _, f := range mtpLimitFields {
		if f.Key == key {
			return f, true
		}
	}
	return mtpLimitField{}, false
}

// parseMtpLimitValue converts admin input into a PatchUserRequest value.
// "none"/"-" returns nil: the individual limit is removed and the [access] default applies.
func parseMtpLimitValue(f mtpLimitField, input string, now time.Time) (any, error) {
	s := strings.TrimSpace(input)
	if strings.EqualFold(s, "none") || s == "-" {
		return nil, nil
	}
	switch f.Key {
	case "ips", "conns":
		n, err := strconv.ParseUint(s, 10, 64)
		if err != nil || n == 0 {
			return nil, fmt.Errorf("нужно положительное целое число или none")
		}
		return n, nil
	case "exp":
		return parseMtpExpiration(s, now)
	case "quota":
		return parseSizeWithSuffix(s, 1024)
	case "up", "down":
		return parseSizeWithSuffix(s, 1000)
	}
	return nil, fmt.Errorf("неизвестное поле %s", f.Key)
}

func parseMtpExpiration(s string, now time.Time) (string, error) {
	if strings.HasSuffix(s, "d") {
		if n, err := strconv.Atoi(strings.TrimSuffix(s, "d")); err == nil && n > 0 {
			return now.UTC().Add(time.Duration(n) * 24 * time.Hour).Format(time.RFC3339), nil
		}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Format(time.RFC3339), nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.Add(24*time.Hour - time.Second).Format(time.RFC3339), nil
	}
	return "", fmt.Errorf("не понял дату: нужно 2026-12-31, 2026-12-31T23:59:59Z, 30d или none")
}

// parseSizeWithSuffix parses "10G", "512K", "1.5M" or a plain number; base is 1024 (bytes) or 1000 (bits/s).
func parseSizeWithSuffix(s string, base float64) (uint64, error) {
	u := strings.ToUpper(strings.TrimSpace(s))
	mult := 1.0
	for i, suf := range []string{"K", "M", "G", "T"} {
		if strings.HasSuffix(u, suf) {
			u = strings.TrimSuffix(u, suf)
			for j := 0; j <= i; j++ {
				mult *= base
			}
			break
		}
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(u), 64)
	scaled := v * mult
	// 2^64 is the exclusive upper bound; float64 rounds MaxUint64 up to it.
	if err != nil || math.IsNaN(scaled) || math.IsInf(scaled, 0) || scaled < 1 || scaled >= math.Exp2(64) {
		return 0, fmt.Errorf("нужно число с суффиксом K/M/G/T или none; результат должен быть от 1 до 18446744073709551615")
	}
	return uint64(scaled), nil
}
