// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package nodestate defines the bootstrap state machine and the
// advertisement payload published to peer discovery (issue #3970,
// parent #3953).
//
// A node moves through three states with explicit trust semantics:
//
//   - BOOTING: pulling state and back-walking for proof-of-derivation.
//     Cannot serve queries reliably; cannot validate.
//   - ACTIVE:  full BPT match against the network's StateTreeAnchor;
//     every account loaded; can re-execute transactions; validator-eligible.
//   - COMPLETE: ACTIVE plus full chain-history backfill.
//
// State must be advertised to peer discovery so clients and other nodes
// route queries by capability. Existing nodes that predate this design
// don't advertise; for backwards compatibility they are treated as
// COMPLETE for legacy queries but cannot serve any of the new bootstrap-
// supporting APIs.
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
	// routing purposes — equivalent to a pre-design legacy node, which
	// is itself treated as COMPLETE for legacy queries (per #3970).
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

// CanServeCurrent reports whether the state is good for current-state
// queries (current account state, validator participation, bootstrap
// data for new launchers). True for ACTIVE and COMPLETE.
func (s State) CanServeCurrent() bool {
	return s == StateActive || s == StateComplete
}

// CanServeHistory reports whether the state is good for historical
// queries beyond the rolling window. True for COMPLETE only.
func (s State) CanServeHistory() bool {
	return s == StateComplete
}

// Advertisement is the payload published to peer discovery.
type Advertisement struct {
	// State is the current bootstrap state of the node.
	State State

	// SinceBlock is the block height at which the state became true.
	SinceBlock uint64

	// BptRootMatched is the BPT root that validates the ACTIVE/COMPLETE
	// claim — consumers can spot-check by querying GetBptLeaf on this peer
	// and verifying the proof anchors at this root.
	// Empty for BOOTING.
	BptRootMatched [32]byte

	// HistoryDepth is the oldest block fully retained, for COMPLETE.
	// Zero means full history (no retention limit). Unused for BOOTING /
	// ACTIVE.
	HistoryDepth uint64

	// LastUpdated is the wall-clock time the advertisement was generated;
	// stale advertisements (older than 2 × heartbeat) should be discarded
	// by consumers.
	LastUpdated time.Time
}

// Validate reports a malformed-payload error.
func (a *Advertisement) Validate() error {
	switch a.State {
	case StateBooting, StateActive, StateComplete:
		// ok
	default:
		return fmt.Errorf("invalid state %d", a.State)
	}
	if a.State == StateActive || a.State == StateComplete {
		if a.BptRootMatched == ([32]byte{}) {
			return fmt.Errorf("ACTIVE/COMPLETE advertisement must carry BptRootMatched")
		}
	}
	return nil
}

// Machine is the in-process node-state machine. It enforces the forward-
// only transition order BOOTING → ACTIVE → COMPLETE. State transitions
// are persisted by the caller (e.g., via #3965).
type Machine struct {
	mu       sync.RWMutex
	state    State
	since    uint64
	bptRoot  [32]byte
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

// Restore reconstructs a Machine from a persisted state record (issue
// #3981 — accumulated run handoff). state must be one of StateBooting,
// StateActive, or StateComplete. ACTIVE / COMPLETE require a non-zero
// bptRootMatched.
func Restore(state State, sinceBlock uint64, bptRootMatched [32]byte, historyDepth uint64) (*Machine, error) {
	switch state {
	case StateBooting, StateActive, StateComplete:
		// ok
	default:
		return nil, fmt.Errorf("nodestate.Restore: invalid state %d", state)
	}
	if (state == StateActive || state == StateComplete) && bptRootMatched == ([32]byte{}) {
		return nil, fmt.Errorf("nodestate.Restore: ACTIVE/COMPLETE requires non-zero bptRootMatched")
	}
	return &Machine{
		state:   state,
		since:   sinceBlock,
		bptRoot: bptRootMatched,
		depth:   historyDepth,
		last:    time.Now(),
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
	return Advertisement{
		State:          m.state,
		SinceBlock:     m.since,
		BptRootMatched: m.bptRoot,
		HistoryDepth:   m.depth,
		LastUpdated:    m.last,
	}
}

// State returns the current state.
func (m *Machine) State() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

// PromoteToActive transitions BOOTING → ACTIVE. bptRoot is the matched
// BPT root (must be non-zero); sinceBlock is the block height the match
// was established at. Returns false if the transition is invalid (e.g.,
// already ACTIVE or beyond).
func (m *Machine) PromoteToActive(bptRoot [32]byte, sinceBlock uint64) bool {
	if bptRoot == ([32]byte{}) {
		return false
	}
	m.mu.Lock()
	if m.state != StateBooting {
		m.mu.Unlock()
		return false
	}
	m.state = StateActive
	m.bptRoot = bptRoot
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

// PromoteToComplete transitions ACTIVE → COMPLETE. historyDepth is the
// oldest block fully retained (zero for unlimited). Returns false if the
// transition is invalid.
func (m *Machine) PromoteToComplete(historyDepth uint64, sinceBlock uint64) bool {
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
// Callbacks are invoked synchronously in the caller's goroutine of the
// transitioning method.
func (m *Machine) OnChange(fn func(Advertisement)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onChange = append(m.onChange, fn)
}

// Heartbeat refreshes the LastUpdated timestamp without changing state.
// Used by the advertisement publisher to keep peer-discovery entries
// fresh even when state is stable.
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
		BptRootMatched: m.bptRoot,
		HistoryDepth:   m.depth,
		LastUpdated:    m.last,
	}
}
