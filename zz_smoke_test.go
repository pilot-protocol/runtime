//go:build wip_daemonapi
// +build wip_daemonapi

// SPDX-License-Identifier: AGPL-3.0-or-later

package runtime_test

import (
	"testing"

	"github.com/pilot-protocol/pilotprotocol/pkg/daemon"
	"github.com/pilot-protocol/runtime"
)

// TestPackageCompiles is a no-op test that proves the test binary links
// against the runtime package. If the package fails to build, this test
// fails to compile, which is the load-bearing assertion.
func TestPackageCompiles(t *testing.T) {
	t.Log("package runtime compiles and links")
}

// TestNewRuntime exercises the runtime.New constructor with a zero-value
// daemon Config. We deliberately do not call StartPlugins — that requires
// a fully wired daemon (registry, sockets, identity). Construction is
// enough to flush out import-cycle or interface-mismatch regressions in
// the composition root.
func TestNewRuntime(t *testing.T) {
	d := daemon.New(daemon.Config{})
	if d == nil {
		t.Fatal("daemon.New returned nil")
	}

	r := runtime.New(d)
	if r == nil {
		t.Fatal("runtime.New returned nil")
	}

	if got := r.Daemon(); got != d {
		t.Fatalf("Runtime.Daemon() = %p, want %p", got, d)
	}
}

// TestNewHandshakeRuntime exercises the handshake.Runtime adapter
// constructor. The returned value satisfies the handshake.Runtime
// interface — that's enforced at compile time inside the package via
// a `var _ handshake.Runtime = ...` guard — so this test just verifies
// the constructor returns a non-nil adapter from public callers.
func TestNewHandshakeRuntime(t *testing.T) {
	d := daemon.New(daemon.Config{})
	hr := runtime.NewHandshakeRuntime(d)
	if hr == nil {
		t.Fatal("NewHandshakeRuntime returned nil")
	}
}

// TestNewPolicyRuntime exercises the policy.Runtime adapter
// constructor. Same shape as the handshake adapter test.
func TestNewPolicyRuntime(t *testing.T) {
	d := daemon.New(daemon.Config{})
	pr := runtime.NewPolicyRuntime(d)
	if pr == nil {
		t.Fatal("NewPolicyRuntime returned nil")
	}
}
