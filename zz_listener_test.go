// SPDX-License-Identifier: AGPL-3.0-or-later

package runtime_test

import (
	"testing"
	"time"

	"github.com/pilot-protocol/common/coreapi"
	"github.com/TeoSlayer/pilotprotocol/pkg/daemon"
	"github.com/pilot-protocol/runtime"
)

// TestDaemonListener_AcceptAfterCloseReturnsError drives the
// listener-closed branch of Accept.
func TestDaemonListener_AcceptAfterCloseReturnsError(t *testing.T) {
	t.Parallel()
	d := daemon.New(daemon.Config{})
	rt := runtime.New(d)

	svc := &fakeService{name: "fake", order: 100}
	if err := rt.Register(svc); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := rt.StartPlugins(nil); err != nil {
		t.Fatalf("StartPlugins: %v", err)
	}
	t.Cleanup(func() { _ = rt.StopPlugins(nil) })

	streams := svc.gotDeps.Streams
	ln, err := streams.Listen(54323)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}

	// Close the listener — Accept should now return an error.
	_ = ln.Close()

	acceptDone := make(chan acceptResult, 1)
	go func() {
		_, err := ln.Accept()
		acceptDone <- acceptResult{err: err}
	}()

	select {
	case r := <-acceptDone:
		if r.err == nil {
			t.Error("Accept after Close: want error")
		}
	case <-time.After(time.Second):
		t.Fatal("Accept blocked after Close")
	}
}

type acceptResult struct {
	stream coreapi.Stream
	err    error
}

// TestDaemonListener_PortAccessor exercises the port shim.
func TestDaemonListener_PortAccessor(t *testing.T) {
	t.Parallel()
	d := daemon.New(daemon.Config{})
	rt := runtime.New(d)

	svc := &fakeService{name: "fake", order: 100}
	if err := rt.Register(svc); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := rt.StartPlugins(nil); err != nil {
		t.Fatalf("StartPlugins: %v", err)
	}
	t.Cleanup(func() { _ = rt.StopPlugins(nil) })

	streams := svc.gotDeps.Streams
	ln, err := streams.Listen(54324)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()
	if got := ln.Port(); got != 54324 {
		t.Errorf("Port = %d, want 54324", got)
	}
}
