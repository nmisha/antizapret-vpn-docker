package bot

import (
	"fmt"
	"hash/crc32"
	"strings"
	"unicode"
	"unicode/utf8"
)

// profileKind selects one of the per-user profile prefix lists in users.json.
type profileKind string

const (
	profileWg      profileKind = "wg" // wg_profiles, shared by WireGuard and Amnezia WireGuard
	profileOvpn    profileKind = "ovpn"
	profileMtproto profileKind = "mtproto"
)

var profileKinds = []profileKind{profileWg, profileOvpn, profileMtproto}

func parseProfileKind(s string) (profileKind, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "wg", "awg":
		return profileWg, nil
	case "ovpn", "openvpn":
		return profileOvpn, nil
	case "mtproto", "mtp", "telemt":
		return profileMtproto, nil
	}
	return "", fmt.Errorf("неизвестный тип профиля %q: нужно wg, ovpn или mtproto", s)
}

func (k profileKind) Label() string {
	switch k {
	case profileWg:
		return "WG/AWG"
	case profileOvpn:
		return "OpenVPN"
	case profileMtproto:
		return "MTProto"
	}
	return string(k)
}

// prefixes returns a pointer to the raw (persisted) prefix list of this kind.
func (k profileKind) prefixes(u *User) *[]string {
	switch k {
	case profileOvpn:
		return &u.OvpnProfilesRaw
	case profileMtproto:
		return &u.MtprotoProfilesRaw
	default:
		return &u.WgProfilesRaw
	}
}

func validateProfilePrefix(p string) error {
	if p == "" {
		return fmt.Errorf("пустой префикс")
	}
	if utf8.RuneCountInString(p) > 64 {
		return fmt.Errorf("префикс длиннее 64 символов")
	}
	for _, r := range p {
		if unicode.IsControl(r) {
			return fmt.Errorf("префикс содержит управляющие символы")
		}
	}
	return nil
}

// prefixToken identifies a prefix in callback data without embedding it (prefixes may be long or non-ASCII).
func prefixToken(p string) string {
	return fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte(strings.ToLower(p))))
}

// modifyByName loads users.json, applies fn to one user, normalizes and saves it.
func (us *UsersStore) modifyByName(name string, fn func(u *User) error) (User, error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	snap, err := us.loadUnlocked()
	if err != nil {
		return User{}, err
	}
	key := normalizeName(name)
	u, ok := snap.ByName[key]
	if !ok {
		return User{}, fmt.Errorf("user %q not found", name)
	}
	if err := fn(&u); err != nil {
		return User{}, err
	}
	nu, err := normalizeUser(u)
	if err != nil {
		return User{}, err
	}
	for i := range snap.List {
		if snap.List[i].Name == key {
			snap.List[i] = nu
			break
		}
	}
	if err := us.saveUnlocked(snap.List); err != nil {
		return User{}, err
	}
	return nu, nil
}

func (us *UsersStore) AddProfilePrefix(name string, kind profileKind, prefix string) (User, error) {
	prefix = strings.TrimSpace(prefix)
	if err := validateProfilePrefix(prefix); err != nil {
		return User{}, err
	}
	return us.modifyByName(name, func(u *User) error {
		list := kind.prefixes(u)
		for _, p := range *list {
			if strings.EqualFold(p, prefix) {
				return fmt.Errorf("префикс %q уже есть", prefix)
			}
		}
		*list = append(*list, prefix)
		return nil
	})
}

// RemoveProfilePrefix removes only an exact, case-insensitive prefix value.
func (us *UsersStore) RemoveProfilePrefix(name string, kind profileKind, prefix string) (User, string, error) {
	want := strings.TrimSpace(prefix)
	return us.removeProfilePrefix(name, kind, func(p string) bool { return strings.EqualFold(p, want) })
}

// RemoveProfilePrefixByToken resolves a callback token only when it matches one prefix.
// In particular, a CRC32 collision must not remove an arbitrary prefix.
func (us *UsersStore) RemoveProfilePrefixByToken(name string, kind profileKind, token string) (User, string, error) {
	return us.removeProfilePrefix(name, kind, func(p string) bool { return prefixToken(p) == token })
}

func (us *UsersStore) removeProfilePrefix(name string, kind profileKind, matches func(string) bool) (User, string, error) {
	var removed string
	nu, err := us.modifyByName(name, func(u *User) error {
		list := kind.prefixes(u)
		index := -1
		for i, p := range *list {
			if matches(p) {
				if index >= 0 {
					return fmt.Errorf("неоднозначный префикс: удалите его по значению через /prefix_del")
				}
				index = i
			}
		}
		if index < 0 {
			return fmt.Errorf("префикс не найден: откройте список заново")
		}
		removed = (*list)[index]
		*list = append((*list)[:index], (*list)[index+1:]...)
		return nil
	})
	return nu, removed, err
}

// SetDisabled turns bot access and notifications off or back on for a user.
func (us *UsersStore) SetDisabled(name string, disabled bool) (User, error) {
	return us.modifyByName(name, func(u *User) error {
		u.Disabled = disabled
		return nil
	})
}
