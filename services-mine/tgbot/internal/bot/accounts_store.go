package bot

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

type Account struct {
	Name     string `json:"name"`
	Login    string `json:"login"`
	Password string `json:"password"`
}

type AccountsStore struct {
	Path string
	mu   sync.Mutex
}

func NewAccountsStore(path string) *AccountsStore { return &AccountsStore{Path: path} }

func (as *AccountsStore) loadUnlocked() ([]Account, error) {
	b, err := os.ReadFile(as.Path)
	if err != nil {
		return nil, err
	}
	var list []Account
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, err
	}

	seen := map[string]struct{}{}
	out := make([]Account, 0, len(list))

	for _, a := range list {
		a.Name = strings.TrimSpace(a.Name)
		a.Login = strings.TrimSpace(a.Login)
		// password триммить не будем агрессивно — вдруг там пробелы важны (редко, но бывает)
		if a.Name == "" {
			return nil, fmt.Errorf("accounts file: empty name")
		}
		key := strings.ToLower(a.Name)
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("accounts file: duplicate name %q", a.Name)
		}
		seen[key] = struct{}{}
		out = append(out, a)
	}

	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})

	return out, nil
}

func (as *AccountsStore) List() ([]Account, error) {
	as.mu.Lock()
	defer as.mu.Unlock()
	return as.loadUnlocked()
}

func (as *AccountsStore) saveUnlocked(list []Account) error {
	// keep sorted
	sort.Slice(list, func(i, j int) bool {
		return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
	})
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := as.Path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, as.Path)
}

// Upsert creates or updates account by name (case-insensitive).
func (as *AccountsStore) Upsert(a Account) (string, error) {
	as.mu.Lock()
	defer as.mu.Unlock()

	list, err := as.loadUnlocked()
	if err != nil {
		return "", err
	}

	a.Name = strings.TrimSpace(a.Name)
	a.Login = strings.TrimSpace(a.Login)
	if a.Name == "" {
		return "", fmt.Errorf("account name required")
	}
	key := strings.ToLower(a.Name)

	for i := range list {
		if strings.ToLower(list[i].Name) == key {
			list[i].Name = a.Name
			list[i].Login = a.Login
			list[i].Password = a.Password
			if err := as.saveUnlocked(list); err != nil {
				return "", err
			}
			return "Обновлена учётная запись: " + a.Name, nil
		}
	}

	list = append(list, a)
	if err := as.saveUnlocked(list); err != nil {
		return "", err
	}
	return "Добавлена учётная запись: " + a.Name, nil
}

func (as *AccountsStore) DeleteByName(name string) (string, error) {
	as.mu.Lock()
	defer as.mu.Unlock()

	list, err := as.loadUnlocked()
	if err != nil {
		return "", err
	}
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return "", fmt.Errorf("name required")
	}

	out := make([]Account, 0, len(list))
	deleted := false
	for _, a := range list {
		if strings.ToLower(a.Name) == key {
			deleted = true
			continue
		}
		out = append(out, a)
	}
	if !deleted {
		return "", fmt.Errorf("account not found: %s", name)
	}
	if err := as.saveUnlocked(out); err != nil {
		return "", err
	}
	return "Удалена учётная запись: " + name, nil
}

func (as *AccountsStore) Rename(oldName, newName string) (string, error) {
	as.mu.Lock()
	defer as.mu.Unlock()

	oldKey := strings.ToLower(strings.TrimSpace(oldName))
	newKey := strings.ToLower(strings.TrimSpace(newName))
	if oldKey == "" || newKey == "" {
		return "", fmt.Errorf("old and new name required")
	}
	if oldKey == newKey {
		return "", fmt.Errorf("new name equals old name")
	}

	list, err := as.loadUnlocked()
	if err != nil {
		return "", err
	}

	// check new unique
	for _, a := range list {
		if strings.ToLower(a.Name) == newKey {
			return "", fmt.Errorf("account name already exists: %s", newName)
		}
	}

	found := false
	for i := range list {
		if strings.ToLower(list[i].Name) == oldKey {
			list[i].Name = strings.TrimSpace(newName)
			found = true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("account not found: %s", oldName)
	}

	if err := as.saveUnlocked(list); err != nil {
		return "", err
	}
	return fmt.Sprintf("Переименовано: %s → %s", oldName, newName), nil
}
