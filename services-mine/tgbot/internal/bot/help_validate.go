package bot

import (
	"log"
	"strings"
)

// ValidateHelpCoverage checks that every registered slash-command is present in the registry
// and that the registry does not contain commands without handlers.
func ValidateHelpCoverage(reg *CommandRegistry) {
	if reg == nil || reg.R == nil {
		return
	}

	// Build set from registry specs (including hidden, because we still want to validate they exist as handlers).
	specSet := make(map[string]struct{}, len(reg.specs))
	for _, s := range reg.specs {
		if strings.HasPrefix(s.Cmd, "/") {
			specSet[s.Cmd] = struct{}{}
		}
	}

	// Missing: registered handlers but not in registry specs
	for cmd := range reg.R.handlers {
		if !strings.HasPrefix(cmd, "/") {
			continue
		}
		if _, ok := specSet[cmd]; !ok {
			log.Printf("HELP: command registered but missing in registry: %s", cmd)
		}
	}

	// Extra: commands in registry but not registered (and not an alias)
	for cmd := range specSet {
		if _, ok := reg.R.handlers[cmd]; ok {
			continue
		}
		if _, ok := reg.R.aliases[cmd]; ok {
			continue
		}
		log.Printf("HELP: command present in registry but not registered: %s", cmd)
	}
}
