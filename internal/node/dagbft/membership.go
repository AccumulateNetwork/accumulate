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

// InCommittee reports whether this node may take a submission for this
// partition.
//
// The answer comes from the one place that reads a committee,
// GlobalValues.MembershipOf, which gives three values; this is the call site
// that decides what UNKNOWN means here. UNKNOWN IS NOT A REFUSAL. If no
// globals have arrived yet — the daemon waits five seconds and then carries
// on with an empty definition (cmd/accumulated/run/dagbft.go:383-390) — the
// node answers as it did before this predicate existed. Refusing on that
// race would take every validator of a starting network off the air, and the
// refusal is meant to be the positive fact that a KNOWN committee does not
// contain this key. The conductor decides the opposite for the same answer,
// and for the opposite reason: it must not sign an anchor it cannot justify
// (internal/core/crosschain/cadence.go). That is why the shared answer is
// three-valued and not a boolean (#4366, #4367).
func (m *Membership) InCommittee() bool {
	if m == nil {
		return true
	}
	return m.globals.Load().MembershipOf(m.key, m.partition) != network.CommitteeOutsider
}
