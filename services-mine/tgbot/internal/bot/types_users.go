package bot

import (
	"fmt"
	"sort"
	"strings"
)

type Role string

const (
	RoleDomainEditor    Role = "DomainEditor"
	RoleDomainManager   Role = "DomainManager"
	RoleServiceManager  Role = "ServiceManager" // renamed from Manager
	RoleInfo            Role = "Info"
	RoleWgStats         Role = "WgStats"
	RoleAwgStats        Role = "AwgStats"
	RoleOvpnStats       Role = "OvpnStats"
	RoleWgUserControl   Role = "WgUserControl"
	RoleAwgUserControl  Role = "AwgUserControl"
	RoleOvpnUserControl Role = "OvpnUserControl"
	RoleSupport         Role = "Support"
	RoleAdmin           Role = "Admin"
	RoleAiUser          Role = "AiUser"
	RoleNetUser         Role = "NetUser"
)

type User struct {
	TelegramID      int64         `json:"telegram_id"`
	Name            string        `json:"name"` // UNIQUE (case-insensitive -> stored normalized)
	WgProfilesRaw   []string      `json:"wg_profiles,omitempty"`
	OvpnProfilesRaw []string      `json:"ovpn_profiles,omitempty"`
	RolesRaw        []string      `json:"roles"` // persisted canonical role names
	Roles           map[Role]bool `json:"-"`     // runtime
	WgProfiles      []string      `json:"-"`     // normalized (lower-case) wg profile prefixes/names
	OvpnProfiles    []string      `json:"-"`     // normalized (lower-case) ovpn profile prefixes/names
}

// Admin includes permissions of all other roles.
func (u User) Has(role Role) bool {
	if u.Roles[RoleAdmin] {
		return true
	}
	return u.Roles[role]
}

// HasExact checks whether the role is explicitly assigned without Admin override.
func (u User) HasExact(role Role) bool {
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
	case "manager":
		return RoleServiceManager, nil
	case strings.ToLower(string(RoleServiceManager)):
		return RoleServiceManager, nil
	case strings.ToLower(string(RoleInfo)):
		return RoleInfo, nil
	case strings.ToLower(string(RoleWgStats)):
		return RoleWgStats, nil
	case strings.ToLower(string(RoleAwgStats)):
		return RoleAwgStats, nil
	case strings.ToLower(string(RoleOvpnStats)):
		return RoleOvpnStats, nil
	case strings.ToLower(string(RoleWgUserControl)):
		return RoleWgUserControl, nil
	case strings.ToLower(string(RoleAwgUserControl)):
		return RoleAwgUserControl, nil
	case strings.ToLower(string(RoleOvpnUserControl)):
		return RoleOvpnUserControl, nil
	case strings.ToLower(string(RoleSupport)):
		return RoleSupport, nil
	case strings.ToLower(string(RoleAdmin)):
		return RoleAdmin, nil
	case strings.ToLower(string(RoleAiUser)):
		return RoleAiUser, nil
	case strings.ToLower(string(RoleNetUser)):
		return RoleNetUser, nil
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

	u.OvpnProfilesRaw = uniqueStringsCaseInsensitive(u.OvpnProfilesRaw)
	u.OvpnProfiles = make([]string, 0, len(u.OvpnProfilesRaw))
	for _, p := range u.OvpnProfilesRaw {
		pp := strings.ToLower(strings.TrimSpace(p))
		if pp == "" {
			continue
		}
		u.OvpnProfiles = append(u.OvpnProfiles, pp)
	}
	sort.Strings(u.OvpnProfiles)

	return u, nil
}
