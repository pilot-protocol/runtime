// SPDX-License-Identifier: AGPL-3.0-or-later

package runtime_test

import (
	"errors"
	"testing"

	"github.com/pilot-protocol/common/coreapi"
	"github.com/pilot-protocol/runtime"
)

// fakePolicyRunner is a coreapi.PolicyRunner whose methods record their
// invocations so we can assert the adapter delegates correctly.
type fakePolicyRunner struct {
	netID      uint16
	hasMember  bool
	gateOK     bool
	gateCalls  int
	actionCh   string
	statusMap  map[string]any
	peerList   []map[string]any
	forceCycle map[string]any
	reconciles int
	policyJSON []byte
	policyErr  error
	stopped    bool
}

func (r *fakePolicyRunner) NetworkID() uint16              { return r.netID }
func (r *fakePolicyRunner) HasMember(uint32) bool          { return r.hasMember }
func (r *fakePolicyRunner) EvaluatePortGate(eventType string, port uint16, peerNodeID uint32, payloadSize int, direction string, localTags, nodeInfoTags []string) bool {
	r.gateCalls++
	return r.gateOK
}
func (r *fakePolicyRunner) EvaluateActions(eventType string, _ map[string]any) {
	r.actionCh = eventType
}
func (r *fakePolicyRunner) Status() map[string]any        { return r.statusMap }
func (r *fakePolicyRunner) PeerList() []map[string]any    { return r.peerList }
func (r *fakePolicyRunner) ForceCycle() map[string]any    { return r.forceCycle }
func (r *fakePolicyRunner) ReconcileNow()                 { r.reconciles++ }
func (r *fakePolicyRunner) PolicyJSON() ([]byte, error)   { return r.policyJSON, r.policyErr }
func (r *fakePolicyRunner) Stop()                         { r.stopped = true }

// fakePolicyManager records calls and returns canned PolicyRunner values.
type fakePolicyManager struct {
	startNetID    uint16
	startJSON     []byte
	startErr      error
	startedRunner *fakePolicyRunner

	stopCalls    []uint16
	getReturns   coreapi.PolicyRunner
	allReturns   []coreapi.PolicyRunner
	stopAllCount int
	loadErr      error
	loadCalls    int
}

func (m *fakePolicyManager) Start(netID uint16, policyJSON []byte) (coreapi.PolicyRunner, error) {
	m.startNetID = netID
	m.startJSON = policyJSON
	if m.startErr != nil {
		return nil, m.startErr
	}
	return m.startedRunner, nil
}
func (m *fakePolicyManager) Stop(netID uint16)         { m.stopCalls = append(m.stopCalls, netID) }
func (m *fakePolicyManager) Get(netID uint16) coreapi.PolicyRunner { return m.getReturns }
func (m *fakePolicyManager) All() []coreapi.PolicyRunner           { return m.allReturns }
func (m *fakePolicyManager) StopAll()                              { m.stopAllCount++ }
func (m *fakePolicyManager) LoadPersisted() error {
	m.loadCalls++
	return m.loadErr
}

// TestAsDaemonPolicyManager_FullDelegation drives every method on the
// policyManagerAdapter and runnerAdapter through fakes.
func TestAsDaemonPolicyManager_FullDelegation(t *testing.T) {
	t.Parallel()
	runner := &fakePolicyRunner{
		netID:      0xABCD,
		hasMember:  true,
		gateOK:     false,
		statusMap:  map[string]any{"ok": true},
		peerList:   []map[string]any{{"peer": 1}},
		forceCycle: map[string]any{"cycled": true},
		policyJSON: []byte(`{"v":1}`),
	}
	pm := &fakePolicyManager{
		startedRunner: runner,
		getReturns:    runner,
		allReturns:    []coreapi.PolicyRunner{runner},
	}
	adapter := runtime.AsDaemonPolicyManager(pm)
	if adapter == nil {
		t.Fatal("adapter is nil")
	}

	// Start happy-path.
	pr, err := adapter.Start(0xABCD, []byte(`{"x":1}`))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if pm.startNetID != 0xABCD || string(pm.startJSON) != `{"x":1}` {
		t.Errorf("Start args = (%v, %s); want (0xABCD, {\"x\":1})", pm.startNetID, pm.startJSON)
	}
	if pr.NetworkID() != 0xABCD {
		t.Errorf("NetworkID = %x, want ABCD", pr.NetworkID())
	}
	if !pr.HasMember(7) {
		t.Errorf("HasMember: want true")
	}
	if pr.EvaluatePortGate("dial", 80, 0xCAFE, 0, "out", []string{"a"}, []string{"b"}) {
		t.Errorf("EvaluatePortGate: want false")
	}
	pr.EvaluateActions("cycle", map[string]any{})
	if runner.actionCh != "cycle" {
		t.Errorf("EvaluateActions not delegated, got %q", runner.actionCh)
	}
	if got := pr.Status()["ok"]; got != true {
		t.Errorf("Status = %v, want ok:true", pr.Status())
	}
	if got := pr.PeerList(); len(got) != 1 {
		t.Errorf("PeerList len = %d, want 1", len(got))
	}
	if got := pr.ForceCycle()["cycled"]; got != true {
		t.Errorf("ForceCycle = %v", pr.ForceCycle())
	}
	pr.ReconcileNow()
	if runner.reconciles != 1 {
		t.Errorf("ReconcileNow count = %d, want 1", runner.reconciles)
	}
	pj, err := pr.PolicyJSON()
	if err != nil || string(pj) != `{"v":1}` {
		t.Errorf("PolicyJSON = (%s, %v)", pj, err)
	}
	pr.Stop()
	if !runner.stopped {
		t.Errorf("Stop not delegated")
	}

	// Get delegates.
	if got := adapter.Get(0xABCD); got.NetworkID() != 0xABCD {
		t.Errorf("Get: NetworkID = %x, want ABCD", got.NetworkID())
	}

	// All delegates.
	if got := adapter.All(); len(got) != 1 {
		t.Errorf("All len = %d, want 1", len(got))
	}

	// Stop / StopAll / LoadPersisted delegate.
	adapter.Stop(0xABCD)
	if len(pm.stopCalls) != 1 || pm.stopCalls[0] != 0xABCD {
		t.Errorf("Stop calls = %v", pm.stopCalls)
	}
	adapter.StopAll()
	if pm.stopAllCount != 1 {
		t.Errorf("StopAll count = %d, want 1", pm.stopAllCount)
	}
	if err := adapter.LoadPersisted(); err != nil {
		t.Errorf("LoadPersisted: %v", err)
	}
	if pm.loadCalls != 1 {
		t.Errorf("LoadPersisted calls = %d, want 1", pm.loadCalls)
	}
}

// TestAsDaemonPolicyManager_StartErrorPropagates verifies the Start
// shim surfaces the underlying error without wrapping a runner.
func TestAsDaemonPolicyManager_StartErrorPropagates(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("boom")
	pm := &fakePolicyManager{startErr: wantErr}
	adapter := runtime.AsDaemonPolicyManager(pm)

	pr, err := adapter.Start(1, []byte(`{}`))
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
	if pr != nil {
		t.Errorf("runner = %v, want nil on error", pr)
	}
}

// TestAsDaemonPolicyManager_NilRunnerPaths checks wrapRunner's nil
// short-circuits via Get / All returning empty / nil.
func TestAsDaemonPolicyManager_NilRunnerPaths(t *testing.T) {
	t.Parallel()
	pm := &fakePolicyManager{getReturns: nil, allReturns: nil}
	adapter := runtime.AsDaemonPolicyManager(pm)

	if got := adapter.Get(7); got != nil {
		t.Errorf("Get(nil-returning) = %v, want nil", got)
	}
	if got := adapter.All(); len(got) != 0 {
		t.Errorf("All(empty) len = %d, want 0", len(got))
	}
}
