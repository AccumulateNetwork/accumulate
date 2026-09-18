// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package nodestate is the state a node is in while it joins, and the
// advertisement that says so (executor.md, "Sync", step 5: a node serves
// last, and says what it can answer).
//
// Four states:
//
//   - BOOTING:  pulling the spine and the state. Cannot serve queries,
//     cannot validate.
//   - WAITING:  the state is local, but no anchored root has been seen that
//     matches it yet. Still cannot serve queries.
//   - ACTIVE:   the local root equals a root the Directory anchored, so the
//     state is verified. Can serve current-state queries and take part in
//     consensus.
//   - COMPLETE: ACTIVE plus the history backfilled — the producer cache
//     filled and the chains the node lacked fetched — so it can answer the
//     sequencer and healing too.
//
// Transitions are forward only: BOOTING → WAITING → ACTIVE → COMPLETE. A node
// never regresses; a verification that breaks means starting over.
//
// A machine is per partition. A node serves two of them, and every block
// number it advertises is a block of one partition or the other (#4205).
//
// Ported from bootstrap-v3 (issue #4293). Changed on this line: the states are
// the partition's, not the node's, and the doc comment named a snapshot this
// line no longer has.
package nodestate

import (
	"fmt"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// State is the bootstrap state of a node.
type State int

const (
	// StateUnknown is the zero value. Treat as "not advertised" for
	// routing purposes — equivalent to a pre-design legacy node,
	// which is itself treated as COMPLETE for legacy queries.
	StateUnknown State = iota
	StateBooting
	StateWaiting
	StateActive
	StateComplete
)

func (s State) String() string {
	switch s {
	case StateBooting:
		return "BOOTING"
	case StateWaiting:
		return "WAITING"
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

	// Partition is the partition this advertisement is about. A node serves
	// two of them, and block numbers collide across partitions — with a
	// one-second cadence the Directory and a BVN are at the same number at
	// the same second — so SinceBlock without it names nothing (#4205).
	Partition *url.URL

	// SinceBlock is the block of Partition at which the state became true.
	SinceBlock uint64

	// VerifiedAnchor is the root the Directory anchored that the node's
	// own root matched. Empty for BOOTING.
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
	case StateBooting, StateWaiting, StateActive, StateComplete:
		// ok
	default:
		return fmt.Errorf("nodestate: invalid state %d", a.State)
	}
	if a.Partition == nil {
		return fmt.Errorf("nodestate: an advertisement must name its partition")
	}
	if a.State == StateActive || a.State == StateComplete {
		if a.VerifiedAnchor == ([32]byte{}) {
			return fmt.Errorf("nodestate: ACTIVE/COMPLETE advertisement must carry VerifiedAnchor")
		}
	}
	return nil
}

// Machine is the in-process state machine. Forward-only transitions.
// Persistence is the caller's.
type Machine struct {
	partition *url.URL

	mu       sync.RWMutex
	state    State
	since    uint64
	anchor   [32]byte
	depth    uint64
	last     time.Time
	onChange []func(Advertisement)
}

// New constructs a Machine in StateBooting for a partition.
func New(partition *url.URL) *Machine {
	return &Machine{
		partition: partition,
		state:     StateBooting,
		last:      time.Now(),
	}
}

// Partition reports the partition this machine is the state of.
func (m *Machine) Partition() *url.URL { return m.partition }

// Restore reconstructs a Machine from a persisted state record.
// state must be one of StateBooting, StateWaiting, StateActive, or
// StateComplete. ACTIVE / COMPLETE require a non-zero verifiedAnchor.
func Restore(partition *url.URL, state State, sinceBlock uint64, verifiedAnchor [32]byte, historyDepth uint64) (*Machine, error) {
	if partition == nil {
		return nil, fmt.Errorf("nodestate.Restore: partition required")
	}
	switch state {
	case StateBooting, StateWaiting, StateActive, StateComplete:
		// ok
	default:
		return nil, fmt.Errorf("nodestate.Restore: invalid state %d", state)
	}
	if (state == StateActive || state == StateComplete) && verifiedAnchor == ([32]byte{}) {
		return nil, fmt.Errorf("nodestate.Restore: ACTIVE/COMPLETE requires non-zero verifiedAnchor")
	}
	return &Machine{
		partition: partition,
		state:     state,
		since:     sinceBlock,
		anchor:    verifiedAnchor,
		depth:     historyDepth,
		last:      time.Now(),
	}, nil
}

// ParseState maps the persisted string form to the typed State.
// Unrecognized strings return StateUnknown plus an error.
func ParseState(s string) (State, error) {
	switch s {
	case "BOOTING":
		return StateBooting, nil
	case "WAITING":
		return StateWaiting, nil
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

// PromoteToWaiting transitions BOOTING → WAITING: the state is local and the
// node knows what root it thinks it has, but no anchored root matching it has
// been seen yet. claimedAnchor is the local BPT root and sinceBlock is the
// block it was taken at. Returns false if the transition is invalid.
func (m *Machine) PromoteToWaiting(claimedAnchor [32]byte, sinceBlock uint64) bool {
	if claimedAnchor == ([32]byte{}) {
		return false
	}
	m.mu.Lock()
	if m.state != StateBooting {
		m.mu.Unlock()
		return false
	}
	m.state = StateWaiting
	m.anchor = claimedAnchor
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

// PromoteToActive transitions WAITING → ACTIVE, or BOOTING → ACTIVE for a
// caller that skips WAITING. anchor is the root the Directory anchored that
// the local root now equals (non-zero), and sinceBlock is the block it was
// anchored for — block Q of executor.md, "Sync". Returns false if the
// transition is invalid.
func (m *Machine) PromoteToActive(anchor [32]byte, sinceBlock uint64) bool {
	if anchor == ([32]byte{}) {
		return false
	}
	m.mu.Lock()
	if m.state != StateBooting && m.state != StateWaiting {
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
		Partition:      m.partition,
		SinceBlock:     m.since,
		VerifiedAnchor: m.anchor,
		HistoryDepth:   m.depth,
		LastUpdated:    m.last,
	}
}
