package bot

import (
	"os"
	"path/filepath"
)

// ensureJSONFile makes sure a file exists; if not, it creates it with defaultContent.
// It also ensures the parent directory exists.
func ensureJSONFile(path string, defaultContent string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, []byte(defaultContent), 0644)
}
