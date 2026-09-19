// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"sync/atomic"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
)

// Membership answers two questions about one partition's current committee:
// is this node's author key in it, and is some other node's author key in it?
//
// The first decides whether this node may propose a transaction at all. A
// submission's only road to a block is the receiving node's own batch and its
// own header (consensus.md, "What a batch is"), and a header whose author is
// in no committee is dropped before any vote
// (pkg/consensus/primary/vote_handler.go:277-284). A node in no committee
// that keeps a submission therefore strands it: reproduced in-process on
// #4366 at 1,249 accepted and 0 committed, and measured on run
// 20260919T191634Z as 3,929 healed entries and one lost user transaction.
// What it does instead is relay it (executor.md, "Sync" step 6).
//
// The second decides where. A relay goes to a node that CAN propose, never
// to another node that would only relay it again, so the relay confirms a
// candidate's author key against this same committee before it hands
// anything over (see relay.go).
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

// Standing is what the globals this node holds say about its own key.
func (m *Membership) Standing() network.CommitteeMembership {
	if m == nil {
		return network.CommitteeUnknown
	}
	return m.globals.Load().MembershipOf(m.key, m.partition)
}

// CanPropose reports whether this node may get a submission into a block for
// this partition by proposing it itself.
//
// Only a KNOWN membership says yes. An unknown committee is not a licence to
// propose: a node that does not know whether it is a validator cannot know
// that its header will be voted on, and if it is not, everything it takes is
// stranded silently — which is #4366 itself, arriving through the one door
// the first build left open (reviewer, 2026-09-19). So unknown relays, and
// the relay, holding no committee to choose a target from, answers NotReady
// and counts it (the lead's decision 4, which now holds for any node and not
// only a joining one).
//
// The cost is real and accepted: a validator whose globals have not loaded
// answers NotReady for a moment at startup — the daemon waits five seconds
// and then carries on with an empty definition
// (cmd/accumulated/run/dagbft.go:383-390) — and its submitter's clients ask
// another node, which is what NotReady is for. Taking traffic it might not
// be able to propose is the worse of the two.
//
// A nil Membership is no gate at all, for the callers that predate this.
func (m *Membership) CanPropose() bool {
	if m == nil {
		return true
	}
	return m.Standing() == network.CommitteeMember
}

// Known reports whether this node holds a committee for the partition at all.
//
// A node that holds none cannot choose a relay target from what it has: it
// does not know which of its peers can propose. It answers NotReady and says
// so (#4366, the lead's decision 4); inventing a target from a peer's own
// claim is what the "no peer is ever asked what it holds" rule forbids
// (executor.md, "Sync" step 4).
func (m *Membership) Known() bool {
	return m.Standing() != network.CommitteeUnknown
}

// ActiveKey returns the public key of the active validator of this partition
// whose SHA-256 hash is keyHash, as the globals this node holds give it.
//
// It is the root of trust for a relay's challenge: the relay verifies the
// candidate's signature against THIS key, so a peer that merely names a
// validator's hash cannot be a target (#4366 F1).
func (m *Membership) ActiveKey(keyHash [32]byte) (ed25519.PublicKey, bool) {
	if m == nil {
		return nil, false
	}
	g := m.globals.Load()
	if g == nil || g.Network == nil {
		return nil, false
	}
	for _, v := range g.Network.Validators {
		if len(v.PublicKey) != ed25519.PublicKeySize {
			continue
		}
		if v.PublicKeyHash != keyHash && sha256.Sum256(v.PublicKey) != keyHash {
			continue
		}
		if !v.IsActiveOn(m.partition) {
			return nil, false
		}
		return ed25519.PublicKey(bytes.Clone(v.PublicKey)), true
	}
	return nil, false
}

// IsMember reports whether the SHA-256 hash of an author key is an active
// validator of this partition, in the globals this node holds.
//
// The hash, not the key, because that is what a peer gives up about itself:
// ConsensusStatus carries ValidatorKeyHash (internal/node/dagbft/api.go), and
// the network definition carries both the key and its hash. This is the
// question the relay asks of every candidate before it hands one a
// submission.
func (m *Membership) IsMember(keyHash [32]byte) bool {
	if m == nil {
		return false
	}
	g := m.globals.Load()
	if g == nil || g.Network == nil {
		return false
	}
	for _, v := range g.Network.Validators {
		if v.PublicKeyHash != keyHash &&
			!(len(v.PublicKey) == ed25519.PublicKeySize && sha256.Sum256(v.PublicKey) == keyHash) {
			continue
		}
		return v.IsActiveOn(m.partition)
	}
	return false
}
