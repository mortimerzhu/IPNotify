// Package notifier defines the Notifier interface and a factory registry.
// Concrete notifiers live in sub-packages and register themselves via init().
package notifier

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mortimerzhu/IPNotify/pkg/event"
)

// Notifier delivers an IP-change event to a destination.
type Notifier interface {
	// Name returns a human-readable identifier for logging.
	Name() string
	// Notify delivers the event, honoring ctx for cancellation/timeout.
	Notify(ctx context.Context, e event.Event) error
}

// Factory builds a Notifier from its opaque config map.
type Factory func(cfg map[string]any) (Notifier, error)

var registry = map[string]Factory{}

// Register adds a notifier factory under typ. It is meant to be called from
// notifier sub-package init() functions. Registering a duplicate type panics.
func Register(typ string, f Factory) {
	if _, ok := registry[typ]; ok {
		panic("notifier: duplicate registration for type " + typ)
	}
	registry[typ] = f
}

// Build constructs a notifier of the given type from cfg.
func Build(typ string, cfg map[string]any) (Notifier, error) {
	f, ok := registry[typ]
	if !ok {
		return nil, fmt.Errorf("notifier: unknown type %q (registered: %s)", typ, strings.Join(registered(), ", "))
	}
	return f(cfg)
}

func registered() []string {
	types := make([]string, 0, len(registry))
	for t := range registry {
		types = append(types, t)
	}
	sort.Strings(types)
	return types
}
