// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dgraph-io/badger"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// route reads every routing table written to dn.acme/routing that the live
// databases and the sidecar hold, routes each account with the last one, and
// tests the account's main-index chain against the root chain of the
// partition it routes to. The live routing account was restarted at the
// reorg, so the chain's later elements -- the pre-reorg tables -- are read
// past the live head, straight from the sidecar.
func route(currentPaths []string, sidecarPath, conflictsPath, overwrittenPath, archiveArg, livePartition string, accounts []string) error {
	s, closeStores, err := openStores(currentPaths, sidecarPath, conflictsPath, overwrittenPath)
	if err != nil {
		return err
	}
	defer closeStores()
	batch := s.model()
	defer batch.Discard()
	era := s.era()
	defer era.Discard()

	var table *protocol.RoutingTable
	if archiveArg != "" {
		table, err = archiveRoutingTable(archiveArg, batch)
	} else {
		table, err = sidecarRoutingTable(batch)
	}
	if err != nil {
		return err
	}
	b, _ := json.Marshal(table)
	fmt.Printf("routing table: %s\n", b)
	tree, err := routing.NewRouteTree(table)
	if err != nil {
		return err
	}
	for _, a := range accounts {
		u, err := url.Parse(a)
		if err != nil {
			return err
		}
		partition, err := tree.Route(u)
		if err != nil {
			fmt.Printf("%s: %v\n", u, err)
			continue
		}
		fmt.Printf("%s routes to %s\n", u, partition)
		indexRoots(batch, era, u, partition, livePartition)
	}
	return nil
}

// archiveRoutingTable reads the routing account's main state from an archive
// (name=path, Badger, read-only) and returns the table its last entry holds.
func archiveRoutingTable(archiveArg string, batch *coredb.Batch) (*protocol.RoutingTable, error) {
	name, path, _ := strings.Cut(archiveArg, "=")
	db, err := badger.Open(badger.DefaultOptions(path).WithReadOnly(true).WithLogger(nil))
	if err != nil {
		return nil, fmt.Errorf("archive %s: %w", name, err)
	}
	defer db.Close()
	txn := db.NewTransaction(false)
	defer txn.Discard()
	key := batch.Account(protocol.DnUrl().JoinPath(protocol.Routing)).Main().Key()
	item, err := txn.Get(hashBytes(key))
	if err != nil {
		return nil, fmt.Errorf("archive %s: routing main state %s: %w", name, hashOf(key), err)
	}
	v, err := item.ValueCopy(nil)
	if err != nil {
		return nil, err
	}
	account, err := protocol.UnmarshalAccount(v)
	if err != nil {
		return nil, fmt.Errorf("archive %s: routing main state: %w", name, err)
	}
	da, ok := account.(*protocol.DataAccount)
	if !ok {
		return nil, fmt.Errorf("archive %s: routing main state is %v", name, account.Type())
	}
	if da.Entry == nil || len(da.Entry.GetData()) != 1 {
		return nil, fmt.Errorf("archive %s: routing main state has no single-field entry", name)
	}
	table := new(protocol.RoutingTable)
	if err := table.UnmarshalBinary(da.Entry.GetData()[0]); err != nil {
		return nil, fmt.Errorf("archive %s: routing table: %w", name, err)
	}
	fmt.Printf("routing table from archive %s (version %d)\n", name, item.Version())
	return table, nil
}

// sidecarRoutingTable returns the last table the live databases and the
// sidecar hold.
func sidecarRoutingTable(batch *coredb.Batch) (*protocol.RoutingTable, error) {
	tables, err := routingTables(batch)
	if err != nil {
		return nil, err
	}
	if len(tables) == 0 {
		return nil, fmt.Errorf("no routing table found; the live routing account was restarted and a chain under 256 entries keeps its hashes in its head, so name an archive")
	}
	for _, t := range tables {
		fmt.Printf("routing table at entry %d: %s\n", t.index, t.summary())
	}
	return tables[len(tables)-1].table, nil
}

type routingAt struct {
	index int64
	table *protocol.RoutingTable
}

func (r routingAt) summary() string {
	b, _ := json.Marshal(r.table)
	return string(b)
}

// routingTables walks the routing account's main chain element by element,
// past the live head, and decodes each write-data entry as a routing table.
func routingTables(batch *coredb.Batch) ([]routingAt, error) {
	chain := batch.Account(protocol.DnUrl().JoinPath(protocol.Routing)).MainChain().Inner()
	var out []routingAt
	for i := int64(0); ; i++ {
		h, err := chain.Element(uint64(i)).Get()
		if err != nil {
			break
		}
		msg, err := batch.Message2(h).Main().Get()
		if err != nil {
			fmt.Printf("routing entry %d %x: message: %v\n", i, h[:8], err)
			continue
		}
		tm, ok := msg.(*messaging.TransactionMessage)
		if !ok {
			continue
		}
		wd, ok := tm.Transaction.Body.(*protocol.WriteData)
		if !ok {
			fmt.Printf("routing entry %d: %v\n", i, tm.Transaction.Body.Type())
			continue
		}
		data := wd.Entry.GetData()
		if len(data) != 1 {
			fmt.Printf("routing entry %d: %d data fields\n", i, len(data))
			continue
		}
		table := new(protocol.RoutingTable)
		if err := table.UnmarshalBinary(data[0]); err != nil {
			fmt.Printf("routing entry %d: %v\n", i, err)
			continue
		}
		out = append(out, routingAt{i, table})
	}
	return out, nil
}

// indexRoots splits an account's main-index chain at the reorg and proves
// each side against its root chain. Index entries are in order and block
// numbers only rise within an era, so the first entry whose block number is
// below the one before it is the restore's: everything before it anchors
// into the root chain of the partition the account routed to, read as it was
// before the reorg (era); it and everything after anchor into the live
// partition's. A chain with no such entry is all one era: new if its first
// entry proves against the live root chain, else old.
func indexRoots(live, era *coredb.Batch, u *url.URL, routed, livePartition string) {
	oldRoot, err := era.Account(protocol.PartitionUrl(routed).JoinPath(protocol.Ledger)).RootChain().Get()
	if err != nil {
		fmt.Printf("\t%s root chain (era): %v\n", routed, err)
		return
	}
	var newRoot *coredb.Chain
	if livePartition != "" {
		newRoot, err = live.Account(protocol.PartitionUrl(livePartition).JoinPath(protocol.Ledger)).RootChain().Get()
		if err != nil {
			fmt.Printf("\t%s root chain: %v\n", livePartition, err)
			return
		}
	}
	main, err := live.Account(u).MainChain().Get()
	if err != nil {
		return
	}
	index, err := live.Account(u).MainChain().Index().Get()
	if err != nil {
		fmt.Printf("\tindex: %v\n", err)
		return
	}
	var entries []*protocol.IndexEntry
	var unread int
	for i := int64(0); i < index.Height(); i++ {
		b, err := index.Entry(i)
		if err != nil {
			unread++
			continue
		}
		e := new(protocol.IndexEntry)
		if e.UnmarshalBinary(b) != nil {
			unread++
			continue
		}
		entries = append(entries, e)
	}
	proves := func(root *coredb.Chain, e *protocol.IndexEntry) bool {
		if root == nil || int64(e.Anchor) >= root.Height() {
			return false
		}
		anchor, err := main.AnchorAt(e.Source)
		if err != nil {
			return false
		}
		rootEntry, err := root.Entry(int64(e.Anchor))
		return err == nil && bytes.Equal(rootEntry, anchor)
	}

	// The transition
	t := -1
	for i := 1; i < len(entries); i++ {
		if entries[i].BlockIndex < entries[i-1].BlockIndex {
			t = i
			break
		}
	}
	switch {
	case t >= 0:
		fmt.Printf("\tindex %d entries, %d unreadable; transition at %d: block %d then %d\n", len(entries), unread, t, entries[t-1].BlockIndex, entries[t].BlockIndex)
	case len(entries) == 0:
		fmt.Printf("\tindex 0 entries, %d unreadable\n", unread)
		return
	case proves(newRoot, entries[0]):
		t = 0
		fmt.Printf("\tindex %d entries, %d unreadable; no transition, all since the reorg\n", len(entries), unread)
	default:
		t = len(entries)
		fmt.Printf("\tindex %d entries, %d unreadable; no transition, all before the reorg\n", len(entries), unread)
	}

	var oldOK, newOK int
	var firstOld, firstNew *protocol.IndexEntry
	for i, e := range entries {
		if i < t {
			if proves(oldRoot, e) {
				oldOK++
			} else if firstOld == nil {
				firstOld = e
			}
		} else {
			if proves(newRoot, e) {
				newOK++
			} else if firstNew == nil {
				firstNew = e
			}
		}
	}
	if t > 0 {
		fmt.Printf("\t  before: %d of %d prove on %s\n", oldOK, t, routed)
		if firstOld != nil {
			fmt.Printf("\t    first that does not: source %d anchor %d block %d\n", firstOld.Source, firstOld.Anchor, firstOld.BlockIndex)
		}
		secondHop(era, oldRoot, routed, entries[0])
	}
	if t < len(entries) {
		fmt.Printf("\t  since: %d of %d prove on %s\n", newOK, len(entries)-t, livePartition)
		if firstNew != nil {
			fmt.Printf("\t    first that does not: source %d anchor %d block %d\n", firstNew.Source, firstNew.Anchor, firstNew.BlockIndex)
		}
	}
}

// secondHop says whether the directory's anchor(P)-root chain, read in the
// same era, holds P's root anchor for the block an index entry was written
// in: the hop a receipt from that entry binds through.
func secondHop(batch *coredb.Batch, root *coredb.Chain, partition string, e *protocol.IndexEntry) {
	received := batch.Account(protocol.DnUrl().JoinPath(protocol.AnchorPool)).AnchorChain(partition).Root()
	rc, err := received.Get()
	if err != nil || rc.Height() == 0 {
		fmt.Printf("\t  dn anchor(%s)-root: none\n", partition)
		return
	}
	rootIndex, err := batch.Account(protocol.PartitionUrl(partition).JoinPath(protocol.Ledger)).RootChain().Index().Get()
	if err != nil {
		fmt.Printf("\t  %s root index chain: %v\n", partition, err)
		return
	}
	_, re, err := indexing.SearchIndexChain(rootIndex, uint64(rootIndex.Height())-1, indexing.MatchAfter, indexing.SearchIndexChainBySource(e.Anchor))
	if err != nil {
		fmt.Printf("\t  %s root index chain (%d entries): no entry covers root height %d: %v\n", partition, rootIndex.Height(), e.Anchor, err)
		return
	}
	blockAnchor, err := root.AnchorAt(re.Source)
	if err != nil {
		fmt.Printf("\t  %s root chain anchor at %d: %v\n", partition, re.Source, err)
		return
	}
	at, err := rc.HeightOf(blockAnchor)
	if err != nil {
		fmt.Printf("\t  dn anchor(%s)-root (%d entries) does not hold %s's root anchor at height %d (block %d)\n", partition, rc.Height(), partition, re.Source, re.BlockIndex)
		return
	}
	fmt.Printf("\t  dn anchor(%s)-root (%d entries) holds %s's root anchor at height %d (block %d) at entry %d\n", partition, rc.Height(), partition, re.Source, re.BlockIndex, at)
}
