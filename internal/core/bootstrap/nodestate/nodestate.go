// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package nodestate is the state a node is in while it joins, and the
// advertisement that says so (executor.md, "Sync", step 6: a node serves
// last, and says what it can answer).
//
// Two states (executor.md, "Sync", step 6):
//
//   - BOOTING: from the start of a join until the local root matches a
//     verified anchored root. Cannot serve queries, cannot validate.
//   - ACTIVE:  the local root equals a root the Directory anchored, so the
//     state is verified. Serves and takes part in consensus.
//
// The one transition is BOOTING → ACTIVE. A node never regresses; a
// verification that breaks means starting over.
//
// WAITING and COMPLETE are retired (#4368): they named a backfilled history
// this line does not have, and nothing reached them. Their numbers stay
// reserved so ACTIVE keeps its value, and a persisted record naming one is
// mapped, not refused — see ParseState and Restore.
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
	// StateUnknown is the zero value: not advertised.
	StateUnknown State = iota
	StateBooting
	stateRetiredWaiting // reserved: was WAITING (#4368)
	StateActive
	stateRetiredComplete // reserved: was COMPLETE (#4368)
)

func (s State) String() string {
	switch s {
	case StateBooting:
		return "BOOTING"
	case StateActive:
		return "ACTIVE"
	default:
		return "UNKNOWN"
	}
}

// CanServeCurrent reports whether the state can serve current-state
// queries (current account state, validator participation, bootstrap
// data for new launchers). True for ACTIVE only.
func (s State) CanServeCurrent() bool {
	return s == StateActive
}

// Serving is what a service asks before it answers for the state this node
// holds. A joining node must not answer: its store is what the pull is filling
// and its ledgers are the ones it has not executed, so an answer from it is an
// answer about nothing (#4297, #4307).
//
// It is an interface because the services that ask are wired separately from
// the join that knows -- the querier is configured on its own, the submitter
// under consensus -- and a registry keyed by partition would give every node
// of a partition one node's state.
type Serving interface {
	// CanServeCurrent reports whether this node may answer for the state it
	// holds.
	CanServeCurrent() bool
}

// Always is a node that has always executed what it holds: it never joined, so
// it answers for itself.
type Always struct{}

// CanServeCurrent implements [Serving].
func (Always) CanServeCurrent() bool { return true }

var _ Serving = Always{}
var _ Serving = (*Machine)(nil)

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

	// LastUpdated is the wall-clock time the advertisement was
	// generated; consumers discard advertisements older than 2 ×
	// the publishing heartbeat to avoid stale routing.
	LastUpdated time.Time
}

// Validate reports a malformed-payload error.
func (a *Advertisement) Validate() error {
	switch a.State {
	case StateBooting, StateActive:
		// ok
	default:
		return fmt.Errorf("nodestate: invalid state %d", a.State)
	}
	if a.Partition == nil {
		return fmt.Errorf("nodestate: an advertisement must name its partition")
	}
	if a.State == StateActive && a.VerifiedAnchor == ([32]byte{}) {
		return fmt.Errorf("nodestate: ACTIVE advertisement must carry VerifiedAnchor")
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

// Restore reconstructs a Machine from a persisted state record. state must
// be StateBooting or StateActive, and ACTIVE requires a non-zero
// verifiedAnchor.
//
// A record from before #4368 may name a retired state, and it is MAPPED, not
// refused, so a node that persisted one restarts: WAITING restores as BOOTING
// with no anchor — its root was claimed, never matched to an anchored one, so
// the join is not done — and COMPLETE restores as ACTIVE, which it was plus a
// backfill this line does not have, so it still requires its anchor.
func Restore(partition *url.URL, state State, sinceBlock uint64, verifiedAnchor [32]byte) (*Machine, error) {
	if partition == nil {
		return nil, fmt.Errorf("nodestate.Restore: partition required")
	}
	switch state {
	case StateBooting, StateActive:
		// ok
	case stateRetiredWaiting:
		state, verifiedAnchor = StateBooting, [32]byte{}
	case stateRetiredComplete:
		state = StateActive
	default:
		return nil, fmt.Errorf("nodestate.Restore: invalid state %d", state)
	}
	if state == StateActive && verifiedAnchor == ([32]byte{}) {
		return nil, fmt.Errorf("nodestate.Restore: ACTIVE requires non-zero verifiedAnchor")
	}
	return &Machine{
		partition: partition,
		state:     state,
		since:     sinceBlock,
		anchor:    verifiedAnchor,
		last:      time.Now(),
	}, nil
}

// ParseState maps the persisted string form to the typed State.
// Unrecognized strings return StateUnknown plus an error.
//
// The retired forms are mapped as Restore maps them (#4368): "WAITING" reads
// as BOOTING, since a root that was never matched to an anchored one is a join
// not finished, and "COMPLETE" reads as ACTIVE, which it was plus a backfill
// this line does not have.
func ParseState(s string) (State, error) {
	switch s {
	case "BOOTING", "WAITING":
		return StateBooting, nil
	case "ACTIVE", "COMPLETE":
		return StateActive, nil
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

// CanServeCurrent implements [Serving]: this node may answer for the state it
// holds once it has matched an anchored root.
func (m *Machine) CanServeCurrent() bool {
	if m == nil {
		return true // no state of its own: it never joined
	}
	return m.State().CanServeCurrent()
}

// State returns the current state.
func (m *Machine) State() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

// PromoteToActive transitions BOOTING → ACTIVE. anchor is the root the Directory anchored that
// the local root now equals (non-zero), and sinceBlock is the block it was
// anchored for — block Q of executor.md, "Sync". Returns false if the
// transition is invalid.
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
		LastUpdated:    m.last,
	}
}
