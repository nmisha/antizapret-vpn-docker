package bot

import (
	"errors"
	"regexp"
	"sort"
	"strings"
)

var reDomain = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)

func normalizeDomain(input string) (string, error) {
	s := strings.TrimSpace(input)
	s = strings.ToLower(s)

	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "https://")

	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}

	if i := strings.IndexByte(s, ':'); i >= 0 {
		s = s[:i]
	}

	s = strings.TrimPrefix(s, ".")
	s = strings.TrimSuffix(s, ".")

	if s == "" {
		return "", errors.New("пустой домен")
	}
	if len(s) > 253 {
		return "", errors.New("слишком длинный домен")
	}
	if !reDomain.MatchString(s) {
		return "", errors.New("не похоже на домен (пример: example.com)")
	}
	return s, nil
}

func normalizeList(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func isSubdomainOf(sub, parent string) bool {
	if len(sub) <= len(parent) {
		return false
	}
	if !strings.HasSuffix(sub, parent) {
		return false
	}
	idx := len(sub) - len(parent) - 1
	return idx >= 0 && sub[idx] == '.'
}
