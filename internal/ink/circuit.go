package ink

import (
	"fmt"
	"sync"
)

type Circuit struct {
	mu        sync.Mutex
	state     *State
	drainFail map[string]error
	openFail  map[string]error
}

func NewCircuit(state *State) *Circuit {
	return &Circuit{state: state, drainFail: map[string]error{}, openFail: map[string]error{}}
}

func (circuit *Circuit) FailDrain(color string, err error) {
	circuit.mu.Lock()
	defer circuit.mu.Unlock()
	circuit.drainFail[color] = err
}

func (circuit *Circuit) FailOpen(color string, err error) {
	circuit.mu.Lock()
	defer circuit.mu.Unlock()
	circuit.openFail[color] = err
}

func (circuit *Circuit) Drain(color string) error {
	circuit.mu.Lock()
	defer circuit.mu.Unlock()
	if circuit.state.Get(color) != CircuitOpen {
		return fmt.Errorf("ink circuit %s is not open", color)
	}
	circuit.state.Set(color, CircuitDraining)
	if err := circuit.drainFail[color]; err != nil {
		delete(circuit.drainFail, color)
		circuit.state.Set(color, CircuitOpen)
		return err
	}
	circuit.state.Set(color, CircuitIsolated)
	return nil
}

func (circuit *Circuit) Open(color string) error {
	circuit.mu.Lock()
	defer circuit.mu.Unlock()
	current := circuit.state.Get(color)
	if current != CircuitClosed && current != CircuitIsolated {
		return fmt.Errorf("ink circuit %s cannot open from %s", color, current)
	}
	if err := circuit.openFail[color]; err != nil {
		delete(circuit.openFail, color)
		return err
	}
	circuit.state.Set(color, CircuitOpen)
	return nil
}

// Restore reopens a circuit that Drain isolated. It is the rollback path for a
// color switch that fails after the active circuit has been drained, so the
// reported active color never names a circuit that is not actually open.
func (circuit *Circuit) Restore(color string) error {
	circuit.mu.Lock()
	defer circuit.mu.Unlock()
	if circuit.state.Get(color) != CircuitIsolated {
		return fmt.Errorf("ink circuit %s cannot restore from %s", color, circuit.state.Get(color))
	}
	circuit.state.Set(color, CircuitOpen)
	return nil
}

func (circuit *Circuit) IsOpen(color string) bool {
	circuit.mu.Lock()
	defer circuit.mu.Unlock()
	return circuit.state.Get(color) == CircuitOpen
}
