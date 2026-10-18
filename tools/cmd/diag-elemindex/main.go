// Diagnostic for run 20261002T145702Z (#4507, BlockchainDB#99): what a
// BlockchainDB store holds for a partition's chains after a kill, read
// shallow and deep. Not part of the build.
package main

import (
	"fmt"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/bcdb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	store, err := bcdb.Open(os.Args[1])
	if err != nil {
		panic(err)
	}
	part := os.Args[2] // e.g. BVN2
	db := database.New(store, nil)
	if len(os.Args) > 3 && os.Args[3] == "tails" {
		var i, mp, cs int64
		fmt.Sscan(os.Args[5], &i)
		fmt.Sscan(os.Args[6], &mp)
		fmt.Sscan(os.Args[7], &cs)
		tails(db, part, os.Args[4], i, mp, cs)
		return
	}
	if len(os.Args) > 3 {
		probe(db, part, os.Args[3:])
		return
	}
	for _, deep := range []bool{false, true} {
		d := db
		if deep {
			d = db.Deep()
		}
		batch := d.Begin(false)
		fmt.Printf("=== %s read\n", map[bool]string{false: "SHALLOW", true: "DEEP"}[deep])
		var ledger *protocol.SystemLedger
		if err := batch.Account(protocol.PartitionUrl(part).JoinPath(protocol.Ledger)).Main().GetAs(&ledger); err == nil {
			fmt.Printf("system ledger: block %d\n", ledger.Index)
		} else {
			fmt.Printf("system ledger: %v\n", err)
		}
		pu := protocol.PartitionUrl(part)
		chains := map[string]*database.Chain2{
			"ledger.RootChain":        batch.Account(pu.JoinPath(protocol.Ledger)).RootChain(),
			"ledger.BlockLedgerChain": batch.Account(pu.JoinPath(protocol.Ledger)).BlockLedgerChain(),
			"anchors.MainChain":       batch.Account(pu.JoinPath(protocol.AnchorPool)).MainChain(),
			"anchors.anchor(dir)-root": batch.Account(pu.JoinPath(protocol.AnchorPool)).AnchorChain(protocol.Directory).Root(),
			"ledger.BptChain.Index":   batch.Account(pu.JoinPath(protocol.Ledger)).BptChain().Index(),
		}
		for name, c := range chains {
			head, err := c.Head().Get()
			if err != nil {
				fmt.Printf("%-26s head: %v\n", name, err)
				continue
			}
			inner := c.Inner()
			// the newest 12 elements: present or not
			var missing []int64
			var present int
			lo := head.Count - 12
			if lo < 0 {
				lo = 0
			}
			for i := lo; i < head.Count; i++ {
				if _, err := inner.Element(uint64(i)).Get(); err != nil {
					missing = append(missing, i)
				} else {
					present++
				}
			}
			// and the hashes the head's open chunk holds
			fmt.Printf("%-26s count=%d hashlist=%d newest12: present=%d missing=%v\n", name, head.Count, len(head.HashList), present, missing)
		}
		batch.Discard()
	}
	_ = url.URL{}
}
