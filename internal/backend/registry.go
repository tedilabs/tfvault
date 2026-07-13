package backend

import (
	"fmt"
	"sort"
	"strings"
)

// Factory builds a backend from the raw options of a profile's backend
// block. Each factory must reject unknown option keys.
type Factory func(opts map[string]string) (Backend, error)

var registry = map[string]Factory{}

// Register makes a backend factory available under name. It is intended
// to be called from backend package init() functions.
func Register(name string, f Factory) {
	if _, dup := registry[name]; dup {
		panic(fmt.Sprintf("backend %q registered twice", name))
	}
	registry[name] = f
}

// New builds the named backend with the given options.
func New(name string, opts map[string]string) (Backend, error) {
	f, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown backend %q (available: %s)", name, strings.Join(Names(), ", "))
	}
	return f(opts)
}

// Names returns the registered backend names, sorted.
func Names() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// IsRegistered reports whether name is a registered backend.
func IsRegistered(name string) bool {
	_, ok := registry[name]
	return ok
}
