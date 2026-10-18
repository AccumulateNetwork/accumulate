// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package sidecar

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Policy is the read-side rule of #4560: which records the API may read from
// the sidecar, and which of those a node keeps.
//
// The sidecar holds the history the reorg dropped, but also every other
// record the archives held that the live database does not: the old
// partitions' ledgers, old state. What a node agrees on must not depend on
// any of it, so the records that feed hashChains for the partition this node
// serves -- its BPT, its ledger and its anchor pool -- never fall through. The
// old partitions' ledgers do, because the executor never reads them and the
// receipt builder needs them (#4567).
//
// What is kept is narrower still: only records the live node will never
// write, so a write-back can never race the executor for a key. Those are
// messages and transactions, keyed by hash, and the elements, mark points and
// element indexes of chains, which are append-only and which the live node
// kept whole through the reorg. A data index entry is not kept: the reorg
// restarted those counters, so the live node writes the same keys again. A
// chain head, a pending set, an account's state are not kept: they are
// current state, and the live node's is the truth.
type Policy struct {
	ledger, anchorPool *url.URL
}

// PolicyFor returns the policy for a node serving the partition.
func PolicyFor(partition string) Policy {
	u := protocol.PartitionUrl(partition)
	return Policy{ledger: u.JoinPath(protocol.Ledger), anchorPool: u.JoinPath(protocol.AnchorPool)}
}

// FallThrough reports whether a read of key that misses the live database may
// be answered from the sidecar.
func (p Policy) FallThrough(key *record.Key) bool {
	if key.Len() == 0 {
		return false
	}
	switch first, _ := key.Get(0).(string); first {
	case "BPT":
		return false
	case "Account":
		// The second element is the account's URL; anything else is not a
		// record of this node's partition
		u, _ := key.Get(1).(*url.URL)
		if u == nil {
			return true
		}
		if p.ledger != nil && u.Equal(p.ledger) {
			return false
		}
		if p.anchorPool != nil && u.Equal(p.anchorPool) {
			return false
		}
	}
	return true
}

// Keep reports whether a value read from the sidecar is written back into
// the live database.
func (p Policy) Keep(key *record.Key) bool {
	if !p.FallThrough(key) {
		return false
	}
	switch first, _ := key.Get(0).(string); first {
	case "Message", "Transaction":
		return true
	case "Account":
		// A chain record: ..., <chain>, {Element|States|ElementIndex|Intermediate}, ...
		for i := 2; i < key.Len(); i++ {
			switch s, _ := key.Get(i).(string); s {
			case "Element", "States", "ElementIndex", "Intermediate":
				return i > 2
			}
		}
	}
	return false
}
