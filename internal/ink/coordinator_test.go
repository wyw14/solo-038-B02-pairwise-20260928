package ink

import (
	"errors"
	"strings"
	"testing"
)

func newTestCoordinator(t *testing.T) (*Coordinator, *State) {
	t.Helper()
	state := NewState("cyan", "magenta", "yellow", "black")
	coordinator := NewCoordinator(NewCircuit(state))
	if err := coordinator.Activate("cyan"); err != nil {
		t.Fatalf("activate cyan: %v", err)
	}
	return coordinator, state
}

func TestSwitchColorSuccess(t *testing.T) {
	coordinator, state := newTestCoordinator(t)

	transition, err := coordinator.SwitchColor("magenta")
	if err != nil {
		t.Fatalf("switch color: %v", err)
	}
	if transition.From != "cyan" || transition.To != "magenta" ||
		!transition.Drained || !transition.Opened || transition.Reverted {
		t.Fatalf("unexpected transition: %+v", transition)
	}
	if coordinator.Active() != "magenta" {
		t.Fatalf("active = %q, want magenta", coordinator.Active())
	}
	if !coordinator.Ready() {
		t.Fatal("coordinator not ready after successful switch")
	}
	if got := state.Get("cyan"); got != CircuitIsolated {
		t.Fatalf("cyan circuit = %s, want isolated", got)
	}
	if got := state.Get("magenta"); got != CircuitOpen {
		t.Fatalf("magenta circuit = %s, want open", got)
	}
	if !ActiveCircuitReady(state, "magenta") {
		t.Fatal("ActiveCircuitReady(magenta) = false, want true")
	}
	if err := RequireReady(state, "cyan"); err != nil {
		t.Fatalf("RequireReady(cyan): %v", err)
	}
}

func TestSwitchColorDrainFailureKeepsOldColorActive(t *testing.T) {
	coordinator, state := newTestCoordinator(t)
	drainErr := errors.New("drain isolation signal failed")
	coordinator.FailDrain("cyan", drainErr)

	_, err := coordinator.SwitchColor("magenta")
	if !errors.Is(err, drainErr) {
		t.Fatalf("switch error = %v, want drain failure", err)
	}
	if coordinator.Active() != "cyan" {
		t.Fatalf("active = %q, want cyan after drain failure", coordinator.Active())
	}
	if !coordinator.Ready() {
		t.Fatal("coordinator must stay ready: cyan circuit is reopened by drain protection")
	}
	if got := state.Get("cyan"); got != CircuitOpen {
		t.Fatalf("cyan circuit = %s, want open after failed drain", got)
	}
	if got := state.Get("magenta"); got != CircuitClosed {
		t.Fatalf("magenta circuit = %s, want closed (never opened)", got)
	}
}

func TestSwitchColorOpenFailureRollsBack(t *testing.T) {
	coordinator, state := newTestCoordinator(t)
	openErr := errors.New("magenta route valve did not open")
	coordinator.FailOpen("magenta", openErr)

	transition, err := coordinator.SwitchColor("magenta")
	if !errors.Is(err, openErr) {
		t.Fatalf("switch error = %v, want wrapped open failure", err)
	}
	if !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("error %q should report rollback", err.Error())
	}
	if !transition.Drained || transition.Opened || !transition.Reverted {
		t.Fatalf("unexpected transition: %+v", transition)
	}
	// The reported active color must match a circuit that is actually open.
	if coordinator.Active() != "cyan" {
		t.Fatalf("active = %q, want cyan after rollback", coordinator.Active())
	}
	if !coordinator.Ready() {
		t.Fatal("coordinator must be ready after successful rollback to cyan")
	}
	if got := state.Get("cyan"); got != CircuitOpen {
		t.Fatalf("cyan circuit = %s, want open after rollback", got)
	}
	if got := state.Get("magenta"); got != CircuitClosed {
		t.Fatalf("magenta circuit = %s, want closed after open failure", got)
	}
}

func TestSwitchColorRetriesAfterRollback(t *testing.T) {
	coordinator, state := newTestCoordinator(t)
	coordinator.FailOpen("magenta", errors.New("transient valve fault"))

	if _, err := coordinator.SwitchColor("magenta"); err == nil {
		t.Fatal("expected first switch to fail")
	}
	if coordinator.Active() != "cyan" || !coordinator.Ready() {
		t.Fatal("coordinator not restored to cyan after failed switch")
	}

	// The open fault is one-shot; after rollback the same switch must succeed.
	transition, err := coordinator.SwitchColor("magenta")
	if err != nil {
		t.Fatalf("retry switch: %v", err)
	}
	if !transition.Opened || transition.Reverted {
		t.Fatalf("unexpected retry transition: %+v", transition)
	}
	if coordinator.Active() != "magenta" || !coordinator.Ready() {
		t.Fatalf("post-retry state: active=%q ready=%v", coordinator.Active(), coordinator.Ready())
	}
	if got := state.Get("cyan"); got != CircuitIsolated {
		t.Fatalf("cyan circuit = %s, want isolated", got)
	}
	if got := state.Get("magenta"); got != CircuitOpen {
		t.Fatalf("magenta circuit = %s, want open", got)
	}
}

func TestSwitchColorInvalidInputs(t *testing.T) {
	coordinator, _ := newTestCoordinator(t)
	for _, next := range []string{"", "cyan"} {
		if _, err := coordinator.SwitchColor(next); err == nil {
			t.Fatalf("SwitchColor(%q) error = nil, want invalid transition", next)
		}
	}
	if coordinator.Active() != "cyan" || !coordinator.Ready() {
		t.Fatal("invalid switch must leave active color untouched")
	}
}

// brokenRestoreCircuit drains and opens like a real circuit but fails Restore,
// simulating the compound fault where the new route cannot open and the old
// route cannot be brought back either.
type brokenRestoreCircuit struct {
	open map[string]bool
}

func (circuit *brokenRestoreCircuit) Drain(color string) error {
	circuit.open[color] = false
	return nil
}

func (circuit *brokenRestoreCircuit) Open(color string) error {
	return errors.New("magenta route blocked")
}

func (circuit *brokenRestoreCircuit) Restore(color string) error {
	return errors.New("cyan restore valve stuck")
}

func (circuit *brokenRestoreCircuit) IsOpen(color string) bool {
	return circuit.open[color]
}

func TestSwitchColorRollbackFailureClearsActive(t *testing.T) {
	backend := &brokenRestoreCircuit{open: map[string]bool{"cyan": true}}
	coordinator := &Coordinator{circuit: backend, active: "cyan"}

	transition, err := coordinator.SwitchColor("magenta")
	if err == nil {
		t.Fatal("expected switch error when rollback fails")
	}
	if !strings.Contains(err.Error(), "rollback") {
		t.Fatalf("error %q should mention rollback failure", err.Error())
	}
	if !transition.Drained || transition.Opened || transition.Reverted {
		t.Fatalf("unexpected transition: %+v", transition)
	}
	if coordinator.Active() != "" {
		t.Fatalf("active = %q, want empty when no circuit is open", coordinator.Active())
	}
	if coordinator.Ready() {
		t.Fatal("coordinator must not be ready when no color is active")
	}
}
