// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

// CometBFT state-DB seeding for the bootstrap-v3 launcher.
//
// After --via-snapshot loads BPT data at major-block boundary B (with
// minor block height H), CometBFT's own consensus state DB is empty.
// On first start the handshake compares a fresh consensus state
// (height 0) against the app's non-zero AppHash and refuses with
// "Did you reset CometBFT without resetting your application's data?".
//
// We bypass this by reusing the same primitives CometBFT's own
// state-sync uses internally:
//
//   - statesync.NewLightClientStateProvider runs a light client over
//     ≥2 peer Tendermint RPC URLs and exposes a State(height) call
//     that returns a fully-populated state.State (validator sets,
//     consensus params, AppHash, LastBlockID, etc.).
//   - state.NewStore(...).Bootstrap(state) writes that State to the
//     CometBFT state DB exactly the way state-sync does at the end
//     of a successful sync.
//   - blockStore.SaveSeenCommit persists the latest commit so
//     blocksync knows where to pick up.
//
// We then cross-check that the State's AppHash equals the BPT root we
// already verified via the Accumulate signed-anchor pool. If it does,
// the launcher's trust chain (signed anchor → BPT root) and the
// CometBFT trust chain (light client → block hash → AppHash) agree on
// the same commitment, and the daemon can come up at height H without
// running InitChain or replaying any historical blocks.

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	cmtcfg "github.com/cometbft/cometbft/config"
	cmtdb "github.com/cometbft/cometbft-db"
	cmtjson "github.com/cometbft/cometbft/libs/json"
	cmtlog "github.com/cometbft/cometbft/libs/log"
	cmtlight "github.com/cometbft/cometbft/light"
	cmtstateproto "github.com/cometbft/cometbft/proto/tendermint/state"
	cmtstoreproto "github.com/cometbft/cometbft/proto/tendermint/store"
	cmtversionproto "github.com/cometbft/cometbft/proto/tendermint/version"
	cmtrpchttp "github.com/cometbft/cometbft/rpc/client/http"
	cmtstate "github.com/cometbft/cometbft/state"
	cmtstatesync "github.com/cometbft/cometbft/statesync"
	cmtstore "github.com/cometbft/cometbft/store"
	cmtversion "github.com/cometbft/cometbft/version"
)

// genesisDocKey is the state DB key that node.LoadStateFromDBOrGenesisDocProvider
// reads to skip the GenesisDocProvider. Mirrors the unexported
// constant in cometbft/node/setup.go.
var genesisDocKey = []byte("genesisDoc")

// writeCometState seeds the CometBFT state DB at <nodeDir>/data/{state,blockstore}.db.
//
//   - nodeDir: e.g. <dataDir>/dnn or <dataDir>/bvnn
//   - tmRPCs:  ≥2 peer Tendermint RPC URLs (light client requirement)
//   - ourAppHash: the BPT root verified against the signed anchor
//   - snapHeight: minor block height the snapshot was taken at
//   - snapTime: the snapshot's recorded block time (unused; light
//     client returns the authoritative value)
func writeCometState(nodeDir string, tmRPCs []string, ourAppHash [32]byte, snapHeight uint64, snapTime time.Time) error {
	_ = snapTime
	if len(tmRPCs) < 2 {
		return fmt.Errorf("need ≥2 RPC servers, got %d", len(tmRPCs))
	}

	dataDir := filepath.Join(nodeDir, "data")
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return fmt.Errorf("mkdir %s: %w", dataDir, err)
	}

	ctx := context.Background()

	// Fetch trust hash + chain ID from primary peer. Trusting the
	// peer for the block hash alone is acceptable: the AppHash check
	// below catches a lying peer.
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

	fmt.Fprintf(os.Stderr, "[comet] trust hash at height %d (chain %s): %s\n",
		trustH, chainID, hex.EncodeToString(trustHash))

	// Build light-client state provider.
	logger := cmtlog.NewNopLogger()
	version := cmtstateproto.Version{
		Consensus: cmtversionproto.Consensus{Block: 11, App: 2},
		Software:  cmtversion.TMCoreSemVer,
	}
	provider, err := cmtstatesync.NewLightClientStateProvider(
		ctx,
		chainID,
		version,
		1, // initial height (chain genesis)
		tmRPCs,
		cmtlight.TrustOptions{
			Period: 168 * time.Hour,
			Height: trustH,
			Hash:   trustHash,
		},
		logger,
	)
	if err != nil {
		return fmt.Errorf("NewLightClientStateProvider: %w", err)
	}

	// AppHash() must be called first — it triggers fetching of blocks
	// at H+1 and H+2 which State() then requires.
	apphash, err := provider.AppHash(ctx, snapHeight)
	if err != nil {
		return fmt.Errorf("provider.AppHash(%d): %w", snapHeight, err)
	}
	if len(apphash) != 32 {
		return fmt.Errorf("unexpected AppHash length %d", len(apphash))
	}
	var got [32]byte
	copy(got[:], apphash)
	if got != ourAppHash {
		return fmt.Errorf("AppHash mismatch: light-client says %x, signed-anchor says %x",
			apphash, ourAppHash[:])
	}
	fmt.Fprintf(os.Stderr, "[comet] light-client AppHash matches signed-anchor root ✓\n")

	state, err := provider.State(ctx, snapHeight)
	if err != nil {
		return fmt.Errorf("provider.State(%d): %w", snapHeight, err)
	}

	commit, err := provider.Commit(ctx, snapHeight)
	if err != nil {
		return fmt.Errorf("provider.Commit(%d): %w", snapHeight, err)
	}

	// Fetch the actual chain's genesis doc. We write it both to the
	// state DB (where node.LoadStateFromDBOrGenesisDocProvider reads
	// it on subsequent starts) AND to <configDir>/genesis.json (which
	// the Accumulate ABCI Info handler reads on first call via
	// genesis.DocProvider).
	genRes, err := primary.Genesis(ctx)
	if err != nil {
		return fmt.Errorf("fetch genesis doc: %w", err)
	}
	genBytes, err := cmtjson.Marshal(genRes.Genesis)
	if err != nil {
		return fmt.Errorf("marshal genesis doc: %w", err)
	}
	cfgDir := filepath.Join(nodeDir, "config")
	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		return fmt.Errorf("mkdir %s: %w", cfgDir, err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "genesis.json"), genBytes, 0600); err != nil {
		return fmt.Errorf("write genesis.json: %w", err)
	}
	fmt.Fprintf(os.Stderr, "[comet] wrote genesis.json (%d bytes)\n", len(genBytes))

	stateDB, err := cmtdb.NewGoLevelDB("state", dataDir)
	if err != nil {
		return fmt.Errorf("open state.db: %w", err)
	}
	defer stateDB.Close()

	if err := stateDB.SetSync(genesisDocKey, genBytes); err != nil {
		return fmt.Errorf("save genesis doc to state.db: %w", err)
	}

	stateStore := cmtstate.NewStore(stateDB, cmtstate.StoreOptions{DiscardABCIResponses: true})
	if err := stateStore.Bootstrap(state); err != nil {
		return fmt.Errorf("stateStore.Bootstrap: %w", err)
	}

	blockDB, err := cmtdb.NewGoLevelDB("blockstore", dataDir)
	if err != nil {
		return fmt.Errorf("open blockstore.db: %w", err)
	}
	defer blockDB.Close()

	bs := cmtstore.NewBlockStore(blockDB)
	if err := bs.SaveSeenCommit(state.LastBlockHeight, commit); err != nil {
		return fmt.Errorf("SaveSeenCommit: %w", err)
	}
	// Manually update the BlockStoreState so the blockstore reports
	// height == state.LastBlockHeight at next start. Without this the
	// node panics with "state (H) and store (0) height mismatch".
	cmtstore.SaveBlockStoreState(&cmtstoreproto.BlockStoreState{
		Base:   state.LastBlockHeight,
		Height: state.LastBlockHeight,
	}, blockDB)

	fmt.Fprintf(os.Stderr, "[comet] seeded state.db + blockstore.db at height %d\n", state.LastBlockHeight)

	// Write tendermint.toml with our preferred small-footprint
	// settings: null tx indexer (we don't run a public RPC), discard
	// ABCI responses (state.db stays small), default mempool/p2p.
	if err := writeTendermintToml(nodeDir); err != nil {
		return fmt.Errorf("write tendermint.toml: %w", err)
	}
	return nil
}

// writeTendermintToml writes <nodeDir>/config/tendermint.toml with
// our preferred low-disk-footprint settings before the daemon's first
// run, so the daemon's existing-file path (run/consensus.go:138-156)
// loads our config instead of generating a default.
//
// The daemon will fill in NodeKey/PrivValidatorKey paths, P2P listen
// addresses, etc. on its own when it sees the file but with empty
// values for those — the existing-file branch trusts whatever's
// loaded via Viper, so we set just the knobs we care about and let
// CometBFT's own defaults handle the rest.
func writeTendermintToml(nodeDir string) error {
	configDir := filepath.Join(nodeDir, "config")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("mkdir %s: %w", configDir, err)
	}
	cfg := cmtcfg.DefaultConfig()
	cfg.SetRoot(nodeDir)
	cfg.TxIndex.Indexer = "null"
	cfg.Storage.DiscardABCIResponses = true
	cfg.P2P.AllowDuplicateIP = false
	cfg.P2P.AddrBookStrict = false // dev/test networks have private addrs
	cfg.Mempool.MaxTxBytes = 4194304
	tmlPath := filepath.Join(configDir, "tendermint.toml")
	cmtcfg.WriteConfigFile(tmlPath, cfg)
	fmt.Fprintf(os.Stderr, "[comet] wrote %s (null indexer, discard ABCI responses)\n", tmlPath)
	return nil
}
