package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const debugLogFilename = "dns-guard-debug.log"

func writeDebugLog(cfg Config, format string, args ...any) {
	if !cfg.DebugLogEnabled {
		return
	}
	line := strings.TrimSpace(fmt.Sprintf(format, args...))
	if line == "" {
		return
	}
	path := filepath.Join(filepath.Dir(cfg.StatePath), debugLogFilename)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "%s %s\n", time.Now().UTC().Format(time.RFC3339), line)
}
