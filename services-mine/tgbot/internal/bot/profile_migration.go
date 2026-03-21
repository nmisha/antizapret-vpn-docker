package bot

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

func precheckMigrationClients(sourceClient, targetClient *wgEasyClient, sourceLabel, targetLabel string) error {
	if _, err := sourceClient.listPeers(); err != nil {
		return fmt.Errorf("%s недоступен: %w", sourceLabel, err)
	}
	if _, err := targetClient.listPeers(); err != nil {
		return fmt.Errorf("%s недоступен: %w", targetLabel, err)
	}
	return nil
}

func findPeerByExactName(peers []wgEasyPeer, name string) (wgEasyPeer, bool) {
	want := strings.ToLower(strings.TrimSpace(name))
	for _, p := range peers {
		if strings.ToLower(strings.TrimSpace(p.Name)) == want {
			return p, true
		}
	}
	return wgEasyPeer{}, false
}

func isIPv4AddressFree(peers []wgEasyPeer, ipv4 string) bool {
	want := strings.TrimSpace(ipv4)
	if want == "" {
		return false
	}
	for _, p := range peers {
		if strings.TrimSpace(p.IPv4Address) == want {
			return false
		}
	}
	return true
}

func lastIPv4Octet(ip string) (byte, bool) {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return 0, false
	}
	ipv4 := parsed.To4()
	if ipv4 == nil {
		return 0, false
	}
	return ipv4[3], true
}

func replaceLastIPv4Octet(ip string, octet byte) (string, bool) {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return "", false
	}
	ipv4 := parsed.To4()
	if ipv4 == nil {
		return "", false
	}
	ipv4[3] = octet
	return ipv4.String(), true
}

func preserveIPv4HostPart(sourceIPv4, targetIPv4 string, targetPeers []wgEasyPeer) (string, bool) {
	last, ok := lastIPv4Octet(sourceIPv4)
	if !ok {
		return "", false
	}
	candidate, ok := replaceLastIPv4Octet(targetIPv4, last)
	if !ok {
		return "", false
	}
	if candidate == strings.TrimSpace(targetIPv4) {
		return candidate, true
	}
	if !isIPv4AddressFree(targetPeers, candidate) {
		return "", false
	}
	return candidate, true
}

func normalizeExpiresAt(v any) any {
	switch x := v.(type) {
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return nil
		}
		return s
	case float64:
		if x <= 0 {
			return nil
		}
		return strconv.FormatInt(int64(x), 10)
	case int64:
		if x <= 0 {
			return nil
		}
		return strconv.FormatInt(x, 10)
	default:
		return v
	}
}

type migrationResult struct {
	SourceName       string
	TargetName       string
	IPPreserved      bool
	AssignedIPv4     string
	ExpiresPreserved bool
}

func migratePeerBetweenClients(sourceClient, targetClient *wgEasyClient, sourcePeerID string, sourceLabel, targetLabel string) (*migrationResult, error) {
	if err := precheckMigrationClients(sourceClient, targetClient, sourceLabel, targetLabel); err != nil {
		return nil, err
	}

	sourcePeer, err := sourceClient.getPeer(sourcePeerID)
	if err != nil {
		return nil, fmt.Errorf("не удалось получить исходный профиль: %w", err)
	}

	targetPeers, err := targetClient.listPeers()
	if err != nil {
		return nil, fmt.Errorf("не удалось получить список целевых профилей: %w", err)
	}

	if existing, ok := findPeerByExactName(targetPeers, sourcePeer.Name); ok {
		if err := targetClient.deleteClient(string(existing.ID)); err != nil {
			return nil, fmt.Errorf("не удалось удалить существующий целевой профиль %q: %w", sourcePeer.Name, err)
		}
		targetPeers, err = targetClient.listPeers()
		if err != nil {
			return nil, fmt.Errorf("не удалось обновить список целевых профилей: %w", err)
		}
	}

	if err := targetClient.createClient(sourcePeer.Name); err != nil {
		return nil, fmt.Errorf("не удалось создать целевой профиль: %w", err)
	}

	targetPeers, err = targetClient.listPeers()
	if err != nil {
		return nil, fmt.Errorf("не удалось получить список целевых профилей после создания: %w", err)
	}
	createdPeer, ok := findPeerByExactName(targetPeers, sourcePeer.Name)
	if !ok {
		return nil, fmt.Errorf("созданный целевой профиль %q не найден", sourcePeer.Name)
	}

	ipPreserved := false
	if desiredIPv4, ok := preserveIPv4HostPart(sourcePeer.IPv4Address, createdPeer.IPv4Address, targetPeers); ok {
		createdPeer.IPv4Address = desiredIPv4
		ipPreserved = true
	}

	createdPeer.Enabled = sourcePeer.Enabled
	createdPeer.ExpiresAt = normalizeExpiresAt(sourcePeer.ExpiresAt)

	if err := targetClient.updateClient(string(createdPeer.ID), &createdPeer); err != nil {
		return nil, fmt.Errorf("не удалось настроить целевой профиль: %w", err)
	}

	if err := sourceClient.deleteClient(sourcePeerID); err != nil {
		return nil, fmt.Errorf("целевой профиль создан, но исходный удалить не удалось: %w", err)
	}

	return &migrationResult{
		SourceName:       sourcePeer.Name,
		TargetName:       createdPeer.Name,
		IPPreserved:      ipPreserved,
		AssignedIPv4:     createdPeer.IPv4Address,
		ExpiresPreserved: sourcePeer.ExpiresAt != nil,
	}, nil
}
