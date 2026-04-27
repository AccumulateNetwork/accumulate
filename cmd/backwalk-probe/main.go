// Throwaway prototype for issue #3953.
// Walks an account's main chain backwards using existing query APIs and
// reports what it sees: entry counts, unique signer keybooks, time taken,
// any obvious oddities. No verification is performed yet — this is purely
// an empirical probe to validate the back-walk model's premises before
// committing to the full implementation.
package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func main() {
	var (
		network    = flag.String("network", "mainnet", "well-known network name or endpoint URL")
		acctFlag   = flag.String("account", "dn.acme/operators", "account whose main chain to walk backwards")
		chainFlag  = flag.String("chain", "main", "chain name to walk")
		pageSize   = flag.Uint64("page", 100, "entries per page")
		maxEntries = flag.Uint64("max", 5000, "max entries to walk (0 = unlimited)")
		verbose    = flag.Bool("v", false, "verbose: print each entry")
	)
	flag.Parse()

	endpoint := accumulate.ResolveWellKnownEndpoint(*network, "v3")
	fmt.Printf("Endpoint: %s\n", endpoint)
	client := jsonrpc.NewClient(endpoint)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	acct, err := url.Parse(*acctFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse account url: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Account:  %s\n", acct)

	// First query the chain head to learn its length.
	headRec, err := client.Query(ctx, acct, &api.ChainQuery{Name: *chainFlag})
	if err != nil {
		fmt.Fprintf(os.Stderr, "query chain head: %v\n", err)
		os.Exit(1)
	}
	chainRec, ok := headRec.(*api.ChainRecord)
	if !ok {
		fmt.Fprintf(os.Stderr, "unexpected record type for chain head: %T\n", headRec)
		os.Exit(1)
	}
	fmt.Printf("Chain:    %s (%s) count=%d\n", chainRec.Name, chainRec.Type, chainRec.Count)

	if chainRec.Count == 0 {
		fmt.Println("Empty chain. Nothing to walk.")
		return
	}

	// Walk backwards in pages of pageSize. The chain has chainRec.Count
	// entries; we start from index Count-pageSize and decrement.
	var (
		started    = time.Now()
		totalSeen  uint64
		pages      int
		expanded   int
		unexpanded int
		signerSeen = map[string]uint64{} // signer URL -> count of entries it signed
		causeSeen  = map[string]uint64{} // cause TxId -> count
		statuses   = map[string]uint64{} // execution status histogram
		earliest   *time.Time            // earliest LastBlockTime we observed
		latest     *time.Time            // latest LastBlockTime we observed
		syntheticN int                   // entries that look synthetic (have Cause)
	)

	end := chainRec.Count
	for end > 0 {
		if *maxEntries > 0 && totalSeen >= *maxEntries {
			break
		}
		count := *pageSize
		if count > end {
			count = end
		}
		start := end - count

		expand := true
		page, err := client.Query(ctx, acct, &api.ChainQuery{
			Name: *chainFlag,
			Range: &api.RangeOptions{
				Start:  start,
				Count:  &count,
				Expand: &expand,
			},
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "query page [%d,%d): %v\n", start, end, err)
			os.Exit(1)
		}
		pages++

		rr, ok := page.(*api.RecordRange[api.Record])
		if !ok {
			fmt.Fprintf(os.Stderr, "unexpected page record type: %T\n", page)
			os.Exit(1)
		}

		// Iterate within page from highest to lowest index.
		entries := make([]api.Record, len(rr.Records))
		copy(entries, rr.Records)
		sort.SliceStable(entries, func(i, j int) bool {
			ai, _ := entries[i].(*api.ChainEntryRecord[api.Record])
			aj, _ := entries[j].(*api.ChainEntryRecord[api.Record])
			if ai == nil || aj == nil {
				return false
			}
			return ai.Index > aj.Index
		})

		for _, rec := range entries {
			cer, ok := rec.(*api.ChainEntryRecord[api.Record])
			if !ok {
				fmt.Fprintf(os.Stderr, "page contained non-chain-entry record: %T\n", rec)
				continue
			}
			totalSeen++

			if cer.LastBlockTime != nil {
				t := *cer.LastBlockTime
				if earliest == nil || t.Before(*earliest) {
					earliest = &t
				}
				if latest == nil || t.After(*latest) {
					latest = &t
				}
			}

			mr, isMsg := cer.Value.(*api.MessageRecord[messaging.Message])
			if !isMsg || mr == nil {
				unexpanded++
				if *verbose {
					fmt.Printf("  %d: %s  (unexpanded value=%T)\n",
						cer.Index, hex.EncodeToString(cer.Entry[:8]), cer.Value)
				}
				continue
			}
			expanded++

			statuses[mr.Status.String()]++

			// Collect signer URLs from the SignatureSetRecord list.
			var signersThis []string
			if mr.Signatures != nil {
				for _, set := range mr.Signatures.Records {
					if set == nil || set.Account == nil {
						continue
					}
					u := set.Account.GetUrl()
					if u == nil {
						continue
					}
					s := u.String()
					signerSeen[s]++
					signersThis = append(signersThis, s)
				}
			}

			// Cause records mark synthetic transactions.
			if mr.Cause != nil && len(mr.Cause.Records) > 0 {
				syntheticN++
				for _, c := range mr.Cause.Records {
					if c == nil || c.Value == nil {
						continue
					}
					causeSeen[c.Value.String()]++
				}
			}

			if *verbose {
				blk := ""
				if cer.LastBlockTime != nil {
					blk = cer.LastBlockTime.Format(time.RFC3339)
				}
				kind := ""
				if mr.Message != nil {
					kind = fmt.Sprintf("%T", mr.Message)
				}
				causeN := 0
				if mr.Cause != nil {
					causeN = len(mr.Cause.Records)
				}
				fmt.Printf("  %d  %s  %s  signers=%v  cause=%d  msg=%s\n",
					cer.Index, hex.EncodeToString(cer.Entry[:8]), blk,
					signersThis, causeN, kind)
			}
		}

		end = start
	}

	elapsed := time.Since(started)

	fmt.Println()
	fmt.Println("=== Back-walk probe results ===")
	fmt.Printf("Pages fetched:     %d\n", pages)
	fmt.Printf("Entries seen:      %d (chain has %d total)\n", totalSeen, chainRec.Count)
	fmt.Printf("  expanded:        %d\n", expanded)
	fmt.Printf("  unexpanded:      %d\n", unexpanded)
	fmt.Printf("  synthetic:       %d\n", syntheticN)
	fmt.Printf("Walk time:         %v\n", elapsed)
	if totalSeen > 0 {
		fmt.Printf("Avg per entry:     %v\n", elapsed/time.Duration(totalSeen))
	}
	if earliest != nil {
		fmt.Printf("Earliest block:    %s\n", earliest.Format(time.RFC3339))
	}
	if latest != nil {
		fmt.Printf("Latest block:      %s\n", latest.Format(time.RFC3339))
	}
	fmt.Printf("Distinct signers:  %d\n", len(signerSeen))

	if len(signerSeen) > 0 {
		fmt.Println("\nTop signer keybooks (by entries signed):")
		printTop(signerSeen, 20)
	}
	if len(causeSeen) > 0 {
		fmt.Printf("\nDistinct producing transactions (Cause): %d\n", len(causeSeen))
	}
	if len(statuses) > 0 {
		fmt.Println("\nExecution status distribution:")
		for s, n := range statuses {
			fmt.Printf("  %-20s %d\n", s, n)
		}
	}
}

func printTop(m map[string]uint64, n int) {
	type kv struct {
		k string
		v uint64
	}
	xs := make([]kv, 0, len(m))
	for k, v := range m {
		xs = append(xs, kv{k, v})
	}
	sort.Slice(xs, func(i, j int) bool { return xs[i].v > xs[j].v })
	if n > len(xs) {
		n = len(xs)
	}
	for i := 0; i < n; i++ {
		fmt.Printf("  %6d  %s\n", xs[i].v, xs[i].k)
	}
}
