// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package nodestate is the in-process bootstrap state machine and
// the advertisement payload published to peer discovery.
//
// A node moves through three states with explicit trust semantics:
//
//   - BOOTING: pulling state and walking headers. Cannot serve
//     queries reliably; cannot validate.
//   - ACTIVE:  local BPT root matched the verified header anchor;
//     can serve current-state queries and (with the rest of the
//     node stack) participate in consensus.
//   - COMPLETE: ACTIVE plus historical backfill complete (or
//     historical retention policy satisfied).
//
// Forward-only transitions: BOOTING → ACTIVE → COMPLETE. A node never
// regresses; if a verification breaks, the launcher exits or rolls
// the data dir back to a pre-bootstrap state and starts over.
//
// The machine is orthogonal to the trust model — it would look the
// same under a back-walk-to-genesis design or any other. Lives here
// (rather than in pipeline/) because consumers like the advertisement
// publisher reference it without depending on the trust phase.
package nodestate

import (
	"fmt"
	"sync"
	"time"
)

// State is the bootstrap state of a node.
type State int

const (
	// StateUnknown is the zero value. Treat as "not advertised" for
	// routing purposes — equivalent to a pre-design legacy node,
	// which is itself treated as COMPLETE for legacy queries.
	StateUnknown State = iota
	StateBooting
	StateActive
	StateComplete
)

func (s State) String() string {
	switch s {
	case StateBooting:
		return "BOOTING"
	case StateActive:
		return "ACTIVE"
	case StateComplete:
		return "COMPLETE"
	default:
		return "UNKNOWN"
	}
}

// CanServeCurrent reports whether the state can serve current-state
// queries (current account state, validator participation, bootstrap
// data for new launchers). True for ACTIVE and COMPLETE.
func (s State) CanServeCurrent() bool {
	return s == StateActive || s == StateComplete
}

// CanServeHistory reports whether the state can serve historical
// queries beyond the rolling window. True for COMPLETE only.
func (s State) CanServeHistory() bool {
	return s == StateComplete
}

// Advertisement is the payload published to peer discovery.
type Advertisement struct {
	State State

	// SinceBlock is the block height at which the state became true.
	SinceBlock uint64

	// VerifiedAnchor is the BPT root that validates the
	// ACTIVE/COMPLETE claim — peers can spot-check by querying a
	// BPT leaf and verifying the proof anchors at this root. Empty
	// for BOOTING.
	VerifiedAnchor [32]byte

	// HistoryDepth is the oldest block fully retained, for COMPLETE.
	// Zero means full history (no retention limit). Unused for
	// BOOTING / ACTIVE.
	HistoryDepth uint64

	// LastUpdated is the wall-clock time the advertisement was
	// generated; consumers discard advertisements older than 2 ×
	// the publishing heartbeat to avoid stale routing.
	LastUpdated time.Time
}

// Validate reports a malformed-payload error.
func (a *Advertisement) Validate() error {
	switch a.State {
	case StateBooting, StateActive, StateComplete:
		// ok
	default:
		return fmt.Errorf("nodestate: invalid state %d", a.State)
	}
	if a.State == StateActive || a.State == StateComplete {
		if a.VerifiedAnchor == ([32]byte{}) {
			return fmt.Errorf("nodestate: ACTIVE/COMPLETE advertisement must carry VerifiedAnchor")
		}
	}
	return nil
}

// Machine is the in-process state machine. Forward-only transitions.
// Persistence is handled by the caller (via bootpersist).
type Machine struct {
	mu       sync.RWMutex
	state    State
	since    uint64
	anchor   [32]byte
	depth    uint64
	last     time.Time
	onChange []func(Advertisement)
}

// New constructs a Machine in StateBooting.
func New() *Machine {
	return &Machine{
		state: StateBooting,
		last:  time.Now(),
	}
}

// Restore reconstructs a Machine from a persisted state record.
// state must be one of StateBooting, StateActive, or StateComplete.
// ACTIVE / COMPLETE require a non-zero verifiedAnchor.
func Restore(state State, sinceBlock uint64, verifiedAnchor [32]byte, historyDepth uint64) (*Machine, error) {
	switch state {
	case StateBooting, StateActive, StateComplete:
		// ok
	default:
		return nil, fmt.Errorf("nodestate.Restore: invalid state %d", state)
	}
	if (state == StateActive || state == StateComplete) && verifiedAnchor == ([32]byte{}) {
		return nil, fmt.Errorf("nodestate.Restore: ACTIVE/COMPLETE requires non-zero verifiedAnchor")
	}
	return &Machine{
		state:  state,
		since:  sinceBlock,
		anchor: verifiedAnchor,
		depth:  historyDepth,
		last:   time.Now(),
	}, nil
}

// ParseState maps the persisted string form ("BOOTING" / "ACTIVE" /
// "COMPLETE") to the typed State. Unrecognized strings return
// StateUnknown plus an error.
func ParseState(s string) (State, error) {
	switch s {
	case "BOOTING":
		return StateBooting, nil
	case "ACTIVE":
		return StateActive, nil
	case "COMPLETE":
		return StateComplete, nil
	default:
		return StateUnknown, fmt.Errorf("nodestate: unknown state %q", s)
	}
}

// Get returns the current advertisement payload.
func (m *Machine) Get() Advertisement {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.adLocked()
}

// State returns the current state.
func (m *Machine) State() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

// PromoteToActive transitions BOOTING → ACTIVE. anchor is the
// verified BPT root (must be non-zero); sinceBlock is the height
// the verification was established at. Returns false if the
// transition is invalid (e.g., already ACTIVE or beyond).
func (m *Machine) PromoteToActive(anchor [32]byte, sinceBlock uint64) bool {
	if anchor == ([32]byte{}) {
		return false
	}
	m.mu.Lock()
	if m.state != StateBooting {
		m.mu.Unlock()
		return false
	}
	m.state = StateActive
	m.anchor = anchor
	m.since = sinceBlock
	m.last = time.Now()
	cbs := append([]func(Advertisement){}, m.onChange...)
	ad := m.adLocked()
	m.mu.Unlock()

	for _, cb := range cbs {
		cb(ad)
	}
	return true
}

// PromoteToComplete transitions ACTIVE → COMPLETE. historyDepth is
// the oldest block fully retained (zero for unlimited). Returns
// false if the transition is invalid.
func (m *Machine) PromoteToComplete(historyDepth, sinceBlock uint64) bool {
	m.mu.Lock()
	if m.state != StateActive {
		m.mu.Unlock()
		return false
	}
	m.state = StateComplete
	m.depth = historyDepth
	m.since = sinceBlock
	m.last = time.Now()
	cbs := append([]func(Advertisement){}, m.onChange...)
	ad := m.adLocked()
	m.mu.Unlock()

	for _, cb := range cbs {
		cb(ad)
	}
	return true
}

// OnChange registers a callback fired on every state transition.
// Callbacks run synchronously in the caller's goroutine of the
// transitioning method.
func (m *Machine) OnChange(fn func(Advertisement)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onChange = append(m.onChange, fn)
}

// Heartbeat refreshes the LastUpdated timestamp without changing
// state. Advertisement publishers call this on a schedule so peers
// don't expire stale entries while state is stable.
func (m *Machine) Heartbeat() Advertisement {
	m.mu.Lock()
	m.last = time.Now()
	ad := m.adLocked()
	m.mu.Unlock()
	return ad
}

func (m *Machine) adLocked() Advertisement {
	return Advertisement{
		State:          m.state,
		SinceBlock:     m.since,
		VerifiedAnchor: m.anchor,
		HistoryDepth:   m.depth,
		LastUpdated:    m.last,
	}
}
