// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package cyclopsrepair carries the canonical list of accounts whose
// BPT entries were dropped or stripped on the Cyclops BVN during the
// post-reorg single-validator window. The list is consumed by:
//
//   - The executor's one-shot V2CyclopsBptRepair routine
//     (internal/core/execute/v2/block/cyclops_bpt_repair.go), which
//     re-registers chains and marks each account dirty so block_end's
//     UpdateBPT refreshes the leaf.
//
//   - The bootstrap-v3 launcher's enumerate hydrate pass
//     (internal/core/bootstrap/orchestrator/orchestrator.go), which
//     skips state-pull for these URLs so the enumerate-recorded leaf
//     hash is preserved instead of being overwritten with a recomputed
//     value that diverges from the source's BPT.
//
// Both consumers reference the same list to avoid drift. The list is
// scoped per-partition: only the Cyclops BVN is affected, so other
// partitions (DN, other BVNs, simulator nets, devnet, testnet) are
// never matched even if a URL happens to coincide.
//
// Once the network activates V2CyclopsBptRepair and the bootstrap
// exception path is no longer needed, the bootstrap consumer can drop
// its dependency on this package; the executor consumer can drop its
// dependency once the activation is committed and old binaries are
// retired.
//
// See docs/incidents/2026-05-cyclops-bpt-drift.md for the full
// investigation methodology and per-account details.
package cyclopsrepair

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Partition is the partition ID where the affected accounts live.
// The exception logic is scoped to this partition so that other
// partitions (and simulator / devnet networks that may reuse some of
// the URL strings) are never matched.
const Partition = "Cyclops"

// cyclopsTargets is the 22-account list on the Cyclops BVN —
// 7 ADIs delegated to marketplace.acme/book, 4 LiteIdentities,
// 3 LiteTokenAccounts, 3 live LiteDataAccounts, 4 body-less orphans,
// and 1 dn.acme/network phantom that lives in the BVN BPT.
var cyclopsTargets = []string{
	// 7 ADIs delegated to marketplace.acme/book.
	"acc://aber.acme",
	"acc://chimneypiece.acme",
	"acc://corrupted.acme",
	"acc://csrc.acme",
	"acc://nadro.acme",
	"acc://treble.acme",
	"acc://zagg.acme",

	// 4 LiteIdentities.
	"acc://45875c282cbf0265fc2369cfc420ab7658f9c378b257608f",
	"acc://981fabf9e5447ead08f2bb1dd7eed3282864ad20a7fc0e1e",
	"acc://ca6c6f2b20ac4fe16cf0e2a6dd1e6d8ccfce21df3fe22468",
	"acc://cb5a976eea2b84a9c78263984bc4ebf205ce99e2d2bfea01",

	// 3 LiteTokenAccounts.
	"acc://1570f386a1cd332a5a33beee62b0dd23df2a08bb74d23f1e/PegNet.acme/assets/peg",
	"acc://78432e204c43d61286daa3800cf462f8a02d8828fdd294b3/ACME",
	"acc://ca59e9e6c08ed245324f4c52e61defe34ab95a15abdcc802/PegNet.acme/assets/rvn",

	// 3 live LiteDataAccounts.
	"acc://2d7b3f44935ee7de9e99766f995aa4afbc3bb9ff3dfebd9aaa8e670f178bc83c",
	"acc://79f09991516f7b88c507c554bc13aa659d9bfff54467a0a4a4372f3468e88bd8",
	"acc://c2db482c10bfa53099a06555d3dc5307076138a8bd003757b18f8d9c181a41c6",

	// 4 body-less orphans (3 LDAs + 1 ADI).
	"acc://675f6bdb8c270e4a573970c69c5ebac7a86c7b3dfa47b41e0fd522ab16654d55",
	"acc://99a480cecc61722afebe6de6874ed42c2a847ff413ffd5f0f1e2243d27b630d8",
	"acc://ab7a5ed9d394389090a62fa199b9b76624de9f2ef6c77fae77b2e8fa8723fdef",
	"acc://kmutt.acme",

	// 1 dn.acme/network — DN-side phantom in the BVN BPT.
	"acc://dn.acme/network",
}

// targetSetByPartition is a per-partition lookup of canonical URL
// strings. Built once at package init.
var targetSetByPartition map[string]map[string]struct{}

func init() {
	targetSetByPartition = map[string]map[string]struct{}{
		Partition: make(map[string]struct{}, len(cyclopsTargets)),
	}
	for _, s := range cyclopsTargets {
		u, err := url.Parse(s)
		if err != nil {
			panic("cyclopsrepair: malformed target URL " + s + ": " + err.Error())
		}
		targetSetByPartition[Partition][u.String()] = struct{}{}
	}
}

// Targets returns the affected account URLs for the given partition.
// Returns nil for any partition without an affected list (DN, other
// BVNs, simulator nets, etc). The returned slice is freshly allocated;
// callers may modify it without affecting the package state.
func Targets(partition string) []*url.URL {
	if partition != Partition {
		return nil
	}
	out := make([]*url.URL, 0, len(cyclopsTargets))
	for _, s := range cyclopsTargets {
		u, err := url.Parse(s)
		if err != nil {
			// init already validated each target; reaching this means
			// the package state was corrupted at runtime.
			panic("cyclopsrepair: malformed target URL " + s + ": " + err.Error())
		}
		out = append(out, u)
	}
	return out
}

// IsTarget reports whether u is one of the affected accounts on
// partition. Returns false for nil URL, an empty partition, a
// partition without an affected list, or a URL not on that partition's
// list. Comparison is on the canonical url.URL.String() form.
func IsTarget(partition string, u *url.URL) bool {
	if u == nil || partition == "" {
		return false
	}
	set, ok := targetSetByPartition[partition]
	if !ok {
		return false
	}
	_, ok = set[u.String()]
	return ok
}

// Count reports the number of affected accounts on partition. Returns
// 0 for any partition without an affected list. Stable across releases
// until the cleanup MR drops this package.
func Count(partition string) int {
	set, ok := targetSetByPartition[partition]
	if !ok {
		return 0
	}
	return len(set)
}
