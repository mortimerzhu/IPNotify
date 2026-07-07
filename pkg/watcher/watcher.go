// Package watcher defines the Watcher interface implemented by the local and
// public IP watchers.
package watcher

import (
	"context"

	"github.com/mortimerzhu/IPNotify/pkg/event"
)

// Watcher observes a category of IP and emits an Event on every change.
type Watcher interface {
	// Name returns a human-readable identifier for logging.
	Name() string
	// Watch runs until ctx is cancelled, sending change events to out.
	// It returns nil on clean shutdown (ctx cancelled) or an error on
	// unrecoverable failure.
	Watch(ctx context.Context, out chan<- event.Event) error
}
