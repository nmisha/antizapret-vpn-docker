package bot

import "strings"

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

	candidate := base
	for {
		segmentAware := filterWgConfigFilenameSegmentChars(candidate)
		if segmentAware == "" {
			return "wg" + wgConfigFilenameExt
		}

		if len(segmentAware) <= maxBaseLen {
			normalized := filterWgConfigFilenameChars(candidate)
			if normalized == "" {
				return "wg" + wgConfigFilenameExt
			}
			return normalized + wgConfigFilenameExt
		}

		if idx := strings.Index(candidate, "_"); idx >= 0 {
			candidate = candidate[idx+1:]
			continue
		}

		normalized := filterWgConfigFilenameChars(candidate)
		if normalized == "" {
			return "wg" + wgConfigFilenameExt
		}
		return normalized[len(normalized)-maxBaseLen:] + wgConfigFilenameExt
	}
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

func filterWgConfigFilenameSegmentChars(s string) string {
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
		case r == '_':
			b.WriteRune(r)
		}
	}
	return b.String()
}

// WG клиент проверяет по регулярке:
// ^[a-zA-Z0-9_=+.-]{1,15}$
