package bot

import (
	"fmt"
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

	desiredIPv4 := strings.TrimSpace(sourcePeer.IPv4Address)
	canPreserveIP := desiredIPv4 != "" && isIPv4AddressFree(targetPeers, desiredIPv4)

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
	if canPreserveIP {
		createdPeer.IPv4Address = desiredIPv4
		ipPreserved = true
	}

	createdPeer.Enabled = sourcePeer.Enabled
	createdPeer.ExpiresAt = sourcePeer.ExpiresAt

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
