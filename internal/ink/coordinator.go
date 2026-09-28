package ink

import (
	"fmt"
	"sync"
)

type Transition struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Drained  bool   `json:"drained"`
	Opened   bool   `json:"opened"`
	Reverted bool   `json:"reverted"`
}

// circuitBackend is the subset of Circuit the coordinator drives. Keeping it
// as an interface lets fault tests simulate compound failures (open fails and
// rollback cannot restore) without mutating shared circuit state by hand.
type circuitBackend interface {
	Drain(color string) error
	Open(color string) error
	Restore(color string) error
	IsOpen(color string) bool
}

type Coordinator struct {
	mu      sync.Mutex
	circuit circuitBackend
	active  string
}

func NewCoordinator(circuit *Circuit) *Coordinator {
	return &Coordinator{circuit: circuit}
}

// FailDrain arms a one-shot drain fault for color on the circuit owned by this
// coordinator. Fault injection must target the coordinator's own circuit,
// otherwise the fault never reaches the live switch path.
func (coordinator *Coordinator) FailDrain(color string, err error) {
	if circuit, ok := coordinator.circuit.(*Circuit); ok {
		circuit.FailDrain(color, err)
	}
}

// FailOpen arms a one-shot open fault for color on the circuit owned by this
// coordinator, simulating an ink route that cannot be opened.
func (coordinator *Coordinator) FailOpen(color string, err error) {
	if circuit, ok := coordinator.circuit.(*Circuit); ok {
		circuit.FailOpen(color, err)
	}
}

func (coordinator *Coordinator) Activate(color string) error {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	if coordinator.active != "" {
		return fmt.Errorf("an ink color is already active")
	}
	if err := coordinator.circuit.Open(color); err != nil {
		return err
	}
	coordinator.active = color
	return nil
}

func (coordinator *Coordinator) SwitchColor(next string) (Transition, error) {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	transition := Transition{From: coordinator.active, To: next}
	if coordinator.active == "" || next == "" || next == coordinator.active {
		return transition, fmt.Errorf("ink color transition is invalid")
	}
	previous := coordinator.active
	if err := coordinator.circuit.Drain(previous); err != nil {
		// Drain failed: the previous circuit is open again and remains active.
		return transition, err
	}
	transition.Drained = true
	if err := coordinator.circuit.Open(next); err != nil {
		// The new route cannot open while the previous one is already isolated.
		// Roll back so the reported active color matches an open circuit.
		if restoreErr := coordinator.circuit.Restore(previous); restoreErr != nil {
			coordinator.active = ""
			return transition, fmt.Errorf("ink switch to %s failed: %w; rollback to %s also failed: %v", next, err, previous, restoreErr)
		}
		transition.Reverted = true
		return transition, fmt.Errorf("ink switch to %s failed, rolled back to %s: %w", next, previous, err)
	}
	transition.Opened = true
	coordinator.active = next
	return transition, nil
}

func (coordinator *Coordinator) Active() string {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	return coordinator.active
}

// Ready reports whether printing may proceed: an active color is named and its
// circuit is actually open. It is the single authority gating print readiness,
// so the active color, circuit state and readiness can never disagree.
func (coordinator *Coordinator) Ready() bool {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	return coordinator.active != "" && coordinator.circuit.IsOpen(coordinator.active)
}
