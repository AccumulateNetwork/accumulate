// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"encoding/hex"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// cyclopsBptRepairTarget describes one account whose BPT leaf was
// corrupted during the post-reorg single-validator window on Cyclops.
//
// The repair consists of:
//
//   - Re-registering the chains listed in Chains in the account's
//     Chains() index (a no-op if already present).
//
//   - For each chain, ensuring the head reflects the entries listed
//     in Entries by appending any that are not already present.
//
//   - Marking the account dirty so that block_end's UpdateBPT call
//     refreshes the stored BPT leaf to match the recomputed
//     account.Hash().
//
// All operations are idempotent. Running the repair twice produces
// the same result as running it once.
type cyclopsBptRepairTarget struct {
	URL    string
	Chains []cyclopsRepairChain
}

type cyclopsRepairChain struct {
	Name    string
	Type    merkle.ChainType
	Entries []string // hex-encoded chain entry hashes (in order)
}

// cyclopsBptRepairTargets is the complete per-partition list of
// accounts whose BPT entries were dropped or stripped during the
// single-validator window. The lists were derived from:
//
//   - snap-bpt-stale against the Cyclops follower DB at
//     /mnt/secondary/.../accman/follower-data/bvnn/data/accumulate.db
//     (BPT root 4364ea01d2e7092a202729d68f7740973b592dee8529e729b4b17fa84a88e5d7).
//
//   - find-dropped against the same DB cross-referenced with the
//     pre-reorg backups at /mnt/secondary/databases7-13-25-restored/.
//
//   - find-silent (with signer-aware tx set) confirming no other
//     account had silent state changes.
//
// See docs/incidents/2026-05-cyclops-bpt-drift.md for the full
// investigation methodology and per-account details.
var cyclopsBptRepairTargets = map[string][]cyclopsBptRepairTarget{
	"Cyclops": cyclopsBvnTargets,
}

const genesisTxHash = "e43be90e349210456662d8b8bdc9cc9e5e46ccb07f2129e7b57a8195e5e916d5"

// cyclopsBvnTargets — 22 accounts on the Cyclops BVN. Each one needs
// its Chains() index restored and the account marked dirty so its BPT
// leaf is recomputed.
//
// The 7 ADIs and 4 LiteIdentities + 3 LTAs + 3 live LDAs are Class A
// (chain-registry strips with intact bodies); their leaves recompute
// cleanly once the Chains() index is restored.
//
// The 4 orphans (3 body-less LDAs + kmutt.acme) and the dn.acme/network
// phantom are Class B (body absent). For those the repair clears the
// stored leaf to the empty-state hash, which will match the recomputed
// account.Hash() over the body-less state and resolves the inconsistency.
var cyclopsBvnTargets = []cyclopsBptRepairTarget{
	// 7 ADIs delegated to marketplace.acme/book.
	{URL: "acc://aber.acme", Chains: standardAdiChains(genesisTxHash, genesisTxHash)},
	{URL: "acc://chimneypiece.acme", Chains: standardAdiChains(genesisTxHash, genesisTxHash)},
	{URL: "acc://corrupted.acme", Chains: standardAdiChains(genesisTxHash, genesisTxHash)},
	{URL: "acc://csrc.acme", Chains: standardAdiChains(genesisTxHash, genesisTxHash)},
	{URL: "acc://nadro.acme", Chains: standardAdiChains(genesisTxHash, genesisTxHash)},
	{URL: "acc://treble.acme", Chains: standardAdiChains(genesisTxHash, genesisTxHash)},
	{URL: "acc://zagg.acme", Chains: standardAdiChains(genesisTxHash, genesisTxHash)},

	// 4 LiteIdentities — each has 3 entries on its main chain on the
	// live network. The first 2 are the genesis-pair pattern; the
	// third is account-specific. Where we don't have the live-side
	// entry hash embedded, the repair adds the chain registry only,
	// which is sufficient to make hashChains include the chain even
	// if the entry list isn't fully reconstructed locally.
	{URL: "acc://45875c282cbf0265fc2369cfc420ab7658f9c378b257608f", Chains: standardLiteChains()},
	{URL: "acc://981fabf9e5447ead08f2bb1dd7eed3282864ad20a7fc0e1e", Chains: standardLiteChains()},
	{URL: "acc://ca6c6f2b20ac4fe16cf0e2a6dd1e6d8ccfce21df3fe22468", Chains: standardLiteChains()},
	{URL: "acc://cb5a976eea2b84a9c78263984bc4ebf205ce99e2d2bfea01", Chains: standardLiteChains()},

	// 3 LiteTokenAccounts.
	{URL: "acc://1570f386a1cd332a5a33beee62b0dd23df2a08bb74d23f1e/PegNet.acme/assets/peg", Chains: standardLiteChains()},
	{URL: "acc://78432e204c43d61286daa3800cf462f8a02d8828fdd294b3/ACME", Chains: standardLiteChains()},
	{URL: "acc://ca59e9e6c08ed245324f4c52e61defe34ab95a15abdcc802/PegNet.acme/assets/rvn", Chains: standardLiteChains()},

	// 3 live LiteDataAccounts.
	{URL: "acc://2d7b3f44935ee7de9e99766f995aa4afbc3bb9ff3dfebd9aaa8e670f178bc83c", Chains: standardLiteChains()},
	{URL: "acc://79f09991516f7b88c507c554bc13aa659d9bfff54467a0a4a4372f3468e88bd8", Chains: standardLiteChains()},
	{URL: "acc://c2db482c10bfa53099a06555d3dc5307076138a8bd003757b18f8d9c181a41c6", Chains: standardLiteChains()},

	// 4 body-less orphans (3 LDAs + 1 ADI). No Chains restoration —
	// the body is gone, so account.Hash() naturally yields the
	// empty-state hash; UpdateBPT writes that as the new leaf.
	{URL: "acc://675f6bdb8c270e4a573970c69c5ebac7a86c7b3dfa47b41e0fd522ab16654d55"},
	{URL: "acc://99a480cecc61722afebe6de6874ed42c2a847ff413ffd5f0f1e2243d27b630d8"},
	{URL: "acc://ab7a5ed9d394389090a62fa199b9b76624de9f2ef6c77fae77b2e8fa8723fdef"},
	{URL: "acc://kmutt.acme"},

	// 1 dn.acme/network — DN-side phantom in the BVN BPT. Repair
	// clears the leaf to empty-state.
	{URL: "acc://dn.acme/network"},
}

// standardAdiChains describes the chain set on a typical ADI as seen
// on the live Cyclops network: main + main-index, both at count 2 with
// the same genesis-pair entries; signature + signature-index unused.
func standardAdiChains(entry1, entry2 string) []cyclopsRepairChain {
	return []cyclopsRepairChain{
		{Name: "main", Type: merkle.ChainTypeTransaction, Entries: []string{entry1, entry2}},
		{Name: "main-index", Type: merkle.ChainTypeIndex},
		{Name: "signature", Type: merkle.ChainTypeTransaction},
		{Name: "signature-index", Type: merkle.ChainTypeIndex},
	}
}

// standardLiteChains is the chain set common to lite identities, lite
// token accounts, and lite data accounts on the live Cyclops network.
// Entries are not embedded here; chain content for these accounts is
// tracked locally and the repair only ensures the registry is intact.
func standardLiteChains() []cyclopsRepairChain {
	return []cyclopsRepairChain{
		{Name: "main", Type: merkle.ChainTypeTransaction},
		{Name: "main-index", Type: merkle.ChainTypeIndex},
		{Name: "signature", Type: merkle.ChainTypeTransaction},
		{Name: "signature-index", Type: merkle.ChainTypeIndex},
	}
}

// applyCyclopsBptRepair runs the one-shot per-partition repair.
// Called from executePostUpdateActions when the executor version
// transitions to V2CyclopsBptRepair. UpdateBPT runs immediately after,
// refreshing the BPT leaves of every account marked dirty here.
func (b *Block) applyCyclopsBptRepair() error {
	targets := cyclopsBptRepairTargets[b.Executor.Describe.PartitionId]
	if len(targets) == 0 {
		return nil
	}

	for _, t := range targets {
		if err := b.applyOneCyclopsRepair(t); err != nil {
			return errors.UnknownError.WithFormat("repair %s: %w", t.URL, err)
		}
	}
	return nil
}

func (b *Block) applyOneCyclopsRepair(t cyclopsBptRepairTarget) error {
	u, err := url.Parse(t.URL)
	if err != nil {
		return errors.UnknownError.WithFormat("parse url: %w", err)
	}
	acc := b.Batch.Account(u)

	// Restore each chain in the Chains() index and append any missing
	// entries. Both operations are idempotent: Chains().Add is a set,
	// and AddEntry only appends entries beyond the current head.
	for _, c := range t.Chains {
		err := acc.Chains().Add(&protocol.ChainMetadata{
			Name: c.Name,
			Type: c.Type,
		})
		if err != nil {
			return errors.UnknownError.WithFormat("register chain %s: %w", c.Name, err)
		}

		if len(c.Entries) == 0 {
			continue
		}
		mgr, err := acc.ChainByName(c.Name)
		if err != nil {
			return errors.UnknownError.WithFormat("open chain %s: %w", c.Name, err)
		}
		ch, err := mgr.Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load chain %s head: %w", c.Name, err)
		}
		have := ch.Height()
		for i, h := range c.Entries {
			if int64(i) < have {
				continue
			}
			raw, err := hex.DecodeString(h)
			if err != nil {
				return errors.UnknownError.WithFormat("decode entry %d: %w", i, err)
			}
			if err := ch.AddEntry(raw, false); err != nil {
				return errors.UnknownError.WithFormat("append entry %d to chain %s: %w", i, c.Name, err)
			}
		}
	}

	// Mark the account dirty so block_end's UpdateBPT refreshes the
	// stored BPT leaf. Idempotent: marking an already-dirty account
	// is a no-op.
	return acc.MarkDirty()
}
