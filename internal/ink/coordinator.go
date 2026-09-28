package ink

import (
	"fmt"
	"sync"
)

type Transition struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Drained bool   `json:"drained"`
	Opened  bool   `json:"opened"`
}

type Coordinator struct {
	mu      sync.Mutex
	circuit *Circuit
	active  string
}

func NewCoordinator(circuit *Circuit) *Coordinator {
	return &Coordinator{circuit: circuit}
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
	if err := coordinator.circuit.Drain(coordinator.active); err != nil {
		return transition, err
	}
	if err := coordinator.circuit.Open(next); err != nil {
		// The new circuit cannot be opened, so roll back the drain: reopen
		// the circuit that was just isolated. Otherwise the active color
		// would keep pointing at an isolated circuit and printing would
		// continue against a state the readiness checks reject.
		if rollbackErr := coordinator.circuit.Open(coordinator.active); rollbackErr != nil {
			coordinator.active = ""
			return transition, fmt.Errorf("open ink circuit %s: %w (rollback of %s failed: %v)", next, err, transition.From, rollbackErr)
		}
		return transition, err
	}
	transition.Drained = true
	transition.Opened = true
	coordinator.active = next
	return transition, nil
}

func (coordinator *Coordinator) Active() string {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	return coordinator.active
}
