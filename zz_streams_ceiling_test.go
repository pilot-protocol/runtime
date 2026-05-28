// SPDX-License-Identifier: AGPL-3.0-or-later

package runtime_test

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/TeoSlayer/pilotprotocol/pkg/coreapi"
	"github.com/TeoSlayer/pilotprotocol/pkg/daemon"
	"github.com/TeoSlayer/pilotprotocol/pkg/protocol"
	"github.com/pilot-protocol/handshake"
	"github.com/pilot-protocol/runtime"
)

// TestDaemonStreams_Dial_CancelledCtxReturnsError verifies the Dial shim
// short-circuits cleanly when the caller's context is already cancelled.
// DialConnectionContext early-returns ctx.Err() before any side-effecting
// work (port alloc, ensureTunnel, SYN send), so this exercises Dial without
// requiring a running daemon / registry. Covers the err-propagation branch
// of daemonStreams.Dial.
func TestDaemonStreams_Dial_CancelledCtxReturnsError(t *testing.T) {
	t.Parallel()
	d := daemon.New(daemon.Config{})
	rt := runtime.New(d)

	svc := &fakeService{name: "fake", order: 100}
	if err := rt.Register(svc); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := rt.StartPlugins(context.Background()); err != nil {
		t.Fatalf("StartPlugins: %v", err)
	}
	t.Cleanup(func() { _ = rt.StopPlugins(context.Background()) })

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel before dial

	dst := coreapi.Addr{Network: 0, Node: 0xCAFE}
	stream, err := svc.gotDeps.Streams.Dial(ctx, dst, 12345)
	if err == nil {
		t.Fatalf("Dial with cancelled ctx: want error, got stream=%v", stream)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Dial err = %v, want context.Canceled", err)
	}
	if stream != nil {
		t.Errorf("Dial returned non-nil stream on error: %v", stream)
	}
}

// TestDaemonStreams_SendDatagram_BroadcastRejected exercises the
// SendDatagram shim. The underlying daemon rejects broadcast addresses
// (Node=0xFFFFFFFF) with a deterministic error before any network I/O,
// which is exactly the surface needed to drive the adapter without a
// running tunnel.
func TestDaemonStreams_SendDatagram_BroadcastRejected(t *testing.T) {
	t.Parallel()
	d := daemon.New(daemon.Config{})
	rt := runtime.New(d)

	svc := &fakeService{name: "fake", order: 100}
	if err := rt.Register(svc); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := rt.StartPlugins(context.Background()); err != nil {
		t.Fatalf("StartPlugins: %v", err)
	}
	t.Cleanup(func() { _ = rt.StopPlugins(context.Background()) })

	bcast := coreapi.Addr{Network: 0, Node: 0xFFFFFFFF}
	err := svc.gotDeps.Streams.SendDatagram(context.Background(), bcast, 7, []byte("ping"))
	if err == nil {
		t.Fatal("SendDatagram(broadcast): want error, got nil")
	}
	if !strings.Contains(err.Error(), "broadcast") {
		t.Errorf("SendDatagram err = %q, want substring 'broadcast'", err.Error())
	}
}

// TestDaemonListener_AcceptDeliversStreamWithAccessors pushes a synthetic
// *daemon.Connection through Listener.AcceptCh and verifies that:
//  1. Accept wires the connection into a streamAdapter via newStreamAdapter,
//  2. every accessor (LocalAddr, LocalPort, RemoteAddr, RemotePort)
//     returns the underlying connection fields, and
//  3. the deadline shims are valid no-ops (return nil).
//
// This is the only path that exercises newStreamAdapter outside a real
// Dial — Dial requires a live tunnel which is ceiling-bound for a unit
// test in this package.
func TestDaemonListener_AcceptDeliversStreamWithAccessors(t *testing.T) {
	t.Parallel()
	d := daemon.New(daemon.Config{})
	rt := runtime.New(d)

	svc := &fakeService{name: "fake", order: 100}
	if err := rt.Register(svc); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := rt.StartPlugins(context.Background()); err != nil {
		t.Fatalf("StartPlugins: %v", err)
	}
	t.Cleanup(func() { _ = rt.StopPlugins(context.Background()) })

	streams := svc.gotDeps.Streams
	ln, err := streams.Listen(54330)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	// Build a synthetic *daemon.Connection through the daemon's own port
	// manager so RecvBuf / SendBuf / RetxStop etc. are initialized the
	// way the real dial path would set them up. The dst-side metadata
	// is what the streamAdapter accessors will surface back.
	remote := protocol.Addr{Network: 7, Node: 0x1234}
	conn := d.Ports().NewConnection(54330, remote, 8888)
	// Drop in a known LocalAddr so the accessor returns a stable value
	// for the assertion.
	conn.LocalAddr = protocol.Addr{Network: 7, Node: 0xABCD}

	// Find the daemon-side listener so we can push our synthetic
	// connection through the accept channel.
	dln := d.Ports().GetListener(54330)
	if dln == nil {
		t.Fatal("daemon-side Listener missing for port 54330")
	}
	if ok := dln.TrySend(conn); !ok {
		t.Fatal("TrySend: failed to enqueue synthetic conn")
	}

	stream, err := ln.Accept()
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if stream == nil {
		t.Fatal("Accept returned nil stream")
	}

	if got := stream.LocalAddr(); got != conn.LocalAddr {
		t.Errorf("LocalAddr = %+v, want %+v", got, conn.LocalAddr)
	}
	if got := stream.LocalPort(); got != 54330 {
		t.Errorf("LocalPort = %d, want 54330", got)
	}
	if got := stream.RemoteAddr(); got != remote {
		t.Errorf("RemoteAddr = %+v, want %+v", got, remote)
	}
	if got := stream.RemotePort(); got != 8888 {
		t.Errorf("RemotePort = %d, want 8888", got)
	}

	// The deadline shims are documented no-ops returning nil.
	if err := stream.SetDeadline(time.Now().Add(time.Hour)); err != nil {
		t.Errorf("SetDeadline: %v", err)
	}
	if err := stream.SetReadDeadline(time.Now().Add(time.Hour)); err != nil {
		t.Errorf("SetReadDeadline: %v", err)
	}
	if err := stream.SetWriteDeadline(time.Now().Add(time.Hour)); err != nil {
		t.Errorf("SetWriteDeadline: %v", err)
	}

	// Closing the stream goes through daemon.CloseConnection, which for
	// a synthetic conn in StateClosed exits via the non-ESTABLISHED
	// branch: it closes RecvBuf and flips State to FIN_WAIT without
	// generating wire traffic.
	if err := stream.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}

	// After Close (CloseRecvBuf), Read should observe io.EOF — the
	// connAdapter's Read drains the closed channel and returns EOF.
	buf := make([]byte, 4)
	n, err := stream.Read(buf)
	if n != 0 || !errors.Is(err, io.EOF) {
		t.Errorf("Read after Close: got (n=%d, err=%v), want (0, io.EOF)", n, err)
	}
}

// TestStreamAdapter_Write_ConnectionNotEstablished covers
// streamAdapter.Write against a synthetic Connection in StateClosed
// (the default after PortManager.NewConnection). SendData rejects the
// write with "connection not established" before any tunnel I/O, which
// is the only Write branch reachable without a real established
// handshake. The connAdapter loop bails immediately on a non-
// ErrSendBufFull error, so this exercises Write end-to-end.
func TestStreamAdapter_Write_ConnectionNotEstablished(t *testing.T) {
	t.Parallel()
	d := daemon.New(daemon.Config{})
	rt := runtime.New(d)

	svc := &fakeService{name: "fake", order: 100}
	if err := rt.Register(svc); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := rt.StartPlugins(context.Background()); err != nil {
		t.Fatalf("StartPlugins: %v", err)
	}
	t.Cleanup(func() { _ = rt.StopPlugins(context.Background()) })

	streams := svc.gotDeps.Streams
	ln, err := streams.Listen(54331)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	remote := protocol.Addr{Network: 0, Node: 0x5555}
	conn := d.Ports().NewConnection(54331, remote, 9999)
	dln := d.Ports().GetListener(54331)
	if dln == nil {
		t.Fatal("daemon-side Listener missing for port 54331")
	}
	if ok := dln.TrySend(conn); !ok {
		t.Fatal("TrySend: failed to enqueue synthetic conn")
	}

	stream, err := ln.Accept()
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	t.Cleanup(func() { _ = stream.Close() })

	n, werr := stream.Write([]byte("hello"))
	if werr == nil {
		t.Fatalf("Write on StateClosed conn: want error, got n=%d", n)
	}
	if n != 0 {
		t.Errorf("Write returned n=%d on error, want 0", n)
	}
	if !strings.Contains(werr.Error(), "not established") {
		t.Errorf("Write err = %q, want substring 'not established'", werr.Error())
	}
}

// TestPolicyRuntime_ListNodes_NoRegConn drives the ListNodes shim on a
// daemon with no registry connection — RegConnListNodes returns a
// deterministic error in that case, which is the only branch reachable
// without standing up a real registry server.
func TestPolicyRuntime_ListNodes_NoRegConn(t *testing.T) {
	t.Parallel()
	d := daemon.New(daemon.Config{})
	pr := runtime.NewPolicyRuntime(d)

	resp, err := pr.ListNodes(42, "admin-tok")
	if err == nil {
		t.Fatalf("ListNodes: want error (no regConn), got resp=%v", resp)
	}
	if resp != nil {
		t.Errorf("resp = %v, want nil on error", resp)
	}
	// Sanity-check the contract: the documented message mentions the
	// uninitialised registry connection.
	if !strings.Contains(err.Error(), "registry") {
		t.Errorf("err = %q, want substring 'registry'", err.Error())
	}
}

// TestHandshakeServiceAdapter_RejectAndSendRequest drives the two
// remaining adapter shims (RejectHandshake / SendRequest) on a fresh
// manager. RejectHandshake on an absent peer is a safe no-op (the
// manager deletes a non-existent pending entry, persists, and publishes
// an event). SendRequest on a daemon with no tunnel will fail; what we
// care about is exercising the adapter delegation through the shim.
func TestHandshakeServiceAdapter_RejectAndSendRequest(t *testing.T) {
	t.Parallel()
	d := daemon.New(daemon.Config{})
	// RejectHandshake / SendRequest internally drive sendMessage →
	// DialAndSend → DialConnection, which without a pre-registered
	// tunnel peer hits a nil regConn during ensureTunnel and panics.
	// Pre-registering a UDP addr lets ensureTunnel short-circuit; the
	// dial then fails fast on the missing socket, which is the path
	// these shims must surface to callers.
	d.AddTunnelPeer(0xBEEF, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1})
	d.AddTunnelPeer(0xCAFE, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1})

	hr := runtime.NewHandshakeRuntime(d)
	svc := handshake.NewService(hr)
	t.Cleanup(func() { svc.Manager().Stop() })

	adapter := runtime.NewHandshakeServiceAdapter(svc)

	// RejectHandshake on an absent peer: safe (delete from empty map +
	// publishEvent + best-effort registry / direct notification, both
	// of which surface clean errors rather than panicking now that a
	// tunnel peer entry exists).
	if err := adapter.RejectHandshake(0xBEEF, "not interested"); err != nil {
		t.Errorf("RejectHandshake: %v", err)
	}

	// SendRequest without a wired tunnel socket will fail at sendMessage
	// time, but the adapter delegation itself executes — that's the
	// SendRequest shim coverage we want.
	_ = adapter.SendRequest(0xCAFE, "want to chat")
}

// TestHandshakeRuntime_DialAndSend_SendErrorPath exercises the
// DialAndSend wrapper end-to-end against a daemon whose tunnel manager
// has a peer entry but no UDP socket. ensureTunnel returns nil (peer
// pre-registered via AddTunnelPeer), the dial proceeds to send a SYN,
// and the underlying routing.WriteFrame surfaces a clean error.
// That error propagates back through dialConnectionLocked →
// DialConnection → DialAndSend, which is the adapter contract we want
// covered.
func TestHandshakeRuntime_DialAndSend_SendErrorPath(t *testing.T) {
	t.Parallel()
	d := daemon.New(daemon.Config{})
	// Pre-register a peer so ensureTunnel takes the fast (no-regConn)
	// path. Any UDP addr works — we never reach the socket write.
	d.AddTunnelPeer(0x9999, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1})

	hr := runtime.NewHandshakeRuntime(d)

	err := hr.DialAndSend(0x9999, 12345, []byte("hello"))
	if err == nil {
		t.Fatal("DialAndSend: want error (no UDP socket), got nil")
	}
}

// TestDaemonStreams_Listen_BindErrorPropagates exercises the err
// branch of daemonStreams.Listen by binding the same port twice — the
// second Bind hits "port N already bound" from PortManager.Bind, which
// is the only deterministic failure mode for Listen.
func TestDaemonStreams_Listen_BindErrorPropagates(t *testing.T) {
	t.Parallel()
	d := daemon.New(daemon.Config{})
	rt := runtime.New(d)

	svc := &fakeService{name: "fake", order: 100}
	if err := rt.Register(svc); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := rt.StartPlugins(context.Background()); err != nil {
		t.Fatalf("StartPlugins: %v", err)
	}
	t.Cleanup(func() { _ = rt.StopPlugins(context.Background()) })

	streams := svc.gotDeps.Streams
	first, err := streams.Listen(54332)
	if err != nil {
		t.Fatalf("first Listen: %v", err)
	}
	t.Cleanup(func() { _ = first.Close() })

	second, err := streams.Listen(54332)
	if err == nil {
		_ = second.Close()
		t.Fatal("second Listen on same port: want error, got nil")
	}
	if second != nil {
		t.Errorf("Listen returned non-nil listener on error: %v", second)
	}
}

// TestHandshakeAndPolicy_TrustedPeers_NonEmptyLoop exercises the
// non-empty branch of both handshakeServiceAdapter.TrustedPeers (which
// otherwise sits at 80% — the for-range body is skipped when the slice
// is empty) and PolicyRuntime.TrustedPeers (same shape). The flow
// stages one trusted peer through the relayed-approval path, then
// asks both adapters to surface it.
func TestHandshakeAndPolicy_TrustedPeers_NonEmptyLoop(t *testing.T) {
	t.Parallel()
	d := daemon.New(daemon.Config{})
	hr := runtime.NewHandshakeRuntime(d)
	svc := handshake.NewService(hr)
	t.Cleanup(func() { svc.Manager().Stop() })

	hsAdapter := runtime.NewHandshakeServiceAdapter(svc)
	d.RegisterHandshakeService(hsAdapter)

	// Stage a trust record without going through the dial/SYN path —
	// the relayed-approval handler marks the peer trusted directly.
	svc.Manager().ProcessRelayedApproval(0xA55A)

	// Adapter-level read covers the for-range body of handshakeServiceAdapter.TrustedPeers.
	hsPeers := hsAdapter.TrustedPeers()
	if len(hsPeers) == 0 {
		t.Fatal("hsAdapter.TrustedPeers: want >=1 record after ProcessRelayedApproval")
	}
	found := false
	for _, p := range hsPeers {
		if p.NodeID == 0xA55A {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("hsAdapter.TrustedPeers missing node 0xA55A; got %+v", hsPeers)
	}

	// PolicyRuntime.TrustedPeers walks d.TrustedPeers (which dispatches
	// through the handshake adapter we just registered) and converts
	// the records to policy.TrustRecord — the same loop body that was
	// previously unreached.
	pr := runtime.NewPolicyRuntime(d)
	polPeers := pr.TrustedPeers()
	if len(polPeers) == 0 {
		t.Fatal("PolicyRuntime.TrustedPeers: want >=1 record, got 0")
	}
	if polPeers[0].NodeID != 0xA55A {
		t.Errorf("PolicyRuntime.TrustedPeers[0].NodeID = %x, want A55A", polPeers[0].NodeID)
	}
}

// fakeIsTrustedChecker satisfies daemon.TrustChecker for the
// HandshakeRuntime.IsTrusted happy-path test below.
type fakeIsTrustedChecker struct {
	want uint32
	name string
}

func (f *fakeIsTrustedChecker) IsTrusted(nodeID uint32) (string, bool) {
	if nodeID == f.want {
		return f.name, true
	}
	return "", false
}

// TestHandshakeRuntime_IsTrusted_DelegatesToRegisteredChecker covers the
// happy branch of HandshakeRuntime.IsTrusted — when a TrustChecker is
// registered on the daemon, the adapter must delegate to it instead of
// short-circuiting on the nil tc.
func TestHandshakeRuntime_IsTrusted_DelegatesToRegisteredChecker(t *testing.T) {
	t.Parallel()
	d := daemon.New(daemon.Config{})
	d.RegisterTrustChecker(&fakeIsTrustedChecker{want: 0xFADE, name: "bob"})

	hr := runtime.NewHandshakeRuntime(d)

	name, ok := hr.IsTrusted(0xFADE)
	if !ok || name != "bob" {
		t.Errorf("IsTrusted(FADE) with registered checker: got (%q, %v); want (bob, true)", name, ok)
	}

	// Sanity: a different ID still returns false through the same path.
	if name, ok := hr.IsTrusted(0xDEAD); ok || name != "" {
		t.Errorf("IsTrusted(DEAD): got (%q, %v); want (\"\", false)", name, ok)
	}
}

// TestDaemonEventBus_PublishAfterPluginStart covers the happy-path
// branch of daemonEventBus.Publish (the nil-daemon early returns are
// only reachable via direct construction inside the package and would
// only mask programming errors there). Subscribe is already covered by
// TestDaemonEventBus_PublishSubscribe in zz_deps_test.go; this case
// pins down the d != nil delivery side once more so the publish-path
// stays observable.
func TestDaemonEventBus_PublishAfterPluginStart(t *testing.T) {
	t.Parallel()
	d := daemon.New(daemon.Config{})
	rt := runtime.New(d)

	svc := &fakeService{name: "fake", order: 100}
	if err := rt.Register(svc); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := rt.StartPlugins(context.Background()); err != nil {
		t.Fatalf("StartPlugins: %v", err)
	}
	t.Cleanup(func() { _ = rt.StopPlugins(context.Background()) })

	// Publish should not panic even on a freshly-started daemon with no
	// subscribers — covers the d != nil branch of daemonEventBus.Publish.
	svc.gotDeps.Events.Publish("ceiling.test", map[string]any{"x": 1})
}
