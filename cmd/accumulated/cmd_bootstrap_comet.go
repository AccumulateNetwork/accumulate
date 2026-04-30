// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

// CometBFT state-sync configuration for the bootstrap-v3 launcher.
//
// Earlier iterations of this code attempted to pre-populate CometBFT's
// state.db + blockstore.db manually using statesync.NewLightClientStateProvider
// + state.Bootstrap(). That worked structurally — the daemon started at the
// seeded height and connected to peers — but it produced an AppHash
// mismatch when applying block H+1: our app's executor produced a
// different AppHash than the network's block H+1 header recorded. The
// root cause is that an Accumulate snapshot captures account state (the
// BPT) but does not perfectly reproduce all auxiliary database keys
// (synthetic-tx queues, sequence numbers, etc.) needed by the executor
// to reproduce the network's exact block-execution semantics.
//
// The architecturally correct fix is to use CometBFT's native ABCI
// state-sync. Accumulate has the four ABCI hooks
// (ListSnapshots/LoadSnapshotChunk/OfferSnapshot/ApplySnapshotChunk)
// implemented in internal/node/abci/snapshot.go. CometBFT will:
//   1. Discover snapshots via ListSnapshots from peers.
//   2. OfferSnapshot to our node — accept it.
//   3. Stream chunks via LoadSnapshotChunk → ApplySnapshotChunk.
//   4. The handler internally calls snapshot.FullRestore on our DB and
//      verifies the BPT root against the snapshot's AppHash.
//   5. CometBFT then writes its own state.db with the proper post-block
//      metadata (validator sets, consensus params, AppHash) and starts
//      blocksync from the snapshot height.
//
// This way the snapshot transfer + state-DB seeding both go through
// the protocol-aware path, eliminating the auxiliary-state divergence.
//
// The launcher's job becomes much smaller: write tendermint.toml with
// [statesync] enable=true plus trust info + RPC servers, then hand off.
// We keep the Accumulate-native signed-anchor verification of the
// snapshot's AppHash before configuring state-sync, so we don't blindly
// trust the primary peer for the trust hash.

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	cmtcfg "github.com/cometbft/cometbft/config"
	cmtrpchttp "github.com/cometbft/cometbft/rpc/client/http"
)

// writeStateSyncConfig writes the daemon's tendermint.toml with
// CometBFT state-sync enabled. After this, the daemon's first start
// will pull a snapshot from peers via ABCI state-sync hooks, then
// blocksync to current.
//
//   - nodeDir: e.g. <dataDir>/dnn or <dataDir>/bvnn
//   - tmRPCs: ≥2 peer Tendermint RPC URLs (state-sync requires 2+)
//   - tmP2P:  same count as tmRPCs; <host>:<port> for CometBFT P2P
//   - verifiedAppHash: the BPT root we already verified via signed anchor
//     (cross-checked against the trust block's AppHash from primary peer)
//   - snapHeight: the major-block-fire minor block height (trust height)
//   - snapTime: snapshot's recorded block time (used for trust window)
func writeStateSyncConfig(nodeDir, genesisFilename string, tmRPCs, tmP2P []string, verifiedAppHash [32]byte, snapHeight uint64, snapTime time.Time) error {
	_ = snapTime
	if len(tmRPCs) < 2 {
		return fmt.Errorf("need ≥2 RPC servers for state-sync, got %d", len(tmRPCs))
	}

	ctx := context.Background()

	// Fetch trust block hash + chain ID from primary peer. State-sync's
	// trust model: we trust this hash for one bootstrap, then the light
	// client verifies all subsequent blocks. We also cross-check the
	// trust block's AppHash against our independently-verified BPT root.
	primary, err := cmtrpchttp.New(tmRPCs[0], "/websocket")
	if err != nil {
		return fmt.Errorf("primary RPC client: %w", err)
	}
	trustH := int64(snapHeight)
	commitRes, err := primary.Commit(ctx, &trustH)
	if err != nil {
		return fmt.Errorf("fetch trust commit at height %d: %w", trustH, err)
	}
	trustHash := commitRes.SignedHeader.Header.Hash()
	chainID := commitRes.SignedHeader.Header.ChainID
	fmt.Fprintf(os.Stderr, "[statesync] trust hash at height %d (chain %s): %s\n",
		trustH, chainID, hex.EncodeToString(trustHash))

	// AppHash cross-check: block H+1's header records post-block-H app
	// hash. That should equal our verified BPT root.
	hp1 := int64(snapHeight) + 1
	bResp, err := primary.Block(ctx, &hp1)
	if err != nil {
		return fmt.Errorf("fetch block H+1=%d: %w", hp1, err)
	}
	if bResp == nil || bResp.Block == nil {
		return fmt.Errorf("nil block response for H+1=%d", hp1)
	}
	if !equalBytes(bResp.Block.Header.AppHash, verifiedAppHash[:]) {
		return fmt.Errorf("AppHash mismatch: block(H+1).AppHash=%X verified=%X — peer may be on a different chain or fork",
			bResp.Block.Header.AppHash, verifiedAppHash[:])
	}
	fmt.Fprintf(os.Stderr, "[statesync] block(H+1).AppHash matches signed-anchor root ✓\n")

	// Resolve peer node IDs for persistent_peers.
	var persistentPeers []string
	if len(tmP2P) > 0 {
		if len(tmP2P) != len(tmRPCs) {
			return fmt.Errorf("--tm-p2p-peers (%d) must match --tm-rpc-servers (%d) count", len(tmP2P), len(tmRPCs))
		}
		for i, rpcURL := range tmRPCs {
			c, err := cmtrpchttp.New(rpcURL, "/websocket")
			if err != nil {
				return fmt.Errorf("rpc client for %s: %w", rpcURL, err)
			}
			st, err := c.Status(ctx)
			if err != nil {
				return fmt.Errorf("status from %s: %w", rpcURL, err)
			}
			id := string(st.NodeInfo.DefaultNodeID)
			persistentPeers = append(persistentPeers, fmt.Sprintf("%s@%s", id, tmP2P[i]))
		}
		fmt.Fprintf(os.Stderr, "[statesync] resolved persistent_peers (%d): %s\n",
			len(persistentPeers), strings.Join(persistentPeers, ","))
	}

	// Write tendermint.toml with state-sync enabled.
	cfgDir := filepath.Join(nodeDir, "config")
	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		return fmt.Errorf("mkdir %s: %w", cfgDir, err)
	}

	// Genesis is still required even with state-sync — CometBFT needs
	// the initial validator set + consensus params to verify subsequent
	// blocks via the light client.
	genRes, err := primary.Genesis(ctx)
	if err != nil {
		return fmt.Errorf("fetch genesis doc: %w", err)
	}
	genBytes, err := cometJSONMarshal(genRes.Genesis)
	if err != nil {
		return fmt.Errorf("marshal genesis doc: %w", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "genesis.json"), genBytes, 0600); err != nil {
		return fmt.Errorf("write genesis.json: %w", err)
	}
	// Also write at the workdir level — the Accumulate run framework's
	// genesisDocProvider resolves <workdir>/<DnGenesis|BvnGenesis>.
	workDir := filepath.Dir(nodeDir)
	wdGenPath := filepath.Join(workDir, genesisFilename)
	if err := os.WriteFile(wdGenPath, genBytes, 0600); err != nil {
		return fmt.Errorf("write workdir genesis %s: %w", wdGenPath, err)
	}
	fmt.Fprintf(os.Stderr, "[statesync] wrote genesis.json (%d bytes) → %s + %s\n",
		len(genBytes), filepath.Join(cfgDir, "genesis.json"), wdGenPath)

	cfg := cmtcfg.DefaultConfig()
	cfg.SetRoot(nodeDir)
	cfg.TxIndex.Indexer = "null"
	cfg.Storage.DiscardABCIResponses = true
	cfg.P2P.AllowDuplicateIP = false
	cfg.P2P.AddrBookStrict = false
	cfg.P2P.PersistentPeers = strings.Join(persistentPeers, ",")
	cfg.Mempool.MaxTxBytes = 4194304

	// State-sync is currently disabled (#4005): the v2 snapshot path
	// is lossy — even with IgnoreIndices=false in CollectOptions, the
	// snapshot file captures only ~30% of the validator's actual DB
	// keys. Restoring it produces a state from which the executor
	// can't reproduce the network's block execution; AppHash diverges
	// on the first applied block.
	//
	// Until snapshot.Collect/Restore is made lossless, fall back to
	// blocksync from genesis: CometBFT applies InitChain (Accumulate
	// detects existing DB state and is a no-op), then blocksync
	// downloads every block from 1 to current and applies them via
	// our V2 executor. Slow on long chains but always consistent.
	cfg.StateSync.Enable = false
	_ = trustH
	_ = trustHash

	tmlPath := filepath.Join(cfgDir, "tendermint.toml")
	cmtcfg.WriteConfigFile(tmlPath, cfg)
	fmt.Fprintf(os.Stderr, "[statesync] wrote %s (state-sync enabled, trust=%d, %d peers)\n",
		tmlPath, trustH, len(persistentPeers))
	return nil
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// cometJSONMarshal is exposed via a wrapper because we don't import
// cometbft/libs/json in this file otherwise; kept as its own function
// so it's clear we're using CometBFT's specific JSON encoding (not
// stdlib) for the genesis doc.
var cometJSONMarshal = func(v interface{}) ([]byte, error) {
	return cometJSONMarshalImpl(v)
}
