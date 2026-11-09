// SPDX-License-Identifier: AGPL-3.0-or-later

package runtime

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/pilot-protocol/common/coreapi"
	"github.com/pilot-protocol/common/daemonapi"
	"github.com/pilot-protocol/common/protocol"
	registryclient "github.com/pilot-protocol/common/registry/client"
	"github.com/pilot-protocol/handshake"
)

// handshakeCloseDelay matches plugins/handshake.HandshakeCloseDelay.
const handshakeCloseDelay = 500 * time.Millisecond

// HandshakeRuntime adapts daemonapi.Daemon to the handshake.Runtime
// interface. Lives here (L12 composition root) so plugins/handshake
// stays free of pkg/daemon.
type HandshakeRuntime struct{ d daemonapi.Daemon }

// NewHandshakeRuntime returns a handshake.Runtime backed by d.
func NewHandshakeRuntime(d daemonapi.Daemon) handshake.Runtime {
	return HandshakeRuntime{d: d}
}

func (r HandshakeRuntime) NodeID() uint32    { return r.d.NodeID() }
func (r HandshakeRuntime) HasIdentity() bool { return r.d.Identity() != nil }

func (r HandshakeRuntime) PublicKey() ed25519.PublicKey {
	id := r.d.Identity()
	if id == nil {
		return nil
	}
	return id.PublicKey
}

func (r HandshakeRuntime) Sign(msg []byte) []byte {
	id := r.d.Identity()
	if id == nil {
		return nil
	}
	return id.Sign(msg)
}

func (r HandshakeRuntime) IdentityPath() string   { return r.d.IdentityPath() }
func (r HandshakeRuntime) TrustAutoApprove() bool { return r.d.TrustAutoApprove() }

func (r HandshakeRuntime) IsTrusted(nodeID uint32) (string, bool) {
	tc := r.d.GetTrustChecker()
	if tc == nil {
		return "", false
	}
	return tc.IsTrusted(nodeID)
}

// IsTrustedWithKey is handshake.KeyedTrustChecker: the trusted-agents list
// with an entry's key pin checked against the key the peer presented. The
// handshake manager asks this when it implements it, and otherwise falls
// back to IsTrusted, which never checked pins. A checker without pin
// support answers by node id as before; a key that isn't valid base64 is
// not trusted.
func (r HandshakeRuntime) IsTrustedWithKey(nodeID uint32, pubKeyB64 string) (string, bool) {
	tc := r.d.GetTrustChecker()
	if tc == nil {
		return "", false
	}
	kc, ok := tc.(interface {
		IsTrustedWithKey(nodeID uint32, pubKey []byte) (string, bool)
	})
	if !ok {
		return tc.IsTrusted(nodeID)
	}
	key, err := base64.StdEncoding.DecodeString(pubKeyB64)
	if err != nil {
		return "", false
	}
	return kc.IsTrustedWithKey(nodeID, key)
}

func (r HandshakeRuntime) PublishEvent(topic string, payload map[string]any) {
	r.d.PublishEvent(topic, payload)
}

func (r HandshakeRuntime) PortListener(port uint16) (coreapi.Listener, error) {
	return daemonStreams{d: r.d}.Listen(port)
}

func (r HandshakeRuntime) DialAndSend(peerNodeID uint32, port uint16, data []byte) error {
	peerAddr := protocol.Addr{Network: 0, Node: peerNodeID}
	conn, err := r.d.DialConnection(peerAddr, port)
	if err != nil {
		return err
	}
	if err := r.d.SendData(conn, data); err != nil {
		r.d.CloseConnection(conn)
		return fmt.Errorf("send handshake data: %w", err)
	}
	go func() {
		time.Sleep(handshakeCloseDelay)
		r.d.CloseConnection(conn)
	}()
	return nil
}

func (r HandshakeRuntime) RemoveTunnelPeer(nodeID uint32) {
	r.d.Tunnels().RemovePeer(nodeID)
}

func (r HandshakeRuntime) Registry() handshake.RegistryClient {
	c := r.d.RegistryClient()
	if c == nil {
		return nil
	}
	return c
}

// handshakeServiceAdapter converts between plugins/handshake types and
// the daemon-local daemonapi.HandshakeService mirror types.
type handshakeServiceAdapter struct{ m *handshake.Manager }

// NewHandshakeServiceAdapter wraps a *handshake.Service so it satisfies
// daemonapi.HandshakeService.
func NewHandshakeServiceAdapter(svc *handshake.Service) daemonapi.HandshakeService {
	return handshakeServiceAdapter{m: svc.Manager()}
}

func (a handshakeServiceAdapter) IsTrusted(nodeID uint32) bool { return a.m.IsTrusted(nodeID) }

// IsTrustedWithKey passes the daemon's key-bound trust check through to the
// handshake manager. The daemon type-asserts its handshake service for this
// method; without it the daemon's key checks fell back to node id alone.
func (a handshakeServiceAdapter) IsTrustedWithKey(nodeID uint32, pubKeyB64 string) bool {
	return a.m.IsTrustedWithKey(nodeID, pubKeyB64)
}

// The daemon's and the handshake manager's optional, key-bound interfaces:
// a missing method here silently weakens their checks to node id.
var (
	_ interface {
		IsTrustedWithKey(nodeID uint32, pubKeyB64 string) bool
	} = handshakeServiceAdapter{}
	_ handshake.KeyedTrustChecker = HandshakeRuntime{}
)

func (a handshakeServiceAdapter) TrustedPeers() []daemonapi.HandshakeTrustRecord {
	src := a.m.TrustedPeers()
	out := make([]daemonapi.HandshakeTrustRecord, len(src))
	for i, r := range src {
		out[i] = daemonapi.HandshakeTrustRecord{
			NodeID:     r.NodeID,
			PublicKey:  r.PublicKey,
			ApprovedAt: r.ApprovedAt,
			Mutual:     r.Mutual,
			Network:    r.Network,
		}
	}
	return out
}

func (a handshakeServiceAdapter) PendingRequests() []daemonapi.HandshakePendingRecord {
	src := a.m.PendingRequests()
	out := make([]daemonapi.HandshakePendingRecord, len(src))
	for i, p := range src {
		out[i] = daemonapi.HandshakePendingRecord{
			NodeID:        p.NodeID,
			PublicKey:     p.PublicKey,
			Justification: p.Justification,
			ReceivedAt:    p.ReceivedAt,
		}
	}
	return out
}

func (a handshakeServiceAdapter) PendingCount() int { return a.m.PendingCount() }

func (a handshakeServiceAdapter) SendRequest(peerNodeID uint32, justification string) error {
	return a.m.SendRequest(peerNodeID, justification)
}

func (a handshakeServiceAdapter) ApproveHandshake(peerNodeID uint32) error {
	return a.m.ApproveHandshake(peerNodeID)
}

func (a handshakeServiceAdapter) RejectHandshake(peerNodeID uint32, reason string) error {
	return a.m.RejectHandshake(peerNodeID, reason)
}

func (a handshakeServiceAdapter) RevokeTrust(peerNodeID uint32) error {
	return a.m.RevokeTrust(peerNodeID)
}

func (a handshakeServiceAdapter) WaitForTrust(peerNodeID uint32, timeout time.Duration) bool {
	return a.m.WaitForTrust(peerNodeID, timeout)
}

func (a handshakeServiceAdapter) ProcessRelayedRequest(fromNodeID uint32, justification string) {
	a.m.ProcessRelayedRequest(fromNodeID, justification)
}

func (a handshakeServiceAdapter) ProcessRelayedApproval(fromNodeID uint32) {
	a.m.ProcessRelayedApproval(fromNodeID)
}

func (a handshakeServiceAdapter) ProcessRelayedRejection(fromNodeID uint32) {
	a.m.ProcessRelayedRejection(fromNodeID)
}

func (a handshakeServiceAdapter) Stop() { a.m.Stop() }

// Compile-time guards.
var (
	_ handshake.Runtime        = HandshakeRuntime{}
	_ daemonapi.HandshakeService  = handshakeServiceAdapter{}
	_ handshake.RegistryClient = (*registryclient.Client)(nil)
)
