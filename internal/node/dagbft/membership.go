// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"bytes"
	"sync/atomic"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
)

// Membership answers one question: is this node's author key in the current
// committee of this partition?
//
// It is the predicate behind "a node does not take what it cannot propose"
// (docs/spec/executor.md, Sync step 5). A submission's only road to a block
// is the receiving node's own batch and its own header (consensus.md, "What a
// batch is"), and a header whose author is in no committee is dropped before
// any vote (pkg/consensus/primary/vote_handler.go:277-284). A node in no
// committee that accepts a submission therefore strands it: reproduced
// in-process on #4366 at 1,249 accepted and 0 committed, and measured on run
// 20260919T191634Z as 3,929 healed entries and one lost user transaction, all
// on the follower's partition.
//
// The committee is READ, from the globals the node already holds, kept
// current by WillChangeGlobals exactly as the executor bridge keeps its
// validator set current (pkg/consensus/adapter/executor_bridge.go:117-140).
// Nothing here changes a committee: promotion and demotion are phase 2.
type Membership struct {
	partition string
	key       []byte
	globals   atomic.Pointer[network.GlobalValues]
}

// NewMembership returns the membership predicate for a partition and this
// node's author (validator) public key.
func NewMembership(partition string, authorKey []byte) *Membership {
	m := new(Membership)
	m.partition = partition
	m.key = bytes.Clone(authorKey)
	return m
}

// SetGlobals seeds or replaces the network definition the predicate reads.
//
// The daemon seeds it directly as well as subscribing, because whether a
// subscriber observes the INITIAL WillChangeGlobals is a startup ordering
// race — the same race the daemon already works around for the adapter and
// the conductor (cmd/accumulated/run/dagbft.go, "Seed the conductor's globals
// directly").
func (m *Membership) SetGlobals(g *network.GlobalValues) {
	if g != nil {
		m.globals.Store(g)
	}
}

// SubscribeGlobals keeps the predicate current.
func (m *Membership) SubscribeGlobals(bus *events.Bus) {
	if bus == nil {
		return
	}
	events.SubscribeSync(bus, func(e events.WillChangeGlobals) error {
		m.SetGlobals(e.New)
		return nil
	})
}

// InCommittee reports whether this node's key is active on this partition in
// the current network definition.
//
// An UNKNOWN committee is not a refusal. If no globals have arrived, or the
// network definition names no validators at all, the node answers as it did
// before this predicate existed: refusing on a startup race would take every
// validator of a starting network off the air, and the refusal is meant to be
// the positive fact that a known committee does not contain this key.
func (m *Membership) InCommittee() bool {
	if m == nil {
		return true
	}
	g := m.globals.Load()
	if g == nil || g.Network == nil || len(g.Network.Validators) == 0 {
		return true
	}
	// A walk over the validators, not NetworkDefinition.ValidatorByKey: that
	// is a binary search over PublicKeyHash, which answers "not in the
	// network" for a definition that is unsorted or whose hashes are unset,
	// and here that answer means refuse everything. Twelve validators is a
	// walk, and it reads the field the daemon itself reads to build the
	// committee (cmd/accumulated/run/dagbft.go:411-421).
	for _, v := range g.Network.Validators {
		if bytes.Equal(v.PublicKey, m.key) {
			return v.IsActiveOn(m.partition)
		}
	}
	return false
}
