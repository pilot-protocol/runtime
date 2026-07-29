// SPDX-License-Identifier: AGPL-3.0-or-later

package runtime

import (
	"context"
	"errors"
	"time"

	"github.com/pilot-protocol/common/coreapi"
	"github.com/pilot-protocol/common/daemonapi"
)

// daemonStreams is the in-process implementation of coreapi.Streams.
// Lives here (plugins/runtime) so daemon doesn't carry the L10 import.

type daemonStreams struct{ d daemonapi.Daemon }

func (s daemonStreams) Listen(port uint16) (coreapi.Listener, error) {
	ln, err := s.d.Ports().Bind(port)
	if err != nil {
		return nil, err
	}
	return &daemonListener{inner: ln, d: s.d}, nil
}

func (s daemonStreams) Dial(ctx context.Context, dst coreapi.Addr, port uint16) (coreapi.Stream, error) {
	conn, err := s.d.DialConnectionContext(ctx, dst, port)
	if err != nil {
		return nil, err
	}
	return newStreamAdapter(s.d, conn), nil
}

func (s daemonStreams) SendDatagram(ctx context.Context, dst coreapi.Addr, port uint16, data []byte) error {
	return s.d.SendDatagram(dst, port, data)
}

type daemonListener struct {
	inner daemonapi.Listener
	d     daemonapi.Daemon
}

func (l *daemonListener) Accept() (coreapi.Stream, error) {
	conn, ok := l.inner.Accept()
	if !ok {
		return nil, errors.New("listener closed")
	}
	return newStreamAdapter(l.d, conn), nil
}

func (l *daemonListener) Close() error {
	l.d.Ports().Unbind(l.inner.Port())
	return nil
}

func (l *daemonListener) Addr() coreapi.Addr { return l.d.Addr() }
func (l *daemonListener) Port() uint16       { return l.inner.Port() }

// streamAdapter wraps daemonapi.Connection so it satisfies coreapi.Stream.
// Daemon exposes a connAdapter via Daemon.NewConnAdapter for this.
type streamAdapter struct {
	d    daemonapi.Daemon
	conn daemonapi.Connection
	rw   daemonapi.ConnReadWriter
}

func newStreamAdapter(d daemonapi.Daemon, conn daemonapi.Connection) *streamAdapter {
	return &streamAdapter{d: d, conn: conn, rw: d.NewConnReadWriter(conn)}
}

func (s *streamAdapter) Read(p []byte) (int, error)  { return s.rw.Read(p) }
func (s *streamAdapter) Write(p []byte) (int, error) { return s.rw.Write(p) }
func (s *streamAdapter) Close() error                { return s.rw.Close() }

// SetReadDeadline forwards to the underlying ConnReadWriter when it
// supports deadlines.
//
// This method existing at all is the point. Frame readers (notably
// dataexchange) gate their slowloris/idle teardown on a
// `conn.(interface{ SetReadDeadline(time.Time) error })` assertion.
// streamAdapter did not implement it, so that assertion failed for every
// in-daemon plugin connection and the configured idle timeout was never
// applied — a peer that opened a stream and stalled held its handler
// goroutine and the whole underlying connection until the process died.
//
// Reported as nil (no-op success) rather than an error when the transport
// cannot do deadlines, so callers that treat a failure as fatal are not
// broken by transports that legitimately lack the capability.
func (s *streamAdapter) SetReadDeadline(t time.Time) error {
	if d, ok := s.rw.(interface{ SetReadDeadline(time.Time) error }); ok {
		return d.SetReadDeadline(t)
	}
	return nil
}

func (s *streamAdapter) LocalAddr() coreapi.Addr  { return s.conn.Info().LocalAddr }
func (s *streamAdapter) LocalPort() uint16        { return s.conn.Info().LocalPort }
func (s *streamAdapter) RemoteAddr() coreapi.Addr { return s.conn.Info().RemoteAddr }
func (s *streamAdapter) RemotePort() uint16       { return s.conn.Info().RemotePort }



var (
	_ coreapi.Streams  = daemonStreams{}
	_ coreapi.Listener = (*daemonListener)(nil)
	_ coreapi.Stream   = (*streamAdapter)(nil)

	// Frame readers (dataexchange) gate their idle/slowloris teardown on
	// this exact optional interface. When streamAdapter stopped satisfying
	// it, the assertion silently fell through to "no deadline" and stalled
	// peers pinned a goroutine plus a full Connection forever — which is
	// what OOM-killed the 431-agent service fleet on 2026-07-28. Assert it
	// at compile time so the capability cannot be dropped again silently.
	_ interface{ SetReadDeadline(time.Time) error } = (*streamAdapter)(nil)
)
