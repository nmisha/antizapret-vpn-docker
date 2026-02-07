package bot

import (
	"sort"
	"strings"
)

// CommandSpec describes a user-facing command for /help and validation.
type CommandSpec struct {
	Cmd     string
	Args    string
	Desc    string
	Section string
	NeedAny []Role
	Hidden  bool // if true, not shown in /help (e.g., legacy alias)
}

// CommandRegistry registers handlers and tracks command specs for /help.
type CommandRegistry struct {
	R      *Router
	specs  []CommandSpec
	specIx map[string]int
}

func NewCommandRegistry(r *Router) *CommandRegistry {
	return &CommandRegistry{
		R:      r,
		specs:  make([]CommandSpec, 0, 64),
		specIx: make(map[string]int),
	}
}

// Handle registers a handler WITHOUT adding to /help (internal routes, callbacks, etc.).
func (cr *CommandRegistry) Handle(cmd string, h HandlerFunc, mws ...Middleware) {
	cr.R.Handle(cmd, h, mws...)
}

// Alias registers an alias WITHOUT adding to /help.
func (cr *CommandRegistry) Alias(alias, cmd string) {
	cr.R.Alias(alias, cmd)
}

// Command registers a user-facing command and adds it to registry for /help.
func (cr *CommandRegistry) Command(spec CommandSpec, h HandlerFunc, mws ...Middleware) {
	cr.R.Handle(spec.Cmd, h, mws...)
	cr.addSpec(spec)
}

// AliasCommand registers an alias to a canonical command and (optionally) adds it to /help.
func (cr *CommandRegistry) AliasCommand(alias, cmd string, showInHelp bool, specOverride *CommandSpec) {
	cr.R.Alias(alias, cmd)
	if showInHelp {
		if specOverride != nil {
			cr.addSpec(*specOverride)
		} else {
			cr.addSpec(CommandSpec{Cmd: alias, Hidden: false})
		}
	}
}

// Specs returns a copy of all registered command specs.
func (cr *CommandRegistry) Specs() []CommandSpec {
	out := make([]CommandSpec, len(cr.specs))
	copy(out, cr.specs)
	return out
}

func (cr *CommandRegistry) addSpec(spec CommandSpec) {
	spec.Cmd = strings.TrimSpace(spec.Cmd)
	if spec.Cmd == "" {
		return
	}
	// Prevent duplicates: last writer wins (useful during refactors).
	if i, ok := cr.specIx[spec.Cmd]; ok {
		cr.specs[i] = spec
		return
	}
	cr.specIx[spec.Cmd] = len(cr.specs)
	cr.specs = append(cr.specs, spec)
}

// VisibleSpecsForUser filters specs by roles and Hidden flag.
func (cr *CommandRegistry) VisibleSpecsForUser(u User) []CommandSpec {
	out := make([]CommandSpec, 0, len(cr.specs))
	for _, s := range cr.specs {
		if s.Hidden {
			continue
		}
		if !strings.HasPrefix(s.Cmd, "/") {
			continue
		}
		if len(s.NeedAny) == 0 {
			out = append(out, s)
			continue
		}
		for _, r := range s.NeedAny {
			if u.Has(r) {
				out = append(out, s)
				break
			}
		}
	}
	return out
}

// Sections returns visible specs grouped by section, preserving stable section ordering.
func (cr *CommandRegistry) SectionsForUser(u User) []helpBlock {
	visible := cr.VisibleSpecsForUser(u)

	// preserve section order by first appearance in cr.specs
	sectionOrder := make([]string, 0, 16)
	seen := make(map[string]bool)
	// build from all specs, but only for sections that have visible entries
	visibleBySection := make(map[string][]CommandSpec)

	for _, s := range visible {
		sec := strings.TrimSpace(s.Section)
		if sec == "" {
			sec = "Прочее"
		}
		visibleBySection[sec] = append(visibleBySection[sec], s)
	}

	for _, s := range cr.specs {
		sec := strings.TrimSpace(s.Section)
		if sec == "" {
			sec = "Прочее"
		}
		if seen[sec] {
			continue
		}
		if _, ok := visibleBySection[sec]; ok {
			seen[sec] = true
			sectionOrder = append(sectionOrder, sec)
		}
	}

	blocks := make([]helpBlock, 0, len(sectionOrder))
	for _, sec := range sectionOrder {
		items := visibleBySection[sec]
		// stable order: keep registration order; but ensure deterministic by sorting by cmd within same section if needed
		sort.SliceStable(items, func(i, j int) bool { return items[i].Cmd < items[j].Cmd })
		blocks = append(blocks, helpBlock{Title: sec, Items: items})
	}
	return blocks
}

// helpBlock is reused by help renderer.
type helpBlock struct {
	Title string
	Items []CommandSpec
}
