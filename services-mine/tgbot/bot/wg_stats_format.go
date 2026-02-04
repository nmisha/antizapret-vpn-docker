package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

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
		t := p.latestHandshakeTime()
		mins := 999999
		if !t.IsZero() {
			mins = int(now.Sub(t).Minutes())
			if mins < 0 {
				mins = 0
			}
		}
		status := "online"
		if mins > 60 || t.IsZero() {
			status = "offline"
		}

		lines = append(lines,
			fmt.Sprintf("Название: %s", p.Name),
			fmt.Sprintf("Handshake: %d мин (%s)", mins, status),
			fmt.Sprintf("RX: %s", humanMBGB(p.TransferRx)),
			fmt.Sprintf("TX: %s", humanMBGB(p.TransferTx)),
		)
	}
	return strings.Join(lines, "\n")
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
