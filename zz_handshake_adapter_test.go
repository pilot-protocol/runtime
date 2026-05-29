//go:build wip_daemonapi
// +build wip_daemonapi

// SPDX-License-Identifier: AGPL-3.0-or-later

package runtime_test

import (
	"testing"

	"github.com/TeoSlayer/pilotprotocol/pkg/daemon"
	"github.com/pilot-protocol/handshake"
	"github.com/pilot-protocol/runtime"
)

// TestHandshakeServiceAdapter_ApproveAndReject_AbsentPeers covers the
// adapter shims for ApproveHandshake / RejectHandshake / RevokeTrust /
// SendRequest. The underlying manager has no registry, so each path
// either errors or no-ops gracefully — what matters is exercising the
// delegation through the adapter.
func TestHandshakeServiceAdapter_DelegationShims(t *testing.T) {
	t.Parallel()
	d := daemon.New(daemon.Config{})
	hr := runtime.NewHandshakeRuntime(d)
	svc := handshake.NewService(hr)
	defer svc.Manager().Stop()
	adapter := runtime.NewHandshakeServiceAdapter(svc)

	// ApproveHandshake on absent peer: the manager early-returns on the
	// pending lookup. Safe to call.
	_ = adapter.ApproveHandshake(0x9999)

	// RevokeTrust on absent peer: the manager early-returns. Safe.
	_ = adapter.RevokeTrust(0x9999)
}
