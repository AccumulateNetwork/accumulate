// blockstore-probe opens a CometBFT blockstore.db read-only and
// reports its height range plus the principal URLs from the first
// few blocks. Used to verify we can decode the on-disk blocks back
// into Accumulate envelopes before scaling up to a full walk.
//
// Usage: blockstore-probe <data-dir> [--height N]
//   <data-dir> is the directory containing blockstore.db (e.g.
//   .../bvnn/data). The blockstore.db itself is the LevelDB inside.
package main

import (
	"flag"
	"fmt"
	"os"

	cometbftdb "github.com/cometbft/cometbft-db"
	"github.com/cometbft/cometbft/store"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

func main() {
	var (
		height = flag.Int64("height", 0, "load this block height (0 = first three after Base)")
	)
	flag.Parse()
	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: blockstore-probe <data-dir> [--height N]")
		os.Exit(1)
	}
	dataDir := flag.Arg(0)

	db, err := cometbftdb.NewDB("blockstore", cometbftdb.GoLevelDBBackend, dataDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open blockstore:", err)
		os.Exit(1)
	}
	defer db.Close()

	bs := store.NewBlockStore(db)
	fmt.Printf("blockstore: base=%d height=%d size=%d\n", bs.Base(), bs.Height(), bs.Size())

	heights := []int64{}
	if *height != 0 {
		heights = []int64{*height}
	} else {
		heights = []int64{bs.Base(), bs.Base() + 1, bs.Base() + 2, bs.Height() - 1, bs.Height()}
	}

	for _, h := range heights {
		if h <= 0 || h > bs.Height() {
			continue
		}
		blk := bs.LoadBlock(h)
		if blk == nil {
			fmt.Printf("[h=%d] LoadBlock returned nil\n", h)
			continue
		}
		fmt.Printf("[h=%d] time=%v txs=%d\n", h, blk.Time, len(blk.Data.Txs))
		for i, raw := range blk.Data.Txs {
			fmt.Printf("  tx[%d] %dB\n", i, len(raw))
			env := new(messaging.Envelope)
			err := env.UnmarshalBinary(raw)
			if err != nil {
				fmt.Printf("    decode err=%v\n", err)
				continue
			}
			fmt.Printf("    sigs=%d txns=%d msgs=%d\n", len(env.Signatures), len(env.Transaction), len(env.Messages))
			for j, m := range env.Messages {
				fmt.Printf("    msg[%d] type=%v\n", j, m.Type())
				if tm, ok := m.(*messaging.TransactionMessage); ok && tm.Transaction != nil {
					principal := "?"
					if tm.Transaction.Header.Principal != nil {
						principal = tm.Transaction.Header.Principal.String()
					}
					body := "?"
					if tm.Transaction.Body != nil {
						body = tm.Transaction.Body.Type().String()
					}
					if len(principal) > 70 {
						principal = principal[:67] + "..."
					}
					fmt.Printf("      txn body=%-22s principal=%s\n", body, principal)
				}
			}
			for j, t := range env.Transaction {
				principal := "?"
				if t.Header.Principal != nil {
					principal = t.Header.Principal.String()
				}
				body := "?"
				if t.Body != nil {
					body = fmt.Sprintf("%v", t.Body.Type())
				}
				if len(principal) > 70 {
					principal = principal[:67] + "..."
				}
				fmt.Printf("  tx[%d].txn[%d] body=%-22s principal=%s sigs=%d\n",
					i, j, body, principal, len(env.Signatures))
			}
		}
	}
}
