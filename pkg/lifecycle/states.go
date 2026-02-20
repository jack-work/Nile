package lifecycle

import "fmt"

// State represents the lifecycle state of a copt.
type State int

const (
	StateCreated State = iota
	StateStarting
	StateIdle
	StateProcessing
	StatePostProcessing
	StateDraining
	StateRetaining
	StateStopping
	StateStopped
	StateFailed
)

var stateNames = [...]string{
	"created",
	"starting",
	"idle",
	"processing",
	"post_processing",
	"draining",
	"retaining",
	"stopping",
	"stopped",
	"failed",
}

func (s State) String() string {
	if int(s) < len(stateNames) {
		return stateNames[s]
	}
	return fmt.Sprintf("state(%d)", s)
}

// validTransitions defines which state transitions are allowed.
var validTransitions = map[State][]State{
	StateCreated:        {StateStarting},
	StateStarting:       {StateIdle, StateFailed},
	StateIdle:           {StateProcessing, StateDraining, StateStopping},
	StateProcessing:     {StatePostProcessing, StateIdle},
	StatePostProcessing: {StateIdle},
	StateDraining:       {StateRetaining},
	StateRetaining:      {StateIdle, StateFailed},
	StateStopping:       {StateStopped},
	StateFailed:         {StateStarting},
}

func canTransition(from, to State) bool {
	for _, allowed := range validTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}
