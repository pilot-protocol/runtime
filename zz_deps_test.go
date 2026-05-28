// SPDX-License-Identifier: AGPL-3.0-or-later

package runtime_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/TeoSlayer/pilotprotocol/pkg/coreapi"
	"github.com/TeoSlayer/pilotprotocol/pkg/daemon"
	"github.com/pilot-protocol/runtime"
)

// fakeService is a Service that captures the Deps it receives in Start
// so tests can exercise the adapter implementations runtime.New wires.
type fakeService struct {
	name     string
	order    int
	started  bool
	stopped  bool
	gotDeps  coreapi.Deps
	startErr error
	stopErr  error
}

func (s *fakeService) Name() string  { return s.name }
func (s *fakeService) Order() int    { return s.order }
func (s *fakeService) Start(_ context.Context, deps coreapi.Deps) error {
	s.gotDeps = deps
	s.started = true
	return s.startErr
}
func (s *fakeService) Stop(_ context.Context) error {
	s.stopped = true
	return s.stopErr
}

// TestRuntime_StartPlugins_CapturesDeps confirms the Runtime wires
// daemonStreams / daemonIdentity / daemonEventBus into Deps for plugins.
func TestRuntime_StartPlugins_CapturesDeps(t *testing.T) {
	t.Parallel()
	d := daemon.New(daemon.Config{})
	rt := runtime.New(d)

	svc := &fakeService{name: "fake", order: 100}
	if err := rt.Register(svc); err != nil {
		t.Fatalf("Register: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := rt.StartPlugins(ctx); err != nil {
		t.Fatalf("StartPlugins: %v", err)
	}
	if !svc.started {
		t.Fatal("plugin Start was not called")
	}

	deps := svc.gotDeps
	if deps.Streams == nil {
		t.Error("Deps.Streams is nil")
	}
	if deps.Identity == nil {
		t.Error("Deps.Identity is nil")
	}
	if deps.Events == nil {
		t.Error("Deps.Events is nil")
	}

	// Exercise daemonIdentity adapter through Deps.Identity.
	if got := deps.Identity.NodeID(); got != 0 {
		t.Errorf("Identity.NodeID = %d, want 0", got)
	}
	addr := deps.Identity.Address()
	if addr.Network != 0 || addr.Node != 0 {
		t.Errorf("Identity.Address = %+v, want zero", addr)
	}
	if pk := deps.Identity.PublicKey(); pk != nil {
		t.Errorf("Identity.PublicKey = %v, want nil (no identity)", pk)
	}
	if sig, err := deps.Identity.Sign([]byte("hi")); err == nil || sig != nil {
		t.Errorf("Identity.Sign with no identity: got (%v, %v), want (nil, error)", sig, err)
	}

	// Exercise daemonEventBus adapter through Deps.Events.
	// Publish should be safe with a real bus on the daemon.
	deps.Events.Publish("test.evt", map[string]any{"k": "v"})
	ch, cancelSub := deps.Events.Subscribe("test.*")
	if ch == nil {
		t.Fatal("Events.Subscribe returned nil channel")
	}
	cancelSub()

	// Exercise daemonStreams adapter through Deps.Streams.
	ln, err := deps.Streams.Listen(54322)
	if err != nil {
		t.Fatalf("Streams.Listen: %v", err)
	}
	if ln == nil {
		t.Fatal("Listener is nil")
	}
	if ln.Port() != 54322 {
		t.Errorf("Listener.Port = %d, want 54322", ln.Port())
	}
	// Addr on listener delegates to daemon.Addr — zero on fresh daemon.
	_ = ln.Addr()
	if err := ln.Close(); err != nil {
		t.Errorf("Listener.Close: %v", err)
	}

	// Now Stop everything.
	if err := rt.StopPlugins(context.Background()); err != nil {
		t.Errorf("StopPlugins: %v", err)
	}
	if !svc.stopped {
		t.Error("plugin Stop was not called")
	}
}

// TestRuntime_StartPlugins_PropagatesError verifies a failing Start
// short-circuits and the error reaches the caller.
func TestRuntime_StartPlugins_PropagatesError(t *testing.T) {
	t.Parallel()
	d := daemon.New(daemon.Config{})
	rt := runtime.New(d)

	wantErr := errors.New("synthetic start failure")
	svc := &fakeService{name: "broken", order: 50, startErr: wantErr}
	if err := rt.Register(svc); err != nil {
		t.Fatalf("Register: %v", err)
	}

	err := rt.StartPlugins(context.Background())
	if !errors.Is(err, wantErr) {
		t.Errorf("StartPlugins err = %v, want %v", err, wantErr)
	}
}

// TestDaemonEventBus_PublishSubscribe exercises the bus adapter end-to-end:
// the subscriber goroutine should forward a daemon.Event to a coreapi.Event.
func TestDaemonEventBus_PublishSubscribe(t *testing.T) {
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

	bus := svc.gotDeps.Events
	ch, cancel := bus.Subscribe("hello.*")
	defer cancel()

	// Publish before we read — the bus is buffered.
	bus.Publish("hello.world", map[string]any{"x": 42})

	select {
	case ev := <-ch:
		if ev.Topic != "hello.world" {
			t.Errorf("Topic = %q, want hello.world", ev.Topic)
		}
		if v, _ := ev.Payload["x"].(int); v != 42 {
			t.Errorf("Payload[x] = %v, want 42", ev.Payload["x"])
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("event not delivered within timeout")
	}
}
