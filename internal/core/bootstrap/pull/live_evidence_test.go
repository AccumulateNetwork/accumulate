// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

// Throwaway probes run against a live Docker network. Skipped unless
// ACC_LIVE_V3 names a v3 endpoint.

import (
	"context"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func live(t *testing.T) api.Querier2 {
	t.Helper()
	s := os.Getenv("ACC_LIVE_V3")
	if s == "" {
		t.Skip("set ACC_LIVE_V3 to a v3 endpoint")
	}
	return api.Querier2{Querier: jsonrpc.NewClient(s)}
}

// TestLivePullComponents fetches an account exactly as the join's state pull
// does (Fetch, ModeStateOnly, with a receipt) and reports which of the four
// components of the account hash -- main, secondary, chains, pending --
// disagrees with the receipt the peer served with it.
//
// The peer's receipt runs from element 0 of its account hasher (the main state
// hash) to the peer's BPT root, so the first two entries of that receipt are
// the peer's own secondary hash and its combined chains+pending hash.
func TestLivePullComponents(t *testing.T) {
	q := live(t)
	part := os.Getenv("ACC_LIVE_PART")
	if part == "" {
		part = "acc://bvn-BVN1.acme"
	}
	pu := url.MustParse(part)
	ctx := context.Background()

	for _, name := range []string{protocol.AnchorPool, protocol.Ledger, protocol.Synthetic} {
		u := pu.JoinPath(name)
		t.Run(name, func(t *testing.T) {
			for attempt := 0; attempt < 4; attempt++ {
				db := database.OpenInMemory(nil)
				db.SetObserver(database.NewDatabaseObserver())
				batch := db.Begin(true)

				start := time.Now()
				p, err := Fetch(ctx, q, batch, u, Options{Mode: ModeStateOnly, Partition: pu}, true)
				took := time.Since(start)
				if err != nil {
					t.Logf("attempt %d: fetch failed: %v", attempt, err)
					batch.Discard()
					continue
				}

				local, err := p.batch.Account(u).StateTreeReceipt()
				if err != nil {
					t.Fatalf("local receipt: %v", err)
				}
				peer := p.receipt.Receipt

				t.Logf("attempt %d: %v servedAtBlock=%d fetch took %v", attempt, u, p.Block, took)
				t.Logf("  local element0 %s", hex.EncodeToString(local.Start))
				t.Logf("  peer  element0 %s", hex.EncodeToString(peer.Start))
				n := len(local.Entries)
				if n > len(peer.Entries) {
					n = len(peer.Entries)
				}
				names := []string{"secondary(dir+events+queues)", "chains+pending"}
				for i := 0; i < n; i++ {
					lab := "bpt-path"
					if i < len(names) {
						lab = names[i]
					}
					l := hex.EncodeToString(local.Entries[i].Hash)
					r := hex.EncodeToString(peer.Entries[i].Hash)
					mark := "  same"
					if l != r {
						mark = "  DIFF"
					}
					t.Logf("%s entry[%d] %-30s local=%s peer=%s", mark, i, lab, l[:16], r[:16])
				}
				t.Logf("  Contains(local) = %v", peer.Contains(local))

				// Per-chain: the head the peer served vs the head the pull
				// reproduced locally.
				chains, err := q.QueryAccountChains(ctx, u, &api.ChainQuery{})
				if err == nil {
					for _, c := range chains.Records {
						if c == nil || c.Name == "" {
							continue
						}
						dst, err := p.batch.Account(u).ChainByName(c.Name)
						if err != nil {
							t.Logf("    chain %-28s LOCAL MISSING: %v", c.Name, err)
							continue
						}
						head, err := dst.Inner().Head().Get()
						if err != nil {
							t.Logf("    chain %-28s head error: %v", c.Name, err)
							continue
						}
						t.Logf("    chain %-28s localCount=%-6d peerCountNow=%-6d", c.Name, head.Count, c.Count)
					}
					idx, err := p.batch.Account(u).Chains().Get()
					if err == nil {
						var got []string
						for _, m := range idx {
							got = append(got, m.Name)
						}
						t.Logf("    local chain index order: %v", got)
					}
				}
				p.Discard()
				batch.Discard()
			}
		})
	}
}

// TestLiveAnchorStride reports how many distinct blocks of each partition the
// Directory has anchored a state tree root for. Pending.Block is the block a
// peer served an account at -- any block -- and AnchoredRoot is an exact
// lookup, so a block the Directory never anchors can never settle.
func TestLiveAnchorStride(t *testing.T) {
	q := live(t)
	ctx := context.Background()
	pool := protocol.DnUrl().JoinPath(protocol.AnchorPool)

	chains, err := q.QueryAccountChains(ctx, pool, &api.ChainQuery{})
	if err != nil {
		t.Fatal(err)
	}
	var total uint64
	for _, c := range chains.Records {
		if c.Name == "main" {
			total = c.Count
		}
	}
	t.Logf("dn.acme/anchors main chain: %d entries", total)

	start := uint64(0)
	if total > 400 {
		start = total - 400
	}
	blocks := map[string]map[uint64]bool{}
	var order []string
	count, expand := uint64(64), true
	for s := start; s < total; s += 64 {
		page, err := q.QueryMainChainEntries(ctx, pool, &api.ChainQuery{
			Name: "main", Range: &api.RangeOptions{Start: s, Count: &count, Expand: &expand},
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, rec := range page.Records {
			if rec.Value == nil || rec.Value.Message == nil || rec.Value.Message.Transaction == nil {
				continue
			}
			body, ok := rec.Value.Message.Transaction.Body.(protocol.AnchorBody)
			if !ok {
				continue
			}
			a := body.GetPartitionAnchor()
			if a == nil || a.Source == nil {
				continue
			}
			k := a.Source.String()
			if blocks[k] == nil {
				blocks[k] = map[uint64]bool{}
				order = append(order, k)
			}
			blocks[k][a.MinorBlockIndex] = true
		}
	}
	for _, k := range order {
		var lo, hi uint64
		first := true
		for b := range blocks[k] {
			if first || b < lo {
				lo = b
			}
			if first || b > hi {
				hi = b
			}
			first = false
		}
		span := hi - lo + 1
		t.Logf("%-24s anchored %3d distinct blocks over [%d,%d] (%d blocks): %.1f%% of blocks have an anchored root, stride %.2f",
			k, len(blocks[k]), lo, hi, span,
			100*float64(len(blocks[k]))/float64(span),
			float64(span)/float64(len(blocks[k])))
	}
}
