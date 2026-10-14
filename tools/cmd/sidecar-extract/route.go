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
func route(currentPaths []string, sidecarPath, conflictsPath, archiveArg, livePartition string, accounts []string) error {
	s, closeStores, err := openStores(currentPaths, sidecarPath, conflictsPath)
	if err != nil {
		return err
	}
	defer closeStores()
	batch := s.model()
	defer batch.Discard()

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
		indexRoots(batch, u, partition, livePartition)
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

// indexRoots tests each main-index entry of an account against the root
// chain of the partition it routed to before the reorg and then, for the
// entries that do not prove there, against the live partition's: an entry
// written since the reorg anchors into the live root chain.
func indexRoots(batch *coredb.Batch, u *url.URL, routed, live string) {
	roots := map[string]*coredb.Chain{}
	for _, p := range []string{routed, live} {
		if p == "" {
			continue
		}
		root, err := batch.Account(protocol.PartitionUrl(p).JoinPath(protocol.Ledger)).RootChain().Get()
		if err != nil {
			fmt.Printf("\t%s root chain: %v\n", p, err)
			return
		}
		roots[p] = root
	}
	main, err := batch.Account(u).MainChain().Get()
	if err != nil {
		return
	}
	index, err := batch.Account(u).MainChain().Index().Get()
	if err != nil {
		fmt.Printf("\tindex: %v\n", err)
		return
	}
	counts := map[string]int{}
	var unread int
	var firstNone *protocol.IndexEntry
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
		anchor, err := main.AnchorAt(e.Source)
		if err != nil {
			unread++
			continue
		}
		proved := "none"
		for _, p := range []string{routed, live} {
			root := roots[p]
			if root == nil || int64(e.Anchor) >= root.Height() {
				continue
			}
			rootEntry, err := root.Entry(int64(e.Anchor))
			if err == nil && bytes.Equal(rootEntry, anchor) {
				proved = p
				break
			}
		}
		counts[proved]++
		if proved == "none" && firstNone == nil {
			firstNone = e
		}
	}
	if routed == live || live == "" {
		fmt.Printf("\tindex %d entries: %d prove on %s, %d on neither, %d unreadable\n",
			index.Height(), counts[routed], routed, counts["none"], unread)
	} else {
		fmt.Printf("\tindex %d entries: %d prove on %s, %d on %s, %d on neither, %d unreadable\n",
			index.Height(), counts[routed], routed, counts[live], live, counts["none"], unread)
	}
	if firstNone != nil {
		fmt.Printf("\t  first on neither: source %d anchor %d block %d\n", firstNone.Source, firstNone.Anchor, firstNone.BlockIndex)
	}
}
