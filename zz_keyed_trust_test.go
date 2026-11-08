// SPDX-License-Identifier: AGPL-3.0-or-later

package runtime_test

import (
	"bytes"
	"encoding/base64"
	"testing"

	"github.com/pilot-protocol/common/daemonapi"
	"github.com/pilot-protocol/handshake"
	"github.com/pilot-protocol/runtime"
)

// pinnedChecker is a trusted-agents list with one entry pinned to a key.
type pinnedChecker struct {
	id  uint32
	key []byte
}

func (c pinnedChecker) IsTrusted(nodeID uint32) (string, bool) { return "agent", nodeID == c.id }
func (c pinnedChecker) IsTrustedWithKey(nodeID uint32, key []byte) (string, bool) {
	return "agent", nodeID == c.id && bytes.Equal(key, c.key)
}

// trustDaemon is a daemon whose only working method is GetTrustChecker,
// all IsTrustedWithKey needs.
type trustDaemon struct {
	daemonapi.Daemon
	tc daemonapi.TrustChecker
}

func (d trustDaemon) GetTrustChecker() daemonapi.TrustChecker { return d.tc }

// nodeOnlyChecker has no key pins.
type nodeOnlyChecker struct{ id uint32 }

func (c nodeOnlyChecker) IsTrusted(nodeID uint32) (string, bool) { return "agent", nodeID == c.id }

// The handshake manager asks KeyedTrustChecker when its runtime has it; the
// runtime used not to, so a trusted-agents key pin was never checked.
func TestHandshakeRuntimeChecksTrustedAgentKeyPins(t *testing.T) {
	t.Parallel()
	key := bytes.Repeat([]byte{7}, 32)
	kc, ok := runtime.NewHandshakeRuntime(trustDaemon{tc: pinnedChecker{id: 42, key: key}}).(handshake.KeyedTrustChecker)
	if !ok {
		t.Fatal("HandshakeRuntime is not a handshake.KeyedTrustChecker")
	}
	if _, ok := kc.IsTrustedWithKey(42, base64.StdEncoding.EncodeToString(key)); !ok {
		t.Error("the pinned key was refused")
	}
	if _, ok := kc.IsTrustedWithKey(42, base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{8}, 32))); ok {
		t.Error("another key passed the pin")
	}
	if _, ok := kc.IsTrustedWithKey(42, "not base64!"); ok {
		t.Error("an undecodable key passed")
	}

	// A checker without pins answers by node id, as IsTrusted did.
	if _, ok := runtime.NewHandshakeRuntime(trustDaemon{tc: nodeOnlyChecker{id: 42}}).(handshake.KeyedTrustChecker).IsTrustedWithKey(42, "anything"); !ok {
		t.Error("a checker without pins stopped trusting its entry")
	}
}
