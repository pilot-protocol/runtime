// SPDX-License-Identifier: AGPL-3.0-or-later

package runtime

import (
	"testing"
)

// TestDaemonEventBus_NilDaemon_Publish covers the defensive nil-daemon
// short-circuit in daemonEventBus.Publish. Constructing the adapter
// with a nil daemon (only reachable via direct package-internal use, but
// still a documented safety branch in events.go) must not panic.
func TestDaemonEventBus_NilDaemon_Publish(t *testing.T) {
	t.Parallel()
	bus := daemonEventBus{d: nil}
	// Must be a no-op (no daemon to forward to). The branch is the
	// `if b.d == nil { return }` guard.
	bus.Publish("nil.daemon", map[string]any{"x": 1})
}

// TestDaemonEventBus_NilDaemon_Subscribe covers the nil-daemon branch of
// Subscribe — it returns a closed channel and a no-op cancel so callers
// never block. The interesting assertion is that the returned channel
// is already drained (recv returns the zero value with ok=false).
func TestDaemonEventBus_NilDaemon_Subscribe(t *testing.T) {
	t.Parallel()
	bus := daemonEventBus{d: nil}

	ch, cancel := bus.Subscribe("anything.*")
	if ch == nil {
		t.Fatal("Subscribe returned nil channel")
	}
	if cancel == nil {
		t.Fatal("Subscribe returned nil cancel")
	}

	// Channel should be closed — a receive must succeed with ok=false.
	select {
	case ev, ok := <-ch:
		if ok {
			t.Errorf("Subscribe(nil daemon): want closed channel, got event %+v", ev)
		}
	default:
		t.Fatal("Subscribe(nil daemon): channel is not closed (would block)")
	}

	// Cancel must be safe to invoke and idempotent.
	cancel()
	cancel()
}
