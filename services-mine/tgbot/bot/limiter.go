package main

import (
	"strconv"
	"sync"
	"time"
)

type cooldownLimiter struct {
	mu       sync.Mutex
	cooldown time.Duration
	last     map[int64]time.Time
}

func newCooldownLimiter(cooldown time.Duration) *cooldownLimiter {
	return &cooldownLimiter{
		cooldown: cooldown,
		last:     make(map[int64]time.Time),
	}
}

// allow returns (ok, wait). For Admin, caller should bypass before calling.
func (l *cooldownLimiter) allow(userID int64) (bool, time.Duration) {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	if t, ok := l.last[userID]; ok {
		elapsed := now.Sub(t)
		if elapsed < l.cooldown {
			return false, l.cooldown - elapsed
		}
	}
	l.last[userID] = now
	return true, 0
}

func fmtDurationRu(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	sec := int(d.Round(time.Second).Seconds())
	if sec < 60 {
		return pluralRu(sec, "секунду", "секунды", "секунд")
	}
	min := sec / 60
	sec = sec % 60
	if sec == 0 {
		return pluralRu(min, "минуту", "минуты", "минут")
	}
	return pluralRu(min, "минуту", "минуты", "минут") + " " + pluralRu(sec, "секунду", "секунды", "секунд")
}

func pluralRu(n int, one, few, many string) string {
	nAbs := n
	if nAbs < 0 {
		nAbs = -nAbs
	}
	nMod100 := nAbs % 100
	nMod10 := nAbs % 10
	switch {
	case nMod100 >= 11 && nMod100 <= 14:
		return strconv.Itoa(n) + " " + many
	case nMod10 == 1:
		return strconv.Itoa(n) + " " + one
	case nMod10 >= 2 && nMod10 <= 4:
		return strconv.Itoa(n) + " " + few
	default:
		return strconv.Itoa(n) + " " + many
	}
}
