package bot

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

type Store struct {
	Path string
	mu   sync.Mutex
}

func NewStore(path string) *Store { return &Store{Path: path} }

func (s *Store) Load() (map[string][]string, error) {
	sections := map[string][]string{}

	f, err := os.Open(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return sections, nil
		}
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	var current string
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			name := strings.TrimSpace(strings.TrimPrefix(line, "#"))
			current = name
			if _, ok := sections[current]; !ok {
				sections[current] = []string{}
			}
			continue
		}
		if current == "" {
			continue
		}
		sections[current] = append(sections[current], line)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	for k, v := range sections {
		sections[k] = normalizeList(v)
	}
	return sections, nil
}

func (s *Store) Save(sections map[string][]string) error {
	tmp := s.Path + ".tmp"

	keys := make([]string, 0, len(sections))
	for k := range sections {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)

	for i, k := range keys {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "# %s\n", k)
		list := normalizeList(sections[k])
		for _, d := range list {
			fmt.Fprintln(w, d)
		}
	}

	if err := w.Flush(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}

func (s *Store) AddDomain(sectionName, domain string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sections, err := s.Load()
	if err != nil {
		return false, err
	}
	list := sections[sectionName]
	for _, d := range list {
		if d == domain {
			return false, nil
		}
	}
	sections[sectionName] = append(list, domain)
	sections[sectionName] = normalizeList(sections[sectionName])
	return true, s.Save(sections)
}

func (s *Store) DelDomain(sectionName, domain string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sections, err := s.Load()
	if err != nil {
		return false, err
	}
	list, ok := sections[sectionName]
	if !ok {
		return false, nil
	}
	out := make([]string, 0, len(list))
	removed := false
	for _, d := range list {
		if d == domain {
			removed = true
			continue
		}
		out = append(out, d)
	}
	if !removed {
		return false, nil
	}
	sections[sectionName] = normalizeList(out)
	return true, s.Save(sections)
}

func (s *Store) DelDomainAny(domain string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sections, err := s.Load()
	if err != nil {
		return nil, err
	}

	affected := make([]string, 0, 1)
	changed := false

	for sec, list := range sections {
		out := make([]string, 0, len(list))
		removedHere := false
		for _, d := range list {
			if d == domain {
				removedHere = true
				changed = true
				continue
			}
			out = append(out, d)
		}
		if removedHere {
			affected = append(affected, sec)
			sections[sec] = normalizeList(out)
		}
	}

	if !changed {
		return nil, nil
	}
	sort.Strings(affected)
	return affected, s.Save(sections)
}

func (s *Store) List(sectionName string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sections, err := s.Load()
	if err != nil {
		return nil, err
	}
	return normalizeList(sections[sectionName]), nil
}

func (s *Store) ExportAll() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sections, err := s.Load()
	if err != nil {
		return "", err
	}
	keys := make([]string, 0, len(sections))
	for k := range sections {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "# %s\n", k)
		for _, d := range normalizeList(sections[k]) {
			b.WriteString(d)
			b.WriteByte('\n')
		}
	}
	return b.String(), nil
}

type ContainmentMatch struct {
	ParentDomain  string
	ParentSection string
}

func (s *Store) FindLongestContainingDomain(candidate string) (*ContainmentMatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sections, err := s.Load()
	if err != nil {
		return nil, err
	}

	var best *ContainmentMatch
	for sec, list := range sections {
		for _, parent := range list {
			if parent == candidate {
				continue
			}
			if isSubdomainOf(candidate, parent) {
				if best == nil || len(parent) > len(best.ParentDomain) {
					best = &ContainmentMatch{ParentDomain: parent, ParentSection: sec}
				}
			}
		}
	}
	return best, nil
}

func (s *Store) ReplaceDomain(parentSection, parentDomain, targetSection, candidate string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sections, err := s.Load()
	if err != nil {
		return err
	}

	list := sections[parentSection]
	newList := make([]string, 0, len(list))
	for _, d := range list {
		if d == parentDomain {
			continue
		}
		newList = append(newList, d)
	}
	sections[parentSection] = normalizeList(newList)

	sections[targetSection] = append(sections[targetSection], candidate)
	sections[targetSection] = normalizeList(sections[targetSection])

	return s.Save(sections)
}

func (s *Store) RenameSection(oldName, newName string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	if oldName == "" || newName == "" {
		return "", fmt.Errorf("old_name and new_name required")
	}
	if oldName == newName {
		return "Секция не изменена (имена совпадают).", nil
	}

	sections, err := s.Load()
	if err != nil {
		return "", err
	}

	oldList, ok := sections[oldName]
	if !ok {
		return fmt.Sprintf("Секция #%s не найдена — нечего переименовывать.", oldName), nil
	}

	if newList, exists := sections[newName]; exists {
		merged := append(newList, oldList...)
		sections[newName] = normalizeList(merged)
	} else {
		sections[newName] = normalizeList(oldList)
	}

	delete(sections, oldName)

	if err := s.Save(sections); err != nil {
		return "", err
	}

	return fmt.Sprintf("Секция доменов переименована #%s → #%s", oldName, newName), nil
}
