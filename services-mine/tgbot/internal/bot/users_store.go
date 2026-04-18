package bot

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

type UsersStore struct {
	Path string
	mu   sync.Mutex
}

func (us *UsersStore) SetGuardNotifyByID(tgID int64, enabled bool) (User, error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	snap, err := us.loadUnlocked()
	if err != nil {
		return User{}, err
	}

	u, ok := snap.ByID[tgID]
	if !ok {
		return User{}, fmt.Errorf("user with telegram_id %d not found", tgID)
	}

	u.GuardNotifyEnabled = &enabled
	nu, err := normalizeUser(u)
	if err != nil {
		return User{}, err
	}

	for i := range snap.List {
		if snap.List[i].TelegramID == tgID {
			snap.List[i] = nu
			break
		}
	}

	if err := us.saveUnlocked(snap.List); err != nil {
		return User{}, err
	}
	return nu, nil
}

func NewUsersStore(path string) *UsersStore { return &UsersStore{Path: path} }

type usersSnapshot struct {
	ByID   map[int64]User
	ByName map[string]User // key: normalized lower-case name
	List   []User
}

func (us *UsersStore) loadUnlocked() (*usersSnapshot, error) {
	b, err := os.ReadFile(us.Path)
	if err != nil {
		return nil, err
	}
	var list []User
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, err
	}

	byID := make(map[int64]User, len(list))
	byName := make(map[string]User, len(list))
	outList := make([]User, 0, len(list))

	for _, u := range list {
		nu, err := normalizeUser(u)
		if err != nil {
			return nil, err
		}
		if _, exists := byID[nu.TelegramID]; exists {
			return nil, fmt.Errorf("users.json: duplicate telegram_id %d", nu.TelegramID)
		}
		if _, exists := byName[nu.Name]; exists {
			return nil, fmt.Errorf("users.json: duplicate name %q", nu.Name)
		}

		byID[nu.TelegramID] = nu
		byName[nu.Name] = nu
		outList = append(outList, nu)
	}

	sort.Slice(outList, func(i, j int) bool { return outList[i].Name < outList[j].Name })
	return &usersSnapshot{ByID: byID, ByName: byName, List: outList}, nil
}

func (us *UsersStore) saveUnlocked(list []User) error {
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := us.Path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, us.Path)
}

func (us *UsersStore) GetByID(tgID int64) (User, bool, error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	snap, err := us.loadUnlocked()
	if err != nil {
		return User{}, false, err
	}
	u, ok := snap.ByID[tgID]
	return u, ok, nil
}

func (us *UsersStore) GetByName(name string) (User, bool, error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	snap, err := us.loadUnlocked()
	if err != nil {
		return User{}, false, err
	}
	key := normalizeName(name)
	u, ok := snap.ByName[key]
	return u, ok, nil
}

func (us *UsersStore) ListUsers() ([]User, error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	snap, err := us.loadUnlocked()
	if err != nil {
		return nil, err
	}
	return snap.List, nil
}

func (us *UsersStore) FindByWGProfile(profileName string) (User, bool, error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	snap, err := us.loadUnlocked()
	if err != nil {
		return User{}, false, err
	}
	want := strings.ToLower(strings.TrimSpace(profileName))
	if want == "" {
		return User{}, false, nil
	}
	for _, u := range snap.List {
		for _, prefix := range u.WgProfiles {
			if prefix != "" && strings.HasPrefix(want, prefix) {
				return u, true, nil
			}
		}
	}
	return User{}, false, nil
}

func (us *UsersStore) FindByOvpnProfile(profileName string) (User, bool, error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	snap, err := us.loadUnlocked()
	if err != nil {
		return User{}, false, err
	}
	want := strings.ToLower(strings.TrimSpace(profileName))
	if want == "" {
		return User{}, false, nil
	}
	for _, u := range snap.List {
		for _, prefix := range u.OvpnProfiles {
			if prefix != "" && strings.HasPrefix(want, prefix) {
				return u, true, nil
			}
		}
	}
	return User{}, false, nil
}

func (us *UsersStore) ListAdmins() ([]User, error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	snap, err := us.loadUnlocked()
	if err != nil {
		return nil, err
	}
	out := make([]User, 0, len(snap.List))
	for _, u := range snap.List {
		if u.HasExact(RoleAdmin) {
			out = append(out, u)
		}
	}
	return out, nil
}

func (us *UsersStore) GrantByName(name string, role Role) (string, error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	nRole, err := normalizeRoleString(string(role))
	if err != nil {
		return "", err
	}

	snap, err := us.loadUnlocked()
	if err != nil {
		return "", err
	}

	key := normalizeName(name)
	u, ok := snap.ByName[key]
	if !ok {
		return "", fmt.Errorf("user %q not found", name)
	}

	u.RolesRaw = append(u.RolesRaw, string(nRole))
	nu, err := normalizeUser(u)
	if err != nil {
		return "", err
	}

	for i := range snap.List {
		if snap.List[i].Name == key {
			snap.List[i] = nu
			break
		}
	}

	if err := us.saveUnlocked(snap.List); err != nil {
		return "", err
	}
	return fmt.Sprintf("Выдана роль %s пользователю %s (tg_id=%d)", nRole, nu.Name, nu.TelegramID), nil
}

func (us *UsersStore) RevokeByName(name string, role Role) (string, error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	nRole, err := normalizeRoleString(string(role))
	if err != nil {
		return "", err
	}

	snap, err := us.loadUnlocked()
	if err != nil {
		return "", err
	}

	key := normalizeName(name)
	u, ok := snap.ByName[key]
	if !ok {
		return "", fmt.Errorf("user %q not found", name)
	}

	out := make([]string, 0, len(u.RolesRaw))
	for _, r := range u.RolesRaw {
		rr, err := normalizeRoleString(r)
		if err != nil {
			return "", err
		}
		if rr == nRole {
			continue
		}
		out = append(out, string(rr))
	}
	u.RolesRaw = out

	nu, err := normalizeUser(u)
	if err != nil {
		return "", err
	}

	for i := range snap.List {
		if snap.List[i].Name == key {
			snap.List[i] = nu
			break
		}
	}
	if err := us.saveUnlocked(snap.List); err != nil {
		return "", err
	}
	return fmt.Sprintf("Снята роль %s у пользователя %s (tg_id=%d)", nRole, nu.Name, nu.TelegramID), nil
}

func (us *UsersStore) Rename(oldName, newName string) (string, error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	oldKey := normalizeName(oldName)
	newKey := normalizeName(newName)

	if oldKey == "" || newKey == "" {
		return "", fmt.Errorf("old_name and new_name required")
	}
	if oldKey == newKey {
		return "", fmt.Errorf("new_name equals old_name")
	}

	snap, err := us.loadUnlocked()
	if err != nil {
		return "", err
	}

	u, ok := snap.ByName[oldKey]
	if !ok {
		return "", fmt.Errorf("user %q not found", oldName)
	}
	if _, exists := snap.ByName[newKey]; exists {
		return "", fmt.Errorf("name %q already exists", newName)
	}

	u.Name = newKey
	nu, err := normalizeUser(u)
	if err != nil {
		return "", err
	}

	for i := range snap.List {
		if snap.List[i].Name == oldKey {
			snap.List[i] = nu
			break
		}
	}

	if err := us.saveUnlocked(snap.List); err != nil {
		return "", err
	}

	return fmt.Sprintf("Переименован пользователь %q → %q (tg_id=%d)", oldName, newName, nu.TelegramID), nil
}

// Add creates a new user with given name and telegram id.
// Roles are empty by default.
func (us *UsersStore) Add(name string, tgID int64) (string, error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	key := normalizeName(name)
	if key == "" {
		return "", fmt.Errorf("name required")
	}
	if tgID <= 0 {
		return "", fmt.Errorf("telegram_id required")
	}

	snap, err := us.loadUnlocked()
	if err != nil {
		return "", err
	}
	if _, exists := snap.ByName[key]; exists {
		return "", fmt.Errorf("name %q already exists", name)
	}
	if _, exists := snap.ByID[tgID]; exists {
		return "", fmt.Errorf("telegram_id %d already exists", tgID)
	}

	u := User{Name: key, TelegramID: tgID, RolesRaw: []string{}}
	nu, err := normalizeUser(u)
	if err != nil {
		return "", err
	}

	list := append(snap.List, nu)
	if err := us.saveUnlocked(list); err != nil {
		return "", err
	}
	return fmt.Sprintf("Добавлен пользователь %q (tg_id=%d)", nu.Name, nu.TelegramID), nil
}

// DeleteByName removes a user by name.
func (us *UsersStore) DeleteByName(name string) (string, error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	key := normalizeName(name)
	if key == "" {
		return "", fmt.Errorf("name required")
	}

	snap, err := us.loadUnlocked()
	if err != nil {
		return "", err
	}
	if _, exists := snap.ByName[key]; !exists {
		return "", fmt.Errorf("user %q not found", name)
	}

	out := make([]User, 0, len(snap.List))
	var tgID int64
	for _, u := range snap.List {
		if u.Name == key {
			tgID = u.TelegramID
			continue
		}
		out = append(out, u)
	}
	if err := us.saveUnlocked(out); err != nil {
		return "", err
	}
	return fmt.Sprintf("Удалён пользователь %q (tg_id=%d)", key, tgID), nil
}
