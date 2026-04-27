// Throwaway driver that exercises the bootstrap algorithm against a
// real Accumulate Badger database (e.g., a local mainnet follower's
// data dir). Proves the in-process algorithm holds up on real chain
// data before any wire-protocol work is done.
//
// Usage:
//   bootstrap-driver -data <path> -account <url> [-time RFC3339] [-walk]
//
// Modes:
//   -resolve (default): runs keybookat.Resolve and dumps the resolved pages.
//   -walk:              runs walker.Walk and reports VerifiedEntry stats.
//
// The follower process MUST NOT be running against the same data dir
// (Badger holds a write lock). Either pause the follower or copy its
// data dir first.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/backwalk"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/keybookat"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func main() {
	var (
		dataDir = flag.String("data", "", "Accumulate data directory (Badger path)")
		acctStr = flag.String("account", "dn.acme/operators", "account URL to probe")
		timeStr = flag.String("time", "", "block time as RFC3339 (default: now)")
		mode    = flag.String("mode", "resolve", "mode: resolve | walk")
		genesis = flag.String("genesis-hash", "", "pinned genesis snapshot hash, hex (32 bytes); zero for none")
		verbose = flag.Bool("verbose", false, "verbose output")
	)
	flag.Parse()

	if *dataDir == "" {
		fmt.Fprintf(os.Stderr, "Error: -data is required\n")
		flag.Usage()
		os.Exit(1)
	}

	acct, err := url.Parse(*acctStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse account url: %v\n", err)
		os.Exit(1)
	}

	t := time.Now()
	if *timeStr != "" {
		t, err = time.Parse(time.RFC3339, *timeStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse -time: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Printf("Data dir:  %s\n", *dataDir)
	fmt.Printf("Account:   %s\n", acct)
	fmt.Printf("Block time: %s\n", t.Format(time.RFC3339))
	fmt.Printf("Mode:      %s\n", *mode)

	db, err := database.OpenBadger(*dataDir, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open badger: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	batch := db.Begin(false)
	defer batch.Discard()

	switch *mode {
	case "resolve":
		runResolve(batch, acct, t, *verbose)
	case "walk":
		runWalk(batch, acct, t, *genesis, *verbose)
	default:
		fmt.Fprintf(os.Stderr, "unknown mode %q (want resolve|walk)\n", *mode)
		os.Exit(1)
	}
}

func runResolve(batch *database.Batch, acct *url.URL, t time.Time, verbose bool) {
	started := time.Now()
	res, err := keybookat.Resolve(batch, acct, t)
	elapsed := time.Since(started)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Resolve: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("=== Resolve result ===")
	fmt.Printf("Walk time:    %v\n", elapsed)
	fmt.Printf("Pages found:  %d\n", len(res.Pages))
	for i, p := range res.Pages {
		if p == nil {
			fmt.Printf("  page %d: nil\n", i+1)
			continue
		}
		fmt.Printf("  page %d: url=%s version=%d keys=%d threshold=%d\n",
			i+1, p.Url, p.Version, len(p.Keys), p.AcceptThreshold)
		if verbose {
			for j, k := range p.Keys {
				delegate := "(none)"
				if k.Delegate != nil {
					delegate = k.Delegate.String()
				}
				fmt.Printf("    key[%d]: hash=%x delegate=%s\n",
					j, k.PublicKeyHash, delegate)
			}
		}
	}
}

func runWalk(batch *database.Batch, acct *url.URL, t time.Time, genesisHex string, verbose bool) {
	var pinned [32]byte
	if genesisHex != "" {
		var n int
		_, err := fmt.Sscanf(genesisHex, "%x", &pinned)
		if err != nil || n == 0 {
			// Use Sscanf with a pointer-receiver array works oddly; fall back to manual decode.
			b, err := decodeHexFixed(genesisHex)
			if err != nil {
				fmt.Fprintf(os.Stderr, "parse -genesis-hash: %v\n", err)
				os.Exit(1)
			}
			pinned = b
		}
	}

	w := backwalk.New(backwalk.Options{PinnedGenesisHash: pinned})

	started := time.Now()
	earliest, err := w.Walk(batch, acct, t)
	elapsed := time.Since(started)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Walk: %v\n", err)
		fmt.Println()
		fmt.Println("=== Walk result (partial) ===")
		fmt.Printf("Walk time:        %v\n", elapsed)
		fmt.Printf("Memoizations:     %d\n", w.MemoSize())
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("=== Walk result ===")
	fmt.Printf("Walk time:        %v\n", elapsed)
	fmt.Printf("Memoizations:     %d\n", w.MemoSize())
	if earliest == nil {
		fmt.Println("Earliest entry:   nil (chain has no in-window entries?)")
		return
	}
	fmt.Printf("Earliest tx:      %x\n", earliest.TxHash)
	fmt.Printf("Earliest time:    %s\n", earliest.BlockTime.Format(time.RFC3339))
	fmt.Printf("Synthetic:        %v\n", earliest.Synthetic)
	fmt.Printf("QuorumPending:    %v\n", earliest.QuorumPending)
	fmt.Printf("GenesisTerm:      %v\n", earliest.GenesisTerm)
	if earliest.SignerUrl != nil {
		fmt.Printf("Signer:           %s\n", earliest.SignerUrl)
	}
	if len(earliest.Causes) > 0 {
		fmt.Printf("Cause links:      %d\n", len(earliest.Causes))
		if verbose {
			for _, c := range earliest.Causes {
				fmt.Printf("  - %s\n", c)
			}
		}
	}
}

// decodeHexFixed parses a 32-byte hex string into a [32]byte.
func decodeHexFixed(s string) ([32]byte, error) {
	var out [32]byte
	if len(s) != 64 {
		return out, fmt.Errorf("expected 64 hex chars, got %d", len(s))
	}
	for i := 0; i < 32; i++ {
		var b byte
		_, err := fmt.Sscanf(s[i*2:i*2+2], "%02x", &b)
		if err != nil {
			return out, fmt.Errorf("decode hex byte %d: %w", i, err)
		}
		out[i] = b
	}
	return out, nil
}
