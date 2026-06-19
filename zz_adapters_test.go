//go:build wip_daemonapi
// +build wip_daemonapi

// SPDX-License-Identifier: AGPL-3.0-or-later

package runtime_test

import (
	"testing"
	"time"

	"github.com/pilot-protocol/pilotprotocol/pkg/daemon"
	"github.com/pilot-protocol/handshake"
	"github.com/pilot-protocol/runtime"
)

// TestHandshakeRuntime_Accessors exercises the read-only accessors on a
// HandshakeRuntime backed by a freshly-constructed daemon. The daemon has
// no identity, no handshake service, no registry client, so every accessor
// should return zero / nil / sensible-fallback.
func TestHandshakeRuntime_Accessors(t *testing.T) {
	t.Parallel()
	d := daemon.New(daemon.Config{})
	hr := runtime.NewHandshakeRuntime(d)

	if got := hr.NodeID(); got != 0 {
		t.Errorf("NodeID = %d, want 0", got)
	}
	if hr.HasIdentity() {
		t.Errorf("HasIdentity = true, want false")
	}
	if pk := hr.PublicKey(); pk != nil {
		t.Errorf("PublicKey = %v, want nil", pk)
	}
	if sig := hr.Sign([]byte("hello")); sig != nil {
		t.Errorf("Sign on nil identity = %v, want nil", sig)
	}
	if got := hr.IdentityPath(); got != "" {
		t.Errorf("IdentityPath = %q, want empty", got)
	}
	if hr.TrustAutoApprove() {
		t.Errorf("TrustAutoApprove = true, want false (zero Config)")
	}
	if name, ok := hr.IsTrusted(0xCAFE); ok || name != "" {
		t.Errorf("IsTrusted(0xCAFE) = (%q, %v); want (\"\", false)", name, ok)
	}
	// PublishEvent on a daemon with a bus should not panic.
	hr.PublishEvent("test.topic", map[string]any{"k": "v"})

	// RemoveTunnelPeer on a daemon with an empty tunnel manager is a no-op.
	hr.RemoveTunnelPeer(0xDEAD)

	// Registry on a daemon with nil regConn returns nil.
	if reg := hr.Registry(); reg != nil {
		t.Errorf("Registry = %v, want nil (no regConn)", reg)
	}
}

// TestHandshakeRuntime_PortListener exercises the Listener-bind path.
func TestHandshakeRuntime_PortListener(t *testing.T) {
	t.Parallel()
	d := daemon.New(daemon.Config{})
	hr := runtime.NewHandshakeRuntime(d)

	ln, err := hr.PortListener(54321)
	if err != nil {
		t.Fatalf("PortListener: %v", err)
	}
	if ln == nil {
		t.Fatal("listener is nil")
	}
	if err := ln.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// TestPolicyRuntime_Accessors exercises read-only PolicyRuntime methods.
func TestPolicyRuntime_Accessors(t *testing.T) {
	t.Parallel()
	d := daemon.New(daemon.Config{})
	pr := runtime.NewPolicyRuntime(d)

	if got := pr.NodeID(); got != 0 {
		t.Errorf("NodeID = %d, want 0", got)
	}
	if got := pr.AdminToken(); got != "" {
		t.Errorf("AdminToken = %q, want empty", got)
	}
	if peers := pr.TrustedPeers(); len(peers) != 0 {
		t.Errorf("TrustedPeers = %v, want empty (no handshakes)", peers)
	}
	pr.PublishEvent("policy.test", map[string]any{"x": 1})
	pr.SetMemberTags(1, []string{"a", "b"})

	// HandshakeRevokeTrust / HandshakeSendRequest both error when
	// handshakes == nil (the documented contract).
	if err := pr.RevokeTrust(0xCAFE); err == nil {
		t.Error("RevokeTrust on nil handshakes: want error, got nil")
	}
	if err := pr.SendHandshakeRequest(0xCAFE, "why"); err == nil {
		t.Error("SendHandshakeRequest on nil handshakes: want error, got nil")
	}
}

// TestNewHandshakeServiceAdapter wires a real handshake.Service to its
// daemon.HandshakeService adapter and exercises the read-only methods.
// The service has an empty manager — all trust queries return zero state.
func TestNewHandshakeServiceAdapter(t *testing.T) {
	t.Parallel()
	d := daemon.New(daemon.Config{})
	hr := runtime.NewHandshakeRuntime(d)
	svc := handshake.NewService(hr)
	defer svc.Manager().Stop()

	adapter := runtime.NewHandshakeServiceAdapter(svc)
	if adapter == nil {
		t.Fatal("adapter is nil")
	}

	// All read-only accessors should return zero / empty / false on a
	// fresh manager.
	if adapter.IsTrusted(0xCAFE) {
		t.Errorf("IsTrusted on fresh manager: want false")
	}
	if got := adapter.TrustedPeers(); len(got) != 0 {
		t.Errorf("TrustedPeers = %v, want empty", got)
	}
	if got := adapter.PendingRequests(); len(got) != 0 {
		t.Errorf("PendingRequests = %v, want empty", got)
	}
	if got := adapter.PendingCount(); got != 0 {
		t.Errorf("PendingCount = %d, want 0", got)
	}
	// WaitForTrust returns false on timeout (peer never approves).
	if adapter.WaitForTrust(0xCAFE, 10*time.Millisecond) {
		t.Errorf("WaitForTrust: want false (timeout)")
	}

	// ProcessRelayedRequest stages a pending request — exercises the
	// internal map-write path without going through the network.
	adapter.ProcessRelayedRequest(0x1234, "test")
	if got := adapter.PendingCount(); got != 1 {
		t.Errorf("PendingCount after relayed request: %d, want 1", got)
	}
	if got := adapter.PendingRequests(); len(got) != 1 {
		t.Errorf("PendingRequests after relayed request: %d, want 1", len(got))
	}
	adapter.ProcessRelayedApproval(0x5678) // unknown peer — should be safe no-op
	adapter.ProcessRelayedRejection(0x5678)

	// Stop is idempotent.
	adapter.Stop()
	adapter.Stop()
}

// TestAsDaemonPolicyManager_Nil exercises the nil short-circuit branch.
func TestAsDaemonPolicyManager_Nil(t *testing.T) {
	t.Parallel()
	if got := runtime.AsDaemonPolicyManager(nil); got != nil {
		t.Errorf("AsDaemonPolicyManager(nil) = %v, want nil", got)
	}
}
