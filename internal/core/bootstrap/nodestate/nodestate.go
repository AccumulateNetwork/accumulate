// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package nodestate is the state a node is in while it joins, and the
// advertisement that says so (executor.md, "Sync", step 6: a node serves
// last, and says what it can answer).
//
// TWO states, because fully synced is a verified state and not a backfilled
// history:
//
//   - BOOTING: from the start of a join until the local root matches a root
//     the Directory anchored. It refuses every read and cannot validate.
//   - ACTIVE:  from that block on. The state is some block's state and the
//     node answers for it.
//
// A node that never joined has no machine at all and serves as ACTIVE
// (nodestate.Always). The transition is forward only and there is one of
// them: a verification that breaks means starting over.
//
// WAITING and COMPLETE are gone. COMPLETE meant "ACTIVE plus the history
// backfilled — the producer cache filled and the chains the node lacked
// fetched"; this line has no backfill, so nothing could reach it, and keying
// serving on it would have meant every node that ever joined or restarted
// refused every read for the rest of its life. WAITING was a step between a
// local root and an anchored one that no caller ever took. Neither had a
// production caller; both are retired by the spec (#4368, and the scan in
// callers_test.go, which fails if either returns). The gauge keeps its
// numbering — 0 booting, 2 active — because a monitor matches on the value.
//
// A machine is per partition. A node serves two of them, and every block
// number it advertises is a block of one partition or the other (#4205).
//
// Ported from bootstrap-v3 (issue #4293). Changed on this line: the states are
// the partition's, not the node's; the doc comment named a snapshot this
// line no longer has; and two of the four states are retired (#4295).
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
	// routing purposes.
	StateUnknown State = iota
	StateBooting
	// 2 was WAITING, retired (#4368). The constant is not reused, so the
	// gauge's numbering — 0 booting, 2 active — is unchanged and a monitor
	// that matches on it keeps working.
	_
	StateActive
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

// Serves reports whether a node in this state may answer for the state it
// holds. True for ACTIVE, and for ACTIVE alone: a node that is BOOTING
// refuses every read (executor.md, "Sync", step 6).
//
// A State deliberately does NOT implement [Serving]. The gauge is written
// from the object that answers, and a State is a reading of one, not the
// thing itself: if State satisfied Serving, every call that used to hand
// Report a state the caller chose would still compile and the two objects
// would be two again (#4295).
func (s State) Serves() bool {
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

// Undecided is a node that has not decided whether it must join. It answers
// nothing, because at that point in start-up none of its services exists.
//
// It is here so that the gauge is never written from anything but the object
// that answers CanServeCurrent (#4295): the daemon puts this partition's
// state on the wire before it reads its own last block, so that "absent"
// cannot be mistaken for "booting" (#4345a), and BOOTING is the honest value
// for a node that has decided nothing.
type Undecided struct{}

// CanServeCurrent implements [Serving].
func (Undecided) CanServeCurrent() bool { return false }

var _ Serving = Always{}
var _ Serving = Undecided{}
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
		return fmt.Errorf("nodestate: an ACTIVE advertisement must carry VerifiedAnchor")
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
	return m.State().Serves()
}

// State returns the current state.
func (m *Machine) State() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

// PromoteToActive transitions BOOTING → ACTIVE, the one transition there is.
// anchor is the root the Directory anchored that the local root now equals
// (non-zero), and sinceBlock is the block it was anchored for — block Q of
// executor.md, "Sync". Returns false if the transition is invalid.
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
