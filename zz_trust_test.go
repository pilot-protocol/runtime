// SPDX-License-Identifier: AGPL-3.0-or-later

package runtime

import (
	"testing"

	"github.com/TeoSlayer/pilotprotocol/pkg/daemon"
)

// fakeTrustChecker implements daemon.TrustChecker.
type fakeTrustChecker struct {
	want uint32
	name string
}

func (f *fakeTrustChecker) IsTrusted(nodeID uint32) (string, bool) {
	if nodeID == f.want {
		return f.name, true
	}
	return "", false
}

// TestAsCoreapiTrust_NilInputReturnsNil covers the nil branch.
func TestAsCoreapiTrust_NilInputReturnsNil(t *testing.T) {
	t.Parallel()
	if got := asCoreapiTrust(nil); got != nil {
		t.Errorf("nil input: got %v, want nil", got)
	}
}

// TestAsCoreapiTrust_DelegatesToInner covers the happy bridge path.
func TestAsCoreapiTrust_DelegatesToInner(t *testing.T) {
	t.Parallel()
	var inner daemon.TrustChecker = &fakeTrustChecker{want: 0xCAFE, name: "alice"}
	got := asCoreapiTrust(inner)
	if got == nil {
		t.Fatal("expected non-nil adapter")
	}
	if name, ok := got.IsTrusted(0xCAFE); !ok || name != "alice" {
		t.Errorf("IsTrusted(CAFE) = (%q, %v); want (alice, true)", name, ok)
	}
	if _, ok := got.IsTrusted(0xDEAD); ok {
		t.Error("IsTrusted(DEAD): want false")
	}
}
