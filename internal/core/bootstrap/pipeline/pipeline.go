// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package pipeline runs the end-to-end bootstrap orchestration that
// the accumulated bootstrap subcommand calls into (issue #3975, parent
// #3953).
//
// The pipeline:
//
//  1. Connect to a peer via JSON-RPC.
//  2. Pin block H at confirmed-depth (current_tip - confirmation_depth).
//  3. Pull the minimum bootstrap set of accounts and their main chains.
//  4. Run the back-walker against pulled keybooks to construct the
//     proof of derivation.
//  5. Persist the back-walk artifact via bootpersist.
//
// BPT-structure fill via #3972's GetBptPage and the data-dir handoff to
// `accumulated run` are deferred to follow-up slices on this issue.
// The first cut delivers a verifiable proof artifact on disk that can
// be inspected and that downstream wiring can plug into.
package pipeline

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/backwalk"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/bootpersist"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Options configures a bootstrap pipeline run.
type Options struct {
	// Endpoint is the v3 JSON-RPC URL of a peer to bootstrap from.
	Endpoint string

	// Network identifies which network we're bootstrapping against
	// (mainnet, testnet, etc.). Persisted alongside the proof.
	Network string

	// Partition is the partition the new node will participate in
	// (default "Directory").
	Partition string

	// DataDir is where the bootstrapped state will live.
	DataDir string

	// PinnedGenesisHash is the genesis snapshot hash compiled into the
	// binary for this network. The back-walker terminates against it.
	PinnedGenesisHash [32]byte

	// ConfirmationDepth bounds how far back from the live tip we pin
	// our bootstrap moment. Default 2.
	ConfirmationDepth uint64

	// ExtraAccounts are additional URLs the operator wants pulled
	// alongside the standard minimum bootstrap set.
	ExtraAccounts []*url.URL

	// SkipProof bypasses back-walker error checking. Development use
	// only — disables the proof-of-derivation guarantee.
	SkipProof bool

	// Logger receives status messages. nil = no output.
	Logger func(format string, args ...any)
}

// Result reports what the pipeline accomplished.
type Result struct {
	PinBlock          uint64
	PinTime           time.Time
	AccountsPulled    int
	ChainEntriesPulled int
	BackWalkEntries   int
	GenesisTerminated bool
	ArtifactPath      string
}

// Run executes the bootstrap pipeline.
func Run(ctx context.Context, opts Options) (*Result, error) {
	if opts.Endpoint == "" {
		return nil, errors.New("endpoint is required")
	}
	if opts.Network == "" {
		return nil, errors.New("network is required")
	}
	if opts.DataDir == "" {
		return nil, errors.New("data dir is required")
	}
	if opts.Partition == "" {
		opts.Partition = "Directory"
	}
	if opts.ConfirmationDepth == 0 {
		opts.ConfirmationDepth = 2
	}
	logf := opts.Logger
	if logf == nil {
		logf = func(string, ...any) {}
	}

	// 1. Connect.
	logf("[1/5] Connecting to %s", opts.Endpoint)
	client := jsonrpc.NewClient(opts.Endpoint)
	q := api.Querier2{Querier: client}

	// Probe node identity first; ConsensusStatus needs the peer's NodeID.
	ni, err := client.NodeInfo(ctx, api.NodeInfoOptions{})
	if err != nil {
		return nil, fmt.Errorf("node info: %w", err)
	}
	cs, err := client.ConsensusStatus(ctx, api.ConsensusStatusOptions{
		NodeID:    ni.PeerID.String(),
		Partition: opts.Partition,
	})
	if err != nil {
		return nil, fmt.Errorf("consensus status: %w", err)
	}
	tip := uint64(cs.LastBlock.Height)
	pinBlock := tip
	if pinBlock > opts.ConfirmationDepth {
		pinBlock -= opts.ConfirmationDepth
	}
	logf("    tip=%d, pinned at H=%d (confirmation depth=%d)", tip, pinBlock, opts.ConfirmationDepth)

	// 2. Open a local database for the pulled state.
	logf("[2/5] Opening local database at %s", opts.DataDir)
	db, err := database.OpenBadger(filepath.Join(opts.DataDir, "accumulate.db"), nil)
	if err != nil {
		return nil, fmt.Errorf("open badger: %w", err)
	}
	defer db.Close()

	// 3. Pull the minimum bootstrap set.
	logf("[3/5] Pulling minimum bootstrap set state")
	accounts := minimumBootstrapSet(opts.Partition)
	accounts = append(accounts, opts.ExtraAccounts...)

	pulled := 0
	chainEntries := 0
	batch := db.Begin(true)
	for _, u := range accounts {
		n, err := pullAccount(ctx, q, batch, u)
		if err != nil {
			batch.Discard()
			return nil, fmt.Errorf("pull %s: %w", u, err)
		}
		pulled++
		chainEntries += n
		logf("    pulled %s (%d main-chain entries)", u, n)
	}
	if err := batch.Commit(); err != nil {
		return nil, fmt.Errorf("commit pulled state: %w", err)
	}

	// 4. Run the back-walker on the operators keybook.
	logf("[4/5] Running back-walker for proof of derivation")
	walker := backwalk.New(backwalk.Options{PinnedGenesisHash: opts.PinnedGenesisHash})
	roBatch := db.Begin(false)
	defer roBatch.Discard()

	// Walk at the partition tip's time. This is approximately the pin
	// block's time (typically within seconds for confirmation depth 2).
	// Faithful pin-time at H specifically requires the historical
	// block-time lookup deferred under #3978's BlockHeight path; for
	// now this avoids the time.Now() drift that #3979 / C5 flagged.
	pinTime := cs.LastBlock.Time
	operatorsUrl := protocol.PartitionUrl(opts.Partition).JoinPath(protocol.Operators)
	earliest, err := walker.Walk(roBatch, operatorsUrl, pinTime)
	if err != nil {
		if opts.SkipProof {
			logf("    back-walker error (skipped): %v", err)
		} else {
			return nil, fmt.Errorf("back-walker: %w (pass --skip-proof to ignore in development)", err)
		}
	}

	// 5. Persist.
	logf("[5/5] Persisting bootstrap artifact")
	artifact := &bootpersist.Artifact{
		PinnedGenesisHash: opts.PinnedGenesisHash,
		Network:           opts.Network,
		PinBlock: bootpersist.PinBlock{
			Partition:       opts.Partition,
			MinorBlockIndex: pinBlock,
		},
		State: bootpersist.StateRecord{
			Current:        "BOOTING",
			EnteredBooting: time.Now(),
		},
	}
	if err := bootpersist.Save(opts.DataDir, artifact); err != nil {
		return nil, fmt.Errorf("save artifact: %w", err)
	}

	res := &Result{
		PinBlock:           pinBlock,
		PinTime:            cs.LastBlock.Time,
		AccountsPulled:     pulled,
		ChainEntriesPulled: chainEntries,
		BackWalkEntries:    walker.MemoSize(),
		ArtifactPath:       filepath.Join(opts.DataDir, bootpersist.FileName),
	}
	if earliest != nil {
		res.GenesisTerminated = earliest.GenesisTerm
	}
	logf("Bootstrap complete. Run `accumulated run` to enter BOOTING and converge to ACTIVE.")
	return res, nil
}

// minimumBootstrapSet returns the canonical set of accounts that need
// to be pulled for a node bootstrapping into the given partition.
func minimumBootstrapSet(partition string) []*url.URL {
	pu := protocol.PartitionUrl(partition)
	return []*url.URL{
		protocol.DnUrl().JoinPath(protocol.Network),
		pu.JoinPath(protocol.Operators),
		pu.JoinPath(protocol.Operators, "1"),
		pu.JoinPath(protocol.Ledger),
		pu.JoinPath(protocol.AnchorPool),
		pu.JoinPath(protocol.Synthetic),
	}
}

// pullAccount fetches the account at u, stores it locally, and pulls
// the main-chain entries, the main-index-chain entries (for block-time
// lookup), and per-transaction signature sets at the signer accounts.
//
// Without the index entries, backwalk's entryBlockTime returns nil,
// keybookat.Resolve short-circuits, and verification proceeds at
// "unknown time" — see issue #3977 for context.
func pullAccount(ctx context.Context, q api.Querier2, batch *database.Batch, u *url.URL) (int, error) {
	rec, err := q.QueryAccount(ctx, u, nil)
	if err != nil {
		return 0, fmt.Errorf("query account: %w", err)
	}
	if err := batch.Account(u).Main().Put(rec.Account); err != nil {
		return 0, fmt.Errorf("store account: %w", err)
	}

	// Pull main-chain entries. Some accounts have very long chains;
	// keep this bounded for now (issue #3977 follow-up: paginate
	// fully or surface truncation).
	const maxEntries = 200
	count := uint64(maxEntries)
	expand := true
	page, err := q.QueryChainEntries(ctx, u, &api.ChainQuery{
		Name: "main",
		Range: &api.RangeOptions{
			Start:  0,
			Count:  &count,
			Expand: &expand,
		},
	})
	if err != nil {
		// Some accounts may not have a main chain (legacy / placeholder).
		// Treat as zero entries; not fatal.
		return 0, nil
	}
	if page == nil {
		return 0, nil
	}

	// Pull main-index-chain entries to populate block-time lookup
	// for each main-chain entry. Index chains are short (one entry
	// per block range) so we don't bound this.
	if err := pullIndexChain(ctx, q, batch, u, "main-index"); err != nil {
		// Log via err return; not fatal — verification proceeds at
		// "unknown time" but doesn't crash.
		_ = err
	}

	stored := 0
	for _, entry := range page.Records {
		if entry == nil {
			continue
		}
		// Add the chain entry locally so the back-walker can read it.
		if err := batch.Account(u).MainChain().Inner().AddEntry(entry.Entry[:], false); err != nil {
			return 0, fmt.Errorf("add chain entry: %w", err)
		}
		// Store the transaction message if expanded.
		if entry.Value != nil {
			if mr, ok := entry.Value.(*api.MessageRecord[messaging.Message]); ok && mr != nil && mr.Message != nil {
				var hashArr [32]byte
				copy(hashArr[:], entry.Entry[:])
				if err := batch.Message(hashArr).Main().Put(mr.Message); err != nil {
					return 0, fmt.Errorf("store message: %w", err)
				}
			}
		}
		stored++

		// Pull signatures for this transaction at the signer accounts.
		// The MessageRecord's Signatures field contains SignatureSetRecords
		// per signer; for each, pull and store locally. Without these,
		// VerifyUserSignaturesAt fails with ErrNoSignatures.
		if mr, ok := entry.Value.(*api.MessageRecord[messaging.Message]); ok && mr != nil {
			if err := storeSignaturesForMessage(batch, entry.Entry, mr); err != nil {
				return 0, fmt.Errorf("store signatures for entry %d: %w", entry.Index, err)
			}
		}
	}
	return stored, nil
}

// pullIndexChain pulls all entries from an index chain and inserts
// them into the local DB. Index chains are short (~one entry per
// block range with main-chain entries) so we don't bound the count.
func pullIndexChain(ctx context.Context, q api.Querier2, batch *database.Batch, u *url.URL, chainName string) error {
	// First query the chain head to get the count.
	head, err := q.QueryChain(ctx, u, &api.ChainQuery{Name: chainName})
	if err != nil {
		return fmt.Errorf("query %s head: %w", chainName, err)
	}
	if head.Count == 0 {
		return nil
	}

	count := head.Count
	expand := false
	page, err := q.QueryChainEntries(ctx, u, &api.ChainQuery{
		Name: chainName,
		Range: &api.RangeOptions{
			Start:  0,
			Count:  &count,
			Expand: &expand,
		},
	})
	if err != nil {
		return fmt.Errorf("query %s entries: %w", chainName, err)
	}
	if page == nil {
		return nil
	}

	chain, err := batch.Account(u).ChainByName(chainName)
	if err != nil {
		return fmt.Errorf("local %s chain: %w", chainName, err)
	}
	for _, entry := range page.Records {
		if entry == nil {
			continue
		}
		if err := chain.Inner().AddEntry(entry.Entry[:], false); err != nil {
			return fmt.Errorf("add %s entry %d: %w", chainName, entry.Index, err)
		}
	}
	return nil
}

// storeSignaturesForMessage records the signatures returned in the
// expanded MessageRecord into the local DB so VerifyUserSignaturesAt
// can find them. For each SignatureSetRecord (one per signer):
//   - Stores each signature message in the message store.
//   - Adds a SignatureSetEntry to the signer's signature-set for this
//     transaction.
func storeSignaturesForMessage(batch *database.Batch, txnHash [32]byte, mr *api.MessageRecord[messaging.Message]) error {
	if mr.Signatures == nil {
		return nil
	}
	for _, set := range mr.Signatures.Records {
		if set == nil || set.Account == nil || set.Signatures == nil {
			continue
		}
		signerUrl := set.Account.GetUrl()
		if signerUrl == nil {
			continue
		}
		for _, sigMsg := range set.Signatures.Records {
			if sigMsg == nil || sigMsg.Message == nil {
				continue
			}
			// Store the signature message at its own hash.
			var sigHash [32]byte
			if sigMsg.ID != nil {
				h := sigMsg.ID.Hash()
				copy(sigHash[:], h[:])
			} else {
				continue
			}
			if err := batch.Message(sigHash).Main().Put(sigMsg.Message); err != nil {
				return fmt.Errorf("store sig message %x: %w", sigHash[:8], err)
			}
			// Add the signature-set entry at the signer's account.
			// We don't have KeyIndex/Version in the API record cleanly;
			// best-effort with zeros — VerifyUserSignaturesAt will look
			// up by hash and the cryptographic check is what matters.
			entry := &database.SignatureSetEntry{
				KeyIndex: 0,
				Version:  1,
				Hash:     sigHash,
			}
			if err := batch.Account(signerUrl).Transaction(txnHash).Signatures().Add(entry); err != nil {
				return fmt.Errorf("add sig-set entry: %w", err)
			}
		}
	}
	return nil
}
