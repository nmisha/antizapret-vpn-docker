package main

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

	lines := make([]string, 0, len(peers)*6)
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
			fmt.Sprintf("TX: %s", humanMBGB(p.TransferTx)),
			fmt.Sprintf("RX: %s", humanMBGB(p.TransferRx)),
		)
	}
	return strings.Join(lines, "\n")
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
