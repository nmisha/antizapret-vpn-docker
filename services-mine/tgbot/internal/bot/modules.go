package bot

// Module groups a feature set (commands etc.).
// We keep modules in the same package (for now) to avoid a breaking refactor,
// but wiring happens via this registry, so the bot stays modular.
type Module interface {
	Name() string
	Register(reg *CommandRegistry)
}

type funcModule struct {
	name string
	fn   func(reg *CommandRegistry)
}

func (m funcModule) Name() string { return m.name }

func (m funcModule) Register(reg *CommandRegistry) {
	if m.fn != nil {
		m.fn(reg)
	}
}

// DefaultModules returns all bot feature modules in registration order.
func DefaultModules() []Module {
	return []Module{
		funcModule{"admin", func(reg *CommandRegistry) { RegisterAdminHandlers(reg) }},
		funcModule{"admin_accounts", func(reg *CommandRegistry) {
			RegisterAdminAccountsHandlers(reg)
			RegisterAdminAccountsUIHandlers(reg)
		}},
		funcModule{"admin_settings", func(reg *CommandRegistry) { RegisterAdminSettingsHandlers(reg) }},
		funcModule{"domains", func(reg *CommandRegistry) { RegisterDomainHandlers(reg) }},
		funcModule{"services", func(reg *CommandRegistry) { RegisterServiceHandlers(reg) }},
		funcModule{"wg", func(reg *CommandRegistry) {
			RegisterWgProfilesHandlers(reg)
			RegisterWgNameCommands(reg)
		}},
		funcModule{"net", func(reg *CommandRegistry) { RegisterNetHandlers(reg) }},
		funcModule{"support", func(reg *CommandRegistry) { RegisterSupportHandlers(reg) }},
		funcModule{"ai", func(reg *CommandRegistry) { RegisterAIHandlers(reg) }},
	}
}

// RegisterAllModules registers all modules in DefaultModules().
func RegisterAllModules(reg *CommandRegistry) {
	for _, m := range DefaultModules() {
		m.Register(reg)
	}
}
