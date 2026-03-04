package voicebox

import (
	"log/slog"
	"sync"
)

// DeviceState is server-side state tracking for one device session.
type DeviceState int

const (
	StateIdle DeviceState = iota
	StateListening
	StateSpeaking
)

func (s DeviceState) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateListening:
		return "listening"
	case StateSpeaking:
		return "speaking"
	default:
		return "unknown"
	}
}

// Session tracks mutable per-connection state.
type Session struct {
	mu         sync.Mutex
	state      DeviceState
	mcpEnabled bool
	aecEnabled bool
}

func NewSession(features *Features) *Session {
	s := &Session{state: StateIdle}
	if features != nil {
		s.mcpEnabled = features.MCP
		s.aecEnabled = features.AEC
	}
	return s
}

func (s *Session) State() DeviceState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

func (s *Session) MCPEnabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mcpEnabled
}

func (s *Session) AECEnabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.aecEnabled
}

// Transition validates and applies state transitions; invalid transitions are non-fatal.
func (s *Session) Transition(to DeviceState) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	from := s.state
	valid := false
	switch {
	case from == StateIdle && to == StateListening:
		valid = true
	case from == StateListening && to == StateSpeaking:
		valid = true
	case from == StateListening && to == StateIdle:
		valid = true
	case from == StateSpeaking && to == StateIdle:
		valid = true
	case from == StateSpeaking && to == StateListening:
		valid = true
	case to == StateIdle:
		valid = true
	}

	if !valid {
		slog.Warn("voicebox: invalid session transition", "from", from.String(), "to", to.String())
	}
	s.state = to
	return valid
}
