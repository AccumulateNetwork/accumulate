// rebuild-chains takes a JSON file of (account_url -> chain_entries)
// pulled from the live mainnet API and replays those entries into a
// writable copy of a follower's accumulate.db. After the replay, it
// computes account.Hash() per account and compares against the stored
// BPT leaf — so we can answer the question:
//
//   "If we fill in the chain data the snapshot stripped, does
//    account.Hash() match the stored leaf?"
//
// Match → the stored leaves are correct; the local mismatch is
// purely a chain-data hole created by snapshot strip.
//
// Mismatch → either the stripped data was different from what the
// API exposes, or some other input (Main, Directory, Pending) also
// differs.
//
// The tool only WRITES to the chain entries (via Chain.AddEntry).
// It does NOT call UpdateBPT, so the stored leaf is untouched and
// the comparison is meaningful.
//
// Input JSON shape (one element per account):
//   {"url": "acc://...", "main_entries": ["<hex32>", ...],
//    "signature_entries": ["<hex32>", ...]}
package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type record struct {
	URL                   string   `json:"url"`
	Class                 string   `json:"class"`
	MainEntries           []string `json:"main_entries"`
	SignatureEntries      []string `json:"signature_entries"`
	MainIndexEntries      []string `json:"main_index_entries"`
	SignatureIndexEntries []string `json:"signature_index_entries"`
}

func main() {
	var (
		dbPath   = flag.String("db", "", "writable accumulate.db path (LevelDB)")
		jsonPath = flag.String("entries", "", "live-chains.json from fetch-live-chains.py")
	)
	flag.Parse()
	if *dbPath == "" || *jsonPath == "" {
		fmt.Fprintln(os.Stderr, "usage: rebuild-chains --db <path> --entries <json>")
		os.Exit(1)
	}

	raw, err := os.ReadFile(*jsonPath)
	must(err)
	var records []record
	must(json.Unmarshal(raw, &records))
	fmt.Printf("loaded %d account records\n", len(records))

	db, err := database.OpenLevelDB(*dbPath, log.NewNopLogger())
	must(err)
	defer db.Close()

	// Single batch for all writes; commit once at the end so we
	// don't cross any partial-state boundary.
	batch := db.Begin(true)
	defer batch.Discard()

	type result struct {
		url           *url.URL
		class         string
		stored        [32]byte
		before        [32]byte
		after         [32]byte
		appendedMain  int
		appendedSig   int
		buildErr      error
	}
	var results []*result

	for _, r := range records {
		u, err := url.Parse(r.URL)
		if err != nil {
			fmt.Printf("  parse %q: %v\n", r.URL, err)
			continue
		}

		acc := batch.Account(u)
		stored, _ := batch.BPT().Get(acc.Key())
		var sh [32]byte
		copy(sh[:], stored)

		before, beforeErr := acc.Hash()

		out := &result{url: u, class: r.Class, stored: sh, before: before}
		if beforeErr != nil {
			out.buildErr = fmt.Errorf("before-hash: %w", beforeErr)
			results = append(results, out)
			continue
		}

		// Replay every chain we have entries for, registering it in
		// the Chains index if it isn't present locally.
		for _, c := range []struct {
			name    string
			ctype   merkle.ChainType
			entries []string
		}{
			{"main", merkle.ChainTypeTransaction, r.MainEntries},
			{"signature", merkle.ChainTypeTransaction, r.SignatureEntries},
			{"main-index", merkle.ChainTypeIndex, r.MainIndexEntries},
			{"signature-index", merkle.ChainTypeIndex, r.SignatureIndexEntries},
		} {
			if len(c.entries) == 0 {
				continue
			}
			ch, err := loadOrRegister(acc, c.name, c.ctype)
			if err != nil {
				if out.buildErr == nil {
					out.buildErr = fmt.Errorf("register %s: %w", c.name, err)
				}
				continue
			}
			added, err := replay(ch, c.entries)
			if err != nil && out.buildErr == nil {
				out.buildErr = fmt.Errorf("%s replay: %w", c.name, err)
			}
			switch c.name {
			case "main":
				out.appendedMain = added
			case "signature":
				out.appendedSig = added
			}
		}

		// Recompute account.Hash() over the replayed state.
		// NB: do NOT call UpdateBPT() — we want the stored leaf to
		// remain whatever the snapshot saved.
		after, err := acc.Hash()
		if err != nil {
			if out.buildErr == nil {
				out.buildErr = fmt.Errorf("after-hash: %w", err)
			}
		}
		out.after = after
		results = append(results, out)
	}

	// Discard the batch — we want this to be a non-destructive analysis.
	// (Comment out the discard and switch to Commit() if you want the
	// rebuilt chains persisted.)
	_ = batch
	// batch.Commit()

	// Report.
	var (
		matches    int
		drifted    int
		errored    int
	)
	for i, r := range results {
		match := r.after == r.stored
		matchStr := "DRIFT"
		if match {
			matchStr = "MATCH"
			matches++
		} else {
			drifted++
		}
		status := ""
		if r.buildErr != nil {
			status = " [err: " + r.buildErr.Error() + "]"
			errored++
		}
		fmt.Printf("[%2d/%d] %s %-22s appended main=%d sig=%d  before=%x  after=%x  stored=%x%s\n",
			i+1, len(results), matchStr, r.class, r.appendedMain, r.appendedSig,
			r.before[:8], r.after[:8], r.stored[:8], status)
		if !match {
			fmt.Printf("       url=%s\n", r.url)
		}
	}

	fmt.Printf("\nsummary: %d MATCH, %d DRIFT, %d errored, %d total\n",
		matches, drifted, errored, len(results))
}

// loadOrRegister gets a chain by name, registering it in the account's
// Chains index if necessary so the chain shows up in observer.hashChains.
func loadOrRegister(acc *database.Account, name string, ctype merkle.ChainType) (*database.Chain, error) {
	// Make sure the chain is present in the Chains index. Add() is a
	// set-add on (name, type); idempotent if it's already registered.
	if err := acc.Chains().Add(&protocol.ChainMetadata{Name: name, Type: ctype}); err != nil {
		return nil, fmt.Errorf("add to chains index: %w", err)
	}
	mgr, err := acc.ChainByName(name)
	if err != nil {
		return nil, err
	}
	return mgr.Get()
}

func replay(ch *database.Chain, entries []string) (int, error) {
	added := 0
	have := ch.Height()
	for i, h := range entries {
		if int64(i) < have {
			continue // already present locally
		}
		raw, err := hex.DecodeString(h)
		if err != nil {
			return added, fmt.Errorf("entry %d: %w", i, err)
		}
		if err := ch.AddEntry(raw, false); err != nil {
			return added, fmt.Errorf("AddEntry %d: %w", i, err)
		}
		added++
	}
	return added, nil
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
