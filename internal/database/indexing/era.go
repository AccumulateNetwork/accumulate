// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import (
	"bytes"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Era is the network as it was before the 13 July 2025 reorg (#4567), for a
// receipt whose index entry anchors into a root chain the live node does not
// have.
//
// An index chain entry records the root chain height its anchor landed at,
// but not whose root chain. The reorg moved every account to Cyclops and
// restarted the Directory's root chain, so every index entry written before
// it names a height in a root chain that lives only in the sidecar. Which
// one is not recorded: it is routed. The routing table in force before the
// reorg names the partition the account lived on, and that partition's root
// chain, read as it was then, is the one the entry anchors into.
type Era struct {
	// Batch reads the databases as they were before the reorg: the sidecar
	// behind the live database, with the overwritten store in front for the
	// keys the reorg restarted.
	Batch *database.Batch

	// Route names the partition an account lived on before the reorg.
	Route func(*url.URL) (string, error)
}

// RouteTable reads the routing table in force before the reorg: the last
// state of the routing account, as the era holds it.
func (e *Era) RouteTable() (*protocol.RoutingTable, error) {
	var da *protocol.DataAccount
	err := e.Batch.Account(protocol.DnUrl().JoinPath(protocol.Routing)).Main().GetAs(&da)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load the era's routing account: %w", err)
	}
	if da.Entry == nil || len(da.Entry.GetData()) != 1 {
		return nil, errors.UnknownError.With("the era's routing account has no single-field entry")
	}
	table := new(protocol.RoutingTable)
	err = table.UnmarshalBinary(da.Entry.GetData()[0])
	if err != nil {
		return nil, errors.UnknownError.WithFormat("decode the era's routing table: %w", err)
	}
	return table, nil
}

// ChainReceipt is what [ChainReceiptFor] returns.
type ChainReceipt struct {
	Receipt *merkle.Receipt

	// RootEntry is the root index entry the receipt terminates in; its block
	// is the receipt's block, in the numbering of Partition.
	RootEntry *protocol.IndexEntry

	// RootIndexIndex is RootEntry's position on the root index chain.
	RootIndexIndex uint64

	// Partition is whose root chain the receipt terminates at: the serving
	// partition, or, for an entry written before the reorg, the partition the
	// account lived on then.
	Partition string

	// Era reports that the receipt was built through the era: Partition's
	// root chain is the pre-reorg one.
	Era bool
}

// ChainReceiptFor builds a receipt from entry index of chain c to the root
// chain its index entry anchors into. That is the serving partition's root
// chain for an entry written since the reorg, and the routed partition's
// pre-reorg root chain, read through the era, for one written before it
// (#4567). Without an era only the first works, as [ReceiptForChainIndex]
// always did.
//
// Which it is, is tested, not inferred: an index entry proves against a root
// chain when that chain's element at the entry's anchor height is the account
// chain's anchor at the entry's source. The transition between the eras is
// where the index chain's block numbers drop (the restore's block 1); testing
// each entry against the live root chain first says the same thing without
// walking the chain to find it.
//
// The restore's own block-1 entries of a few Directory accounts anchor a
// state their chain never had (#4567); such an entry proves against neither
// root chain. The genesis element it covers is then proved through the next
// index entry of the live era, which covers it too.
func ChainReceiptFor(partition config.NetworkUrl, batch *database.Batch, c *database.Chain2, index int64, targetHeight *uint64, era *Era) (*ChainReceipt, error) {
	if c.Type() == merkle.ChainTypeIndex {
		return nil, errors.BadRequest.WithFormat("cannot get a receipt for %s: index chains are not anchored", c.Name())
	}
	indexChain, err := c.Index().Get()
	if err != nil {
		return nil, fmt.Errorf("unable to load %s index chain: %w", c.Name(), err)
	}
	if indexChain.Height() == 0 {
		return nil, fmt.Errorf("cannot create receipt for entry %d of %s chain: index chain is empty", index, c.Name())
	}
	chain, err := c.Get()
	if err != nil {
		return nil, fmt.Errorf("unable to load %s chain: %w", c.Name(), err)
	}

	// The index entry covering this entry: the position the transaction's
	// chains index recorded when the entry was written (#4263), else searched
	// for. The root index position it recorded is only used once the root
	// chain is known.
	var anchorIndex *uint64
	pos, entry, ok := storedChainIndexPosition(batch, c, index, targetHeight)
	if ok {
		anchorIndex = &pos.AnchorIndex
	} else {
		_, entry, err = SearchIndexChain(indexChain, uint64(indexChain.Height())-1, MatchAfter, SearchIndexChainBySource(uint64(index)))
		if err != nil {
			return nil, fmt.Errorf("unable to locate index entry for entry %d of %s chain: %w", index, c.Name(), err)
		}
	}

	// Whose root chain
	ledger := batch.Account(partition.Ledger())
	root, err := ledger.RootChain().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get minor root chain: %w", err)
	}
	r := &ChainReceipt{Partition: partition.PartitionID()}
	rootIndexChain := ledger.RootChain().Index()
	switch {
	case proves(root, chain, entry):
		// The live era
	case era == nil:
		// Nothing else to try; the receipt below fails as it always did
	default:
		routed, err := era.Route(c.Account())
		if err != nil {
			return nil, errors.UnknownError.WithFormat("route %v before the reorg: %w", c.Account(), err)
		}
		oldLedger := era.Batch.Account(protocol.PartitionUrl(routed).JoinPath(protocol.Ledger))
		oldRoot, err := oldLedger.RootChain().Get()
		if err != nil {
			return nil, errors.UnknownError.WithFormat("get %s's pre-reorg root chain: %w", routed, err)
		}
		if proves(oldRoot, chain, entry) {
			root, rootIndexChain = oldRoot, oldLedger.RootChain().Index()
			r.Partition, r.Era = routed, true
			break
		}
		// The restore's block-1 anomaly: the next entry of the live era
		// covers this element too
		next, err := nextProvableEntry(indexChain, root, chain, entry)
		if err != nil {
			return nil, err
		}
		entry, anchorIndex = next, nil
	}

	// The root index entry: the one the chains index recorded, if it covers
	// the anchor, else searched for
	rootIndex, err := rootIndexChain.Get()
	if err != nil {
		return nil, fmt.Errorf("unable to load minor root index chain: %w", err)
	}
	if rootIndex.Height() == 0 {
		return nil, errors.InternalError.With("root index chain is empty")
	}
	if targetHeight == nil {
		targetHeight = &entry.Anchor
	} else if *targetHeight < entry.Anchor {
		return nil, fmt.Errorf("cannot satisfy target height %v: entry is anchored at height %v", *targetHeight, entry.Anchor)
	}
	if anchorIndex != nil {
		re := new(protocol.IndexEntry)
		err := rootIndex.EntryAs(int64(*anchorIndex), re)
		if err == nil && re.Source >= *targetHeight {
			r.RootIndexIndex, r.RootEntry = *anchorIndex, re
		}
	}
	if r.RootEntry == nil {
		r.RootIndexIndex, r.RootEntry, err = SearchIndexChain(rootIndex, uint64(rootIndex.Height())-1, MatchAfter, SearchIndexChainBySource(*targetHeight))
		if err != nil {
			return nil, fmt.Errorf("unable to locate index entry for entry %d of the minor root chain: %w", entry.Anchor, err)
		}
	}

	// The receipt: the account chain to its anchor, the root chain from there
	accountReceipt, err := chain.Receipt(index, int64(entry.Source))
	if err != nil {
		return nil, err
	}
	rootReceipt, err := root.Receipt(int64(entry.Anchor), int64(r.RootEntry.Source))
	if err != nil {
		return nil, errors.UnknownError.WithFormat("unable to construct a receipt from %d to %d for the %s root chain: %w", entry.Anchor, r.RootEntry.Source, r.Partition, err)
	}
	r.Receipt, err = accountReceipt.Combine(rootReceipt)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// proves reports whether the index entry anchors into root: root's element at
// the entry's anchor height is chain's anchor at the entry's source.
func proves(root, chain *database.Chain, e *protocol.IndexEntry) bool {
	if int64(e.Anchor) >= root.Height() {
		return false
	}
	anchor, err := chain.AnchorAt(e.Source)
	if err != nil {
		return false
	}
	rootEntry, err := root.Entry(int64(e.Anchor))
	return err == nil && bytes.Equal(rootEntry, anchor)
}

// anomalyWindow bounds how far past an entry that proves against no root
// chain the next live-era entry is looked for. The restore's block-1 entry is
// followed directly by the live era's.
const anomalyWindow = 8

// nextProvableEntry returns the first entry after e on the index chain that
// proves against root.
func nextProvableEntry(indexChain, root, chain *database.Chain, e *protocol.IndexEntry) (*protocol.IndexEntry, error) {
	pos, _, err := SearchIndexChain(indexChain, uint64(indexChain.Height())-1, MatchExact, SearchIndexChainBySource(e.Source))
	if err != nil {
		return nil, fmt.Errorf("locate index entry for source %d: %w", e.Source, err)
	}
	for i := pos + 1; i < uint64(indexChain.Height()) && i <= pos+anomalyWindow; i++ {
		next := new(protocol.IndexEntry)
		err := indexChain.EntryAs(int64(i), next)
		if err != nil {
			return nil, fmt.Errorf("load index entry %d: %w", i, err)
		}
		if proves(root, chain, next) {
			return next, nil
		}
	}
	return nil, errors.NotFound.WithFormat("index entry for source %d anchors into no root chain this node can read, and no later entry of the live era follows it", e.Source)
}

// storedChainIndexPosition returns the index entry the transaction's chains
// index recorded for this entry (#4263), with the record it came from. ok is
// false when there is none to use.
func storedChainIndexPosition(batch *database.Batch, c *database.Chain2, index int64, targetHeight *uint64) (*database.TransactionChainEntry, *protocol.IndexEntry, bool) {
	if targetHeight != nil {
		return nil, nil, false
	}
	chain, err := c.Get()
	if err != nil {
		return nil, nil, false
	}
	hash, err := chain.Entry(index)
	if err != nil {
		return nil, nil, false
	}
	entries, err := batch.Transaction(hash).Chains().Get()
	if err != nil {
		return nil, nil, false
	}
	var rec *database.TransactionChainEntry
	for _, e := range entries {
		if e.Chain == c.Name() && e.Account.Equal(c.Account()) {
			rec = e
			break
		}
	}
	if rec == nil {
		return nil, nil, false
	}
	entry, err := loadIndexEntry(c, rec.ChainIndex)
	if err != nil || entry.Source < uint64(index) {
		return nil, nil, false
	}
	return rec, entry, true
}
