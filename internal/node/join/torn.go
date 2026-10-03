// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// mStoreTorn is 1 while this node's store was found torn at start and the
// node refuses to resume onto it (executor spec, "Sync", "the node's store is
// its own execution"; #4507).
var mStoreTorn = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "accumulate",
	Subsystem: "join",
	Name:      "store_torn",
	Help:      "1 while this node's store was found torn at start: a chain's head names records the store does not hold, so the node does not resume onto it",
}, []string{"partition"})

// CheckStoreNewest is how many of each chain's newest elements CheckStore
// reads: the records a kill can leave unflushed are the last commit's, and a
// block appends a handful to each chain.
const CheckStoreNewest = 64

// CheckStore verifies the records a node would resume onto: for the chains
// the block path writes -- the system ledger's root, block-ledger and BPT
// chains, the anchor pool's main chain and its anchor chains from sources --
// every Tail chunk of the open mark set holds what the head says and the
// newest elements are present (merkle.Chain.CheckRecords). A kill can leave
// a head ahead of its records; a node that resumes onto such a store fails
// every block it produces (run 20261002T145702Z: bvn2-val3's RootChain
// Tail(21) 4 of 8, bvn2-val4's BlockLedgerChain Tail(5) 7 of 8; #4507). It
// returns the first record found wanting, nil for a store whose heads are
// backed. It reads through a deep view, so the store's window does not pass
// for a tear.
func CheckStore(db *database.Database, partition *url.URL, sources []string) error {
	batch := db.Deep().Begin(false)
	defer batch.Discard()
	ledger := batch.Account(partition.JoinPath(protocol.Ledger))
	pool := batch.Account(partition.JoinPath(protocol.AnchorPool))
	chains := []*database.Chain2{ledger.RootChain(), ledger.BlockLedgerChain(), ledger.BptChain(), ledger.BptChain().Index(), pool.MainChain()}
	for _, src := range sources {
		chains = append(chains, pool.AnchorChain(src).Root(), pool.AnchorChain(src).BPT())
	}
	for _, c := range chains {
		if err := c.Inner().CheckRecords(CheckStoreNewest); err != nil {
			return errors.InvalidRecord.WithFormat("store torn: %w", err)
		}
	}
	return nil
}
