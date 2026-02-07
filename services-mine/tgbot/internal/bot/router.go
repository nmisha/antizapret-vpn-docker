package bot

import "strings"

type HandlerFunc func(ctx *Ctx, arg string)
type Middleware func(next HandlerFunc) HandlerFunc

type Router struct {
	handlers map[string]HandlerFunc
	aliases  map[string]string // alias -> canonical command
}

func NewRouter() *Router {
	return &Router{
		handlers: make(map[string]HandlerFunc),
		aliases:  make(map[string]string),
	}
}

// Handle регистрирует хэндлер и оборачивает его middleware (справа налево).
func (r *Router) Handle(cmd string, h HandlerFunc, mws ...Middleware) {
	if len(mws) > 0 {
		h = Chain(h, mws...)
	}
	r.handlers[cmd] = h
}

func (r *Router) Alias(alias, cmd string) { r.aliases[alias] = cmd }

func (r *Router) Dispatch(ctx *Ctx, cmd, arg string) bool {
	if cmd == "" {
		return false
	}
	if mapped, ok := r.aliases[cmd]; ok {
		cmd = mapped
	}
	cmd = strings.TrimSpace(cmd)

	h, ok := r.handlers[cmd]
	if !ok {
		return false
	}
	h(ctx, arg)
	return true
}

// Chain применяет middleware к handler'у.
func Chain(h HandlerFunc, mws ...Middleware) HandlerFunc {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}
