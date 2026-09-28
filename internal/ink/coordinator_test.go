package ink_test

import (
	"errors"
	"testing"

	"github.com/wyw14/solo-038/internal/ink"
)

func TestSwitchColorRollsBackWhenNextCircuitCannotOpen(t *testing.T) {
	state := ink.NewState("cyan", "magenta")
	circuit := ink.NewCircuit(state)
	coordinator := ink.NewCoordinator(circuit)

	if err := coordinator.Activate("cyan"); err != nil {
		t.Fatalf("activate cyan: %v", err)
	}
	// The next circuit is mid-drain, so it cannot be opened.
	state.Set("magenta", ink.CircuitDraining)

	transition, err := coordinator.SwitchColor("magenta")
	if err == nil {
		t.Fatal("expected switch to fail")
	}
	if transition.Drained || transition.Opened {
		t.Fatalf("rolled-back transition must report no drain and no open, got %+v", transition)
	}
	if active := coordinator.Active(); active != "cyan" {
		t.Fatalf("active color = %q, want cyan", active)
	}
	if got := state.Get("cyan"); got != ink.CircuitOpen {
		t.Fatalf("cyan circuit = %s, want open after rollback", got)
	}
	if !ink.ActiveCircuitReady(state, coordinator.Active()) {
		t.Fatal("active color circuit must be ready to print after rollback")
	}
}

func TestSwitchColorDrainFaultProtectionStillApplies(t *testing.T) {
	state := ink.NewState("cyan", "magenta")
	circuit := ink.NewCircuit(state)
	coordinator := ink.NewCoordinator(circuit)

	if err := coordinator.Activate("cyan"); err != nil {
		t.Fatalf("activate cyan: %v", err)
	}
	drainErr := errors.New("drain isolation signal failed")
	circuit.FailDrain("cyan", drainErr)

	if _, err := coordinator.SwitchColor("magenta"); !errors.Is(err, drainErr) {
		t.Fatalf("switch error = %v, want %v", err, drainErr)
	}
	if active := coordinator.Active(); active != "cyan" {
		t.Fatalf("active color = %q, want cyan", active)
	}
	if got := state.Get("cyan"); got != ink.CircuitOpen {
		t.Fatalf("cyan circuit = %s, want open after protected drain failure", got)
	}

	// The injected fault is consumed, so a retry switches cleanly.
	transition, err := coordinator.SwitchColor("magenta")
	if err != nil {
		t.Fatalf("retry switch: %v", err)
	}
	if !transition.Drained || !transition.Opened {
		t.Fatalf("successful transition = %+v, want drained and opened", transition)
	}
	if active := coordinator.Active(); active != "magenta" {
		t.Fatalf("active color = %q, want magenta", active)
	}
	if got := state.Get("cyan"); got != ink.CircuitIsolated {
		t.Fatalf("cyan circuit = %s, want isolated", got)
	}
	if !ink.ActiveCircuitReady(state, "magenta") {
		t.Fatal("magenta circuit must be ready to print")
	}
	if err := ink.RequireReady(state, "cyan"); err != nil {
		t.Fatalf("previous color must be isolated: %v", err)
	}
}
