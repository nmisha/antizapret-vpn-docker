package main

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
