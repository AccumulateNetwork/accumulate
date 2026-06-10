package main

import (
	"flag"
	"fmt"
	"os"

	cometbftdb "github.com/cometbft/cometbft-db"
	"github.com/cometbft/cometbft/store"
)

func main() {
	from := flag.Int64("from", 0, "start height")
	to := flag.Int64("to", 0, "end height (exclusive)")
	step := flag.Int64("step", 1, "sample step")
	flag.Parse()
	dataDir := flag.Arg(0)
	db, _ := cometbftdb.NewDB("blockstore", cometbftdb.GoLevelDBBackend, dataDir)
	defer db.Close()
	bs := store.NewBlockStore(db)
	if *from == 0 {
		*from = bs.Base()
	}
	if *to == 0 {
		*to = bs.Height() + 1
	}
	fmt.Fprintf(os.Stderr, "scan h=[%d,%d) step=%d (height=%d)\n", *from, *to, *step, bs.Height())
	var nonempty, scanned, totalTxs int64
	for h := *from; h < *to; h += *step {
		blk := bs.LoadBlock(h)
		if blk == nil {
			continue
		}
		scanned++
		if n := len(blk.Data.Txs); n > 0 {
			nonempty++
			totalTxs += int64(n)
			if nonempty <= 10 {
				fmt.Printf("[h=%d] txs=%d (%v)\n", h, n, blk.Time)
			}
		}
	}
	fmt.Fprintf(os.Stderr, "scanned %d blocks, %d non-empty, %d total txs\n", scanned, nonempty, totalTxs)
}
