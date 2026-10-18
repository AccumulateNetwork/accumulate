package main

import (
	"fmt"
	"strconv"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// probe reads named elements of named chains: spec is chain:idx,idx,...
func probe(db *database.Database, part string, specs []string) {
	for _, deep := range []bool{false, true} {
		d := db
		if deep {
			d = db.Deep()
		}
		batch := d.Begin(false)
		pu := protocol.PartitionUrl(part)
		for _, spec := range specs {
			name, idxs, _ := strings.Cut(spec, ":")
			var c *database.Chain2
			switch name {
			case "root":
				c = batch.Account(pu.JoinPath(protocol.Ledger)).RootChain()
			case "blockledger":
				c = batch.Account(pu.JoinPath(protocol.Ledger)).BlockLedgerChain()
			case "anchors-main":
				c = batch.Account(pu.JoinPath(protocol.AnchorPool)).MainChain()
			case "bpt-index":
				c = batch.Account(pu.JoinPath(protocol.Ledger)).BptChain().Index()
			}
			for _, s := range strings.Split(idxs, ",") {
				i, _ := strconv.Atoi(s)
				_, eerr := c.Inner().Element(uint64(i)).Get()
				_, serr := c.Inner().States(uint64(i)).Get()
				fmt.Printf("%-7s %-12s element(%d)=%s state(%d)=%s\n", map[bool]string{false: "SHALLOW", true: "DEEP"}[deep], name, i, okOr(eerr), i, okOr(serr))
			}
		}
		batch.Discard()
	}
}

func okOr(err error) string {
	if err == nil {
		return "ok"
	}
	return "MISSING"
}

// tails prints the Tail chunk records of the mark set holding index i.
func tails(db *database.Database, part, chain string, i int64, markPower, chunkSize int64) {
	for _, deep := range []bool{false, true} {
		d := db
		if deep {
			d = db.Deep()
		}
		batch := d.Begin(false)
		pu := protocol.PartitionUrl(part)
		var c *database.Chain2
		switch chain {
		case "root":
			c = batch.Account(pu.JoinPath(protocol.Ledger)).RootChain()
		case "blockledger":
			c = batch.Account(pu.JoinPath(protocol.Ledger)).BlockLedgerChain()
		case "anchors-main":
			c = batch.Account(pu.JoinPath(protocol.AnchorPool)).MainChain()
		}
		markMask := int64(1)<<markPower - 1
		for k := int64(0); k < (markMask+1)/chunkSize; k++ {
			ch, err := c.Inner().Tail(uint64(k)).Get()
			if err != nil {
				fmt.Printf("%-7s %s Tail(%d): %v\n", map[bool]string{false: "SHALLOW", true: "DEEP"}[deep], chain, k, okOr(err))
				continue
			}
			fmt.Printf("%-7s %s Tail(%d): index=%d hashes=%d\n", map[bool]string{false: "SHALLOW", true: "DEEP"}[deep], chain, k, ch.Index, len(ch.Hashes))
		}
		batch.Discard()
	}
}
