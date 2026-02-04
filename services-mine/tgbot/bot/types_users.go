package main

import (
	"fmt"
	"sort"
	"strings"
)

type Role string

const (
	RoleDomainEditor   Role = "DomainEditor"
	RoleDomainManager  Role = "DomainManager"
	RoleServiceManager Role = "ServiceManager" // renamed from Manager
	RoleInfo           Role = "Info"
	RoleAdmin          Role = "Admin"
	RoleAiUser         Role = "AiUser"
)

type User struct {
	TelegramID    int64         `json:"telegram_id"`
	Name          string        `json:"name"` // UNIQUE (case-insensitive -> stored normalized)
	WgProfilesRaw []string      `json:"wg_profiles,omitempty"`
	RolesRaw      []string      `json:"roles"` // persisted (canonical)
	Roles         map[Role]bool `json:"-"`     // runtime
	WgProfiles    []string      `json:"-"`     // normalized (lower-case) wg profile prefixes/names
}

// Admin включает возможности всех ролей
func (u User) Has(role Role) bool {
	if u.Roles[RoleAdmin] {
		return true
	}
	return u.Roles[role]
}

func normalizeName(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func normalizeRoleString(s string) (Role, error) {
	r := strings.ToLower(strings.TrimSpace(s))
	switch r {
	case strings.ToLower(string(RoleDomainEditor)):
		return RoleDomainEditor, nil
	case strings.ToLower(string(RoleDomainManager)):
		return RoleDomainManager, nil
	case "manager": // backward compatibility
		return RoleServiceManager, nil
	case strings.ToLower(string(RoleServiceManager)):
		return RoleServiceManager, nil
	case strings.ToLower(string(RoleInfo)):
		return RoleInfo, nil
	case strings.ToLower(string(RoleAdmin)):
		return RoleAdmin, nil
	case strings.ToLower(string(RoleAiUser)):
		return RoleAiUser, nil
	default:
		return "", fmt.Errorf("unknown role: %q", s)
	}
}

func uniqueStringsCaseInsensitive(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		key := strings.ToLower(s)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func normalizeUser(u User) (User, error) {
	u.Name = normalizeName(u.Name)
	if u.TelegramID == 0 || u.Name == "" {
		return User{}, fmt.Errorf("invalid user: telegram_id and name required")
	}

	roleSeen := map[Role]struct{}{}
	canon := make([]string, 0, len(u.RolesRaw))
	u.Roles = map[Role]bool{}

	for _, rr := range u.RolesRaw {
		role, err := normalizeRoleString(rr)
		if err != nil {
			return User{}, err
		}
		if _, ok := roleSeen[role]; ok {
			continue
		}
		roleSeen[role] = struct{}{}
		u.Roles[role] = true
		canon = append(canon, string(role))
	}

	canon = uniqueStringsCaseInsensitive(canon)
	u.RolesRaw = canon

	// WireGuard profile prefixes/names (case-insensitive)
	u.WgProfilesRaw = uniqueStringsCaseInsensitive(u.WgProfilesRaw)
	u.WgProfiles = make([]string, 0, len(u.WgProfilesRaw))
	for _, p := range u.WgProfilesRaw {
		pp := strings.ToLower(strings.TrimSpace(p))
		if pp == "" {
			continue
		}
		u.WgProfiles = append(u.WgProfiles, pp)
	}
	sort.Strings(u.WgProfiles)

	return u, nil
}
