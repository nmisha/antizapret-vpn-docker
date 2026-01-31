package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
)

type UsersStore struct {
	Path string
	mu   sync.Mutex
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
