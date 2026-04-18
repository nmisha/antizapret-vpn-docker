package bot

import (
	"fmt"
	"strings"
)

func findWgPeerByID(peers []wgEasyPeer, peerID string) (wgEasyPeer, bool) {
	for _, p := range peers {
		if string(p.ID) == peerID {
			return p, true
		}
	}
	return wgEasyPeer{}, false
}

func canAccessWgPeerByPrefixes(peer wgEasyPeer, prefixes []string) bool {
	name := strings.ToLower(strings.TrimSpace(peer.Name))
	if name == "" || len(prefixes) == 0 {
		return false
	}
	for _, prefix := range prefixes {
		pp := strings.ToLower(strings.TrimSpace(prefix))
		if pp != "" && strings.HasPrefix(name, pp) {
			return true
		}
	}
	return false
}

func validateWgUserPeerAccess(ctx *Ctx, client *wgEasyClient, peerID string) (wgEasyPeer, error) {
	if !ctx.User.Has(RoleWgUserControl) && !ctx.User.Has(RoleAdmin) {
		return wgEasyPeer{}, fmt.Errorf("недостаточно прав")
	}

	peers, err := client.listPeers()
	if err != nil {
		return wgEasyPeer{}, err
	}
	peer, ok := findWgPeerByID(peers, peerID)
	if !ok {
		return wgEasyPeer{}, fmt.Errorf("профиль больше не найден")
	}
	if !canAccessWgPeerByPrefixes(peer, ctx.User.WgProfiles) && !ctx.User.Has(RoleAdmin) {
		return wgEasyPeer{}, fmt.Errorf("доступ к профилю отозван")
	}
	return peer, nil
}

func validateWgAdminPeerAccess(ctx *Ctx, _ *wgEasyClient, peerID string) (wgEasyPeer, error) {
	if !ctx.User.Has(RoleAdmin) {
		return wgEasyPeer{}, fmt.Errorf("недостаточно прав")
	}

	scope, ok := getAdminScope(ctx.TgID)
	if !ok || scope.Mode == "" {
		return wgEasyPeer{}, fmt.Errorf("кнопка устарела, открой список профилей заново")
	}

	peers, err := getPeersForScope(ctx, scope)
	if err != nil {
		return wgEasyPeer{}, err
	}
	peer, ok := findWgPeerByID(peers, peerID)
	if !ok {
		return wgEasyPeer{}, fmt.Errorf("профиль больше не доступен в текущем scope")
	}
	return peer, nil
}

func validateWgAdminAddAccess(ctx *Ctx) error {
	if !ctx.User.Has(RoleAdmin) {
		return fmt.Errorf("недостаточно прав")
	}
	scope, ok := getAdminScope(ctx.TgID)
	if !ok || scope.Mode == "" {
		return fmt.Errorf("кнопка устарела, открой список профилей заново")
	}
	return nil
}

func validateAwgUserPeerAccess(ctx *Ctx, client *wgEasyClient, peerID string) (wgEasyPeer, error) {
	if !ctx.User.Has(RoleAwgUserControl) && !ctx.User.Has(RoleAdmin) {
		return wgEasyPeer{}, fmt.Errorf("недостаточно прав")
	}
	peers, err := client.listPeers()
	if err != nil {
		return wgEasyPeer{}, err
	}
	peer, ok := findWgPeerByID(peers, peerID)
	if !ok {
		return wgEasyPeer{}, fmt.Errorf("профиль больше не найден")
	}
	if !canAccessWgPeerByPrefixes(peer, ctx.User.WgProfiles) && !ctx.User.Has(RoleAdmin) {
		return wgEasyPeer{}, fmt.Errorf("доступ к профилю отозван")
	}
	return peer, nil
}

func validateAwgAdminPeerAccess(ctx *Ctx, peerID string) (wgEasyPeer, error) {
	if !ctx.User.Has(RoleAdmin) {
		return wgEasyPeer{}, fmt.Errorf("недостаточно прав")
	}
	scope, ok := getAwgAdminScope(ctx.TgID)
	if !ok || scope.Mode == "" {
		return wgEasyPeer{}, fmt.Errorf("кнопка устарела, открой список профилей заново")
	}
	peers, err := getAwgPeersForScope(ctx, scope)
	if err != nil {
		return wgEasyPeer{}, err
	}
	peer, ok := findWgPeerByID(peers, peerID)
	if !ok {
		return wgEasyPeer{}, fmt.Errorf("профиль больше не доступен в текущем scope")
	}
	return peer, nil
}

func validateAwgAdminAddAccess(ctx *Ctx) error {
	if !ctx.User.Has(RoleAdmin) {
		return fmt.Errorf("недостаточно прав")
	}
	scope, ok := getAwgAdminScope(ctx.TgID)
	if !ok || scope.Mode == "" {
		return fmt.Errorf("кнопка устарела, открой список профилей заново")
	}
	return nil
}

func canAccessOvpnProfileByPrefixes(profileName string, prefixes []string) bool {
	name := strings.ToLower(strings.TrimSpace(profileName))
	if name == "" || len(prefixes) == 0 {
		return false
	}
	for _, prefix := range prefixes {
		pp := strings.ToLower(strings.TrimSpace(prefix))
		if pp != "" && strings.HasPrefix(name, pp) {
			return true
		}
	}
	return false
}

func validateOvpnUserProfileAccess(ctx *Ctx, client *ovpnUIClient, profileName string) (ovpnProfile, error) {
	if !ctx.User.Has(RoleOvpnUserControl) && !ctx.User.Has(RoleAdmin) {
		return ovpnProfile{}, fmt.Errorf("недостаточно прав")
	}

	profiles, err := client.listProfiles()
	if err != nil {
		return ovpnProfile{}, err
	}
	want := strings.ToLower(strings.TrimSpace(profileName))
	for _, p := range profiles {
		if strings.ToLower(strings.TrimSpace(p.Name)) != want {
			continue
		}
		if !canAccessOvpnProfileByPrefixes(p.Name, ctx.User.OvpnProfiles) && !ctx.User.Has(RoleAdmin) {
			return ovpnProfile{}, fmt.Errorf("доступ к профилю отозван")
		}
		return p, nil
	}
	return ovpnProfile{}, fmt.Errorf("профиль больше не найден")
}

func validateOvpnAdminProfileAccess(ctx *Ctx, _ *ovpnUIClient, profileName string) (ovpnProfile, error) {
	if !ctx.User.Has(RoleAdmin) {
		return ovpnProfile{}, fmt.Errorf("недостаточно прав")
	}

	scope, ok := getOvpnAdminScope(ctx.TgID)
	if !ok || scope.Mode == "" {
		return ovpnProfile{}, fmt.Errorf("кнопка устарела, открой список профилей заново")
	}

	profiles, err := getOvpnProfilesForScope(ctx, scope)
	if err != nil {
		return ovpnProfile{}, err
	}
	want := strings.ToLower(strings.TrimSpace(profileName))
	for _, p := range profiles {
		if strings.ToLower(strings.TrimSpace(p.Name)) == want {
			return p, nil
		}
	}
	return ovpnProfile{}, fmt.Errorf("профиль больше не доступен в текущем scope")
}

func validateOvpnAdminAddAccess(ctx *Ctx) error {
	if !ctx.User.Has(RoleAdmin) {
		return fmt.Errorf("недостаточно прав")
	}
	scope, ok := getOvpnAdminScope(ctx.TgID)
	if !ok || scope.Mode == "" {
		return fmt.Errorf("кнопка устарела, открой список профилей заново")
	}
	return nil
}
