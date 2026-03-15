package bot

import (
	"strings"
)

const (
	wgConfigFilenameExt      = ".conf"
	wgConfigFilenameMaxTotal = 15
)

func normalizeWgConfigFilename(raw string) string {
	base := strings.TrimSpace(raw)
	if strings.HasSuffix(strings.ToLower(base), wgConfigFilenameExt) {
		base = base[:len(base)-len(wgConfigFilenameExt)]
	}

	maxBaseLen := wgConfigFilenameMaxTotal - len(wgConfigFilenameExt)
	if maxBaseLen < 1 {
		return "wg" + wgConfigFilenameExt
	}

	segments := splitWgFilenameSegments(base)
	if len(segments) == 1 {
		normalized := buildWgFilenameFromSingleSegment(filterWgConfigFilenameChars(segments[0]), maxBaseLen)
		if normalized == "" {
			return "wg" + wgConfigFilenameExt
		}
		return normalized + wgConfigFilenameExt
	}
	if len(segments) == 2 {
		return "wg" + wgConfigFilenameExt
	}

	segments = segments[1:]
	if len(segments) < 2 {
		return "wg" + wgConfigFilenameExt
	}

	first := filterWgConfigFilenameChars(segments[len(segments)-2])
	second := filterWgConfigFilenameChars(segments[len(segments)-1])
	normalized := buildWgFilenameFromLastSegments(first, second, maxBaseLen)
	if normalized == "" {
		return "wg" + wgConfigFilenameExt
	}
	if len(normalized) > maxBaseLen {
		normalized = normalized[:maxBaseLen]
	}
	return normalized + wgConfigFilenameExt
}

func filterWgConfigFilenameChars(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-':
			b.WriteRune(r)
		}
	}
	return b.String()
}

func splitWgFilenameSegments(s string) []string {
	parts := strings.Split(s, "_")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func buildWgFilenameFromLastSegments(first, second string, maxLen int) string {
	if first == "" && second == "" {
		return ""
	}

	firstTake := min(4, len(first))
	secondPrefixTake := min(4, len(second))
	secondSuffixTake := min(2, max(0, len(second)-secondPrefixTake))

	target := min(maxLen, 10)
	current := firstTake + secondPrefixTake + secondSuffixTake
	if current < target {
		need := target - current
		add := min(need, max(0, len(first)-firstTake))
		firstTake += add
		current += add
	}
	if current < target {
		need := target - current
		add := min(need, max(0, len(second)-secondPrefixTake-secondSuffixTake))
		secondPrefixTake += add
		current += add
	}

	firstPart := first[:firstTake]
	secondPrefix := second[:secondPrefixTake]
	secondSuffix := ""
	if secondSuffixTake > 0 {
		secondSuffix = second[len(second)-secondSuffixTake:]
	}
	return firstPart + secondPrefix + secondSuffix
}

func buildWgFilenameFromSingleSegment(segment string, maxLen int) string {
	if segment == "" {
		return ""
	}
	if len(segment) <= maxLen {
		return segment
	}

	prefixTake := min(8, len(segment))
	suffixTake := min(2, max(0, len(segment)-prefixTake))
	current := prefixTake + suffixTake
	if current < maxLen {
		prefixTake += min(maxLen-current, len(segment)-prefixTake-suffixTake)
	}

	result := segment[:prefixTake]
	if suffixTake > 0 {
		result += segment[len(segment)-suffixTake:]
	}
	if len(result) > maxLen {
		result = result[:maxLen]
	}
	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// WG клиент проверяет по регулярке:
// ^[a-zA-Z0-9_=+.-]{1,15}$
