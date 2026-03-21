package bot

import (
	"fmt"
	"html"
	"sort"
	"strings"
	"time"
)

const wgOfflineAfter = 10 * time.Minute

func filterPeersByUserPrefixes(peers []wgEasyPeer, prefixesLower []string) []wgEasyPeer {
	if len(prefixesLower) == 0 {
		return nil
	}
	out := make([]wgEasyPeer, 0, len(peers))
	for _, p := range peers {
		nameLower := strings.ToLower(strings.TrimSpace(p.Name))
		if nameLower == "" {
			continue
		}
		for _, pref := range prefixesLower {
			if pref == "" {
				continue
			}
			if strings.HasPrefix(nameLower, pref) {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

func formatWgPeersStats(peers []wgEasyPeer) string {
	now := time.Now()

	// sort by latest activity (latest handshake desc). zero handshake -> last.
	sort.Slice(peers, func(i, j int) bool {
		ti := peers[i].latestHandshakeTime()
		tj := peers[j].latestHandshakeTime()
		if ti.Equal(tj) {
			return strings.ToLower(peers[i].Name) < strings.ToLower(peers[j].Name)
		}
		if ti.IsZero() {
			return false
		}
		if tj.IsZero() {
			return true
		}
		return ti.After(tj)
	})

	lines := make([]string, 0, len(peers)*7)
	for idx, p := range peers {
		if idx > 0 {
			lines = append(lines, "") // blank line between profiles
		}

		nameEsc := html.EscapeString(strings.TrimSpace(p.Name))
		last := p.latestHandshakeTime()
		delta, hasDelta := time.Duration(0), false
		if !last.IsZero() {
			delta = now.Sub(last)
			if delta < 0 {
				delta = 0
			}
			hasDelta = true
		}

		// If the old formatting produced "999999 мин", show a dash.
		showDash := (!hasDelta) || (delta.Minutes() >= 999999)
		offline := showDash || delta > wgOfflineAfter

		lastStr := "—"
		if !showDash {
			lastStr = humanSince(delta)
		}
		statusSuffix := ""
		if offline {
			statusSuffix = " (offline)"
		}

		lines = append(lines,
			fmt.Sprintf("<b>Profile: %s</b>", nameEsc),
			fmt.Sprintf("Last Handshake: %s%s", lastStr, statusSuffix),
			fmt.Sprintf("Enabled: %s", yesNo(p.Enabled)),
			fmt.Sprintf("Expires: %s", formatWgExpiry(p.ExpiresAt)),
			fmt.Sprintf("TX: %s", humanMBGB(p.TransferTx)),
			fmt.Sprintf("RX: %s", humanMBGB(p.TransferRx)),
		)
	}
	return strings.Join(lines, "\n")
}

func formatWgExpiry(v any) string {
	if v == nil {
		return "—"
	}

	switch x := v.(type) {
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return "—"
		}
		if t, ok := parseWgExpiryTime(s); ok {
			return t.Local().Format("2006-01-02 15:04:05")
		}
		return html.EscapeString(s)
	case float64:
		if x <= 0 {
			return "—"
		}
		return time.Unix(int64(x), 0).Local().Format("2006-01-02 15:04:05")
	case int64:
		if x <= 0 {
			return "—"
		}
		return time.Unix(x, 0).Local().Format("2006-01-02 15:04:05")
	case jsonNumberLike:
		if n, err := x.Int64(); err == nil && n > 0 {
			return time.Unix(n, 0).Local().Format("2006-01-02 15:04:05")
		}
		return "—"
	default:
		return html.EscapeString(fmt.Sprint(v))
	}
}

type jsonNumberLike interface {
	Int64() (int64, error)
}

func parseWgExpiryTime(s string) (time.Time, bool) {
	layouts := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func humanSince(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	sec := int(d.Seconds())
	if sec < 60 {
		return fmt.Sprintf("%d сек", sec)
	}
	min := sec / 60
	if min < 60 {
		return fmt.Sprintf("%d мин", min)
	}
	hr := min / 60
	if hr < 24 {
		return fmt.Sprintf("%d ч", hr)
	}
	day := hr / 24
	return fmt.Sprintf("%d д", day)
}

func humanMBGB(bytes int64) string {
	if bytes < 0 {
		bytes = 0
	}
	const (
		MB = 1024 * 1024
		GB = 1024 * 1024 * 1024
	)
	if bytes >= GB {
		return fmt.Sprintf("%.2f ГБ", float64(bytes)/float64(GB))
	}
	return fmt.Sprintf("%.2f МБ", float64(bytes)/float64(MB))
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
