// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/anchorsrc"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/bootpersist"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/clientsrc"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/orchestrator"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	snapshotV2 "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/websocket"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// cmdBootstrap is `accumulated bootstrap`.
//
// Drives the bootstrap-v3 orchestrator from BOOTING to ACTIVE
// against an ACTIVE/COMPLETE peer over the v3 WebSocket transport.
// Exits zero on ACTIVE; non-zero on failure.
//
// AnchorSource is configurable:
//
//   --peer-anchor-pool: production AnchorSource (#3988). Walks the
//     peer's anchor pool main chain, verifies validator-quorum
//     signatures locally. The URL is the receiving partition's
//     anchor pool — to verify a BVN's BPT root, pass dn.acme/anchors.
//
//   --anchor-hex (dev only): stub source returning a fixed anchor.
//     Useful for ramping the orchestrator on synthetic networks
//     where you already know the expected BPT root.
//
// Persistence (#3990): the launcher reads bootstrap-state.json from
// the data dir on start and writes it on every phase / state
// boundary. Crash-resume picks up where the previous run left off.
var cmdBootstrap = &cobra.Command{
	Use:   "bootstrap",
	Short: "Run bootstrap-v3: sync a node from BOOTING to ACTIVE",
	Run:   runBootstrap,
}

var flagBootstrap = struct {
	Network        string
	Partition      string
	PeerWS         string
	PeerAnchorPool string
	DataDir        string
	PageSize       uint64
	AnchorHex      string
	AnchorBlk      uint64
	DiffOnly       bool
	DiffSteadySecs uint64
	ViaSnapshot    string // HTTP base URL of peer's snapshot endpoint, e.g. http://localhost:26680
	WriteConfig    bool
	TmListen       string
	TmBootstrap    string // comma-separated Tendermint multiaddrs for catch-up
	TmRpcServers   string // comma-separated Tendermint RPC URLs for state seeding (≥2)
	TmP2PPeers     string // comma-separated Tendermint P2P host:port pairs (paired with --tm-rpc-servers; node IDs derived via /status)
}{}

func init() {
	cmdMain.AddCommand(cmdBootstrap)
	f := cmdBootstrap.Flags()
	f.StringVar(&flagBootstrap.Network, "network", "", "Network identifier (mainnet/testnet/devnet/...)")
	f.StringVar(&flagBootstrap.Partition, "partition", "Directory", "Partition to bootstrap (Directory or BVN name)")
	f.StringVar(&flagBootstrap.PeerWS, "peer", "", "WebSocket URL of an ACTIVE/COMPLETE peer")
	f.StringVar(&flagBootstrap.PeerAnchorPool, "peer-anchor-pool", "", "URL of the peer's anchor pool that holds anchors from --partition (e.g. acc://dn.acme/anchors when bootstrapping a BVN)")
	f.StringVar(&flagBootstrap.DataDir, "data-dir", "", "Bootstrap data directory (defaults to --work-dir)")
	f.Uint64Var(&flagBootstrap.PageSize, "page-size", 256, "Paginated query page size")
	f.StringVar(&flagBootstrap.AnchorHex, "anchor-hex", "", "DEV: hex-encoded BPT root that promotes ACTIVE on match")
	f.Uint64Var(&flagBootstrap.AnchorBlk, "anchor-block", 0, "DEV: block height associated with --anchor-hex")
	f.BoolVar(&flagBootstrap.DiffOnly, "diff", false, "DEV: after spine+enumerate, run a leaf-level BPT diff against the source and exit (no steady-state)")
	f.Uint64Var(&flagBootstrap.DiffSteadySecs, "diff-after-steady", 0, "DEV: run --diff after this many seconds of steady-state (0 = immediately after enumerate)")
	f.StringVar(&flagBootstrap.ViaSnapshot, "via-snapshot", "", "Bootstrap by fetching a verified major-block snapshot from this HTTP base URL (e.g. http://localhost:26680). Bypasses spine pull / enumerate / steady-state.")
	f.BoolVar(&flagBootstrap.WriteConfig, "write-config", true, "After bootstrap, write accumulate.toml + move BPT db into the launchable layout that 'accumulated run' expects")
	f.StringVar(&flagBootstrap.TmListen, "tm-listen", "/ip4/0.0.0.0/tcp/26656", "Listen address for the launched daemon (Tendermint base port; Accumulate ports derived from it)")
	f.StringVar(&flagBootstrap.TmBootstrap, "tm-bootstrap-peers", "", "Comma-separated Tendermint P2P multiaddrs (e.g. /dns/host/tcp/26656/p2p/<id>) used as persistent peers for catch-up")
	f.StringVar(&flagBootstrap.TmRpcServers, "tm-rpc-servers", "", "Comma-separated Tendermint RPC URLs (≥2 required for light-client state seeding, e.g. http://host:26657,http://host2:26657)")
	f.StringVar(&flagBootstrap.TmP2PPeers, "tm-p2p-peers", "", "Comma-separated Tendermint P2P host:port pairs paired with --tm-rpc-servers; node IDs are fetched via /status (e.g. host:26656,host2:26656)")
}

func runBootstrap(cmd *cobra.Command, _ []string) {
	if flagBootstrap.Partition == "" {
		fatalf("--partition required")
	}
	if flagBootstrap.Network == "" {
		fatalf("--network required")
	}
	if flagBootstrap.ViaSnapshot == "" {
		// Legacy enumerate+steady path requires --peer + AnchorSource.
		if flagBootstrap.PeerWS == "" {
			fatalf("--peer required (or use --via-snapshot)")
		}
		if flagBootstrap.PeerAnchorPool == "" && flagBootstrap.AnchorHex == "" {
			fatalf("either --peer-anchor-pool or --anchor-hex required (or use --via-snapshot)")
		}
	}

	dataDir := flagBootstrap.DataDir
	if dataDir == "" {
		dataDir = flagMain.WorkDir
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		fatalf("create data dir: %v", err)
	}

	if flagBootstrap.ViaSnapshot != "" {
		runBootstrapViaSnapshot(dataDir)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigs
		fmt.Fprintln(os.Stderr, "received interrupt; shutting down…")
		cancel()
	}()
	defer cancel()

	ws, err := websocket.NewClient(flagBootstrap.PeerWS, flagBootstrap.Network)
	checkf(err, "dial peer %s", flagBootstrap.PeerWS)
	defer ws.Close()

	dbPath := filepath.Join(dataDir, "bootstrap-"+flagBootstrap.Partition+".db")
	db, err := database.OpenBadger(dbPath, nil)
	checkf(err, "open local db at %s", dbPath)
	// Install the production observer so the BPT update path
	// (UpdateBPT during pull / gossip commits) can compute per-account
	// hashes. Without this, batch.UpdateBPT errors with "observer is
	// not set".
	db.SetObserver(execute.NewDatabaseObserver())

	// Pick AnchorSource: production over flag, dev fallback.
	var anchors orchestrator.AnchorSource
	if flagBootstrap.PeerAnchorPool != "" {
		poolURL, perr := url.Parse(flagBootstrap.PeerAnchorPool)
		checkf(perr, "parse --peer-anchor-pool")
		as, aerr := anchorsrc.New(ws, poolURL, db)
		checkf(aerr, "build production AnchorSource")
		anchors = as
	} else {
		anchors = newDevAnchorSource(flagBootstrap.AnchorBlk, flagBootstrap.AnchorHex)
	}

	src := clientsrc.New(ws, anchors)

	// Resume from persisted state if present.
	machine := nodestate.New()
	if art, err := bootpersist.Load(dataDir); err == nil {
		machine = restoreMachine(art)
		fmt.Fprintf(os.Stderr, "[resume] state=%s spine=%v enumerate=%v observed=%d\n",
			art.State.Current, art.Phases.SpinePullDone, art.Phases.EnumerateDone, len(art.ObservedAnchors))
	} else if !errors.Is(err, os.ErrNotExist) {
		fatalf("load persisted state: %v", err)
	}

	// Wire persistence on every state transition.
	machine.OnChange(func(ad nodestate.Advertisement) {
		if err := saveState(dataDir, ad); err != nil {
			fmt.Fprintf(os.Stderr, "[persist] warn: %v\n", err)
		}
	})

	evCh, err := ws.Subscribe(ctx, api.SubscribeOptions{
		Partition: flagBootstrap.Partition,
	})
	checkf(err, "subscribe to %s events", flagBootstrap.Partition)

	scope := protocol.PartitionUrl(flagBootstrap.Partition)
	isDN := flagBootstrap.Partition == protocol.Directory

	opts := orchestrator.Options{
		Partition:    flagBootstrap.Partition,
		PartitionURL: scope,
		IsDirectory:  isDN,
		PageSize:     flagBootstrap.PageSize,
		OnPhase: func(phase, msg string) {
			fmt.Fprintf(os.Stderr, "[%s] %s\n", phase, msg)
			if err := savePhase(dataDir, phase); err != nil {
				fmt.Fprintf(os.Stderr, "[persist] warn: %v\n", err)
			}
			// In --diff mode, cancel context after the configured
			// steady-state delay (0 = immediately after enumerate).
			if flagBootstrap.DiffOnly && phase == "enumerate" && msg != "scanning partition BPT" {
				if flagBootstrap.DiffSteadySecs == 0 {
					cancel()
				} else {
					go func() {
						time.Sleep(time.Duration(flagBootstrap.DiffSteadySecs) * time.Second)
						cancel()
					}()
				}
			}
		},
	}

	runErr := orchestrator.Run(ctx, src, evCh, db, machine, opts)
	if flagBootstrap.DiffOnly {
		fmt.Fprintf(os.Stderr, "--- bpt diff: comparing local vs source ---\n")
		// Use a fresh background context for the diff (the orchestrator
		// run was canceled to bail out after enumerate).
		diffCtx, diffCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer diffCancel()
		_, derr := orchestrator.RunBPTDiff(diffCtx, src, db, scope, flagBootstrap.PageSize, os.Stderr)
		if derr != nil {
			fatalf("bpt diff: %v", derr)
		}
		return
	}
	if runErr != nil {
		fatalf("bootstrap: %v", runErr)
	}

	ad := machine.Get()
	if ad.State == nodestate.StateActive {
		fmt.Fprintf(os.Stderr, "ACTIVE at block %d (anchor=%x…)\n", ad.SinceBlock, ad.VerifiedAnchor[:8])
		return
	}
	fmt.Fprintf(os.Stderr, "exited in state %s\n", ad.State)
	os.Exit(2)
}

// runBootstrapViaSnapshot is the snapshot-fetch path. The peer
// exposes /v3/snapshot/:partition/list and /v3/snapshot/:partition/:N
// (added by daemon when periodic snapshots are enabled). The
// launcher fetches the latest, restores it, and exits.
//
// After this completes, local BPT root == signed anchor for the
// loaded major block. accumulated run picks it up from there and
// catches up to current via normal consensus.
func runBootstrapViaSnapshot(dataDir string) {
	base := strings.TrimRight(flagBootstrap.ViaSnapshot, "/")
	partition := strings.ToLower(flagBootstrap.Partition)
	if partition == strings.ToLower(protocol.Directory) {
		// daemon's PartitionId for the DN is "directory" lowercase.
		partition = "directory"
	}
	listURL := base + "/v3/snapshot/" + partition

	fmt.Fprintf(os.Stderr, "[snapshot] listing available major-block snapshots at %s\n", listURL)
	resp, err := http.Get(listURL)
	checkf(err, "list snapshots")
	if resp.StatusCode != 200 {
		fatalf("list snapshots: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	checkf(err, "read list")
	lines := strings.Fields(string(body))
	if len(lines) == 0 {
		fatalf("no snapshots available — peer must have enable-snapshots = true and have crossed at least one major block")
	}
	// Use the second-most-recent snapshot — the latest may not yet have
	// a signed anchor propagated through the cross-partition cycle.
	idx := len(lines) - 1
	if len(lines) >= 2 {
		idx = len(lines) - 2
	}
	latest := lines[idx]
	fmt.Fprintf(os.Stderr, "[snapshot] selected snapshot %s (idx %d of %d available)\n", latest, idx, len(lines))

	// Fetch the snapshot file.
	fetchURL := base + "/v3/snapshot/" + partition + "/" + latest
	resp, err = http.Get(fetchURL)
	checkf(err, "fetch snapshot")
	if resp.StatusCode != 200 {
		fatalf("fetch snapshot: HTTP %d", resp.StatusCode)
	}
	tmpFile := filepath.Join(dataDir, "fetched-major-"+latest+".bpt")
	out, err := os.Create(tmpFile)
	checkf(err, "create temp file")
	n, err := io.Copy(out, resp.Body)
	resp.Body.Close()
	out.Close()
	checkf(err, "save snapshot")
	fmt.Fprintf(os.Stderr, "[snapshot] saved %d bytes to %s\n", n, tmpFile)

	// Read snapshot header to extract trust height + BPT root. We use
	// these to a) verify against the signed-anchor pool, b) tell
	// CometBFT state-sync what height+hash to trust. The snapshot bytes
	// themselves are NOT restored locally — CometBFT state-sync will
	// pull a fresh snapshot from peers via ABCI hooks on first daemon
	// start, avoiding any auxiliary-state divergence.
	headerFile, err := os.Open(tmpFile)
	checkf(err, "open snapshot for header read")
	v2r, herr := snapshotV2.Open(headerFile)
	checkf(herr, "read snapshot header")
	headerFile.Close()
	if v2r.Header == nil || v2r.Header.SystemLedger == nil {
		fatalf("snapshot has no SystemLedger; cannot determine block height")
	}
	snapHeight := v2r.Header.SystemLedger.Index
	snapTime := v2r.Header.SystemLedger.Timestamp
	root := v2r.Header.RootHash
	fmt.Fprintf(os.Stderr, "[snapshot] header height=%d root=%x (no local restore — state-sync handles it)\n", snapHeight, root[:8])

	// Restore the snapshot into an in-memory DB so we can resolve the
	// validator keypage when verifying signed anchors. We discard the
	// in-memory DB after verification; the daemon will get its own
	// state via CometBFT state-sync.
	db := database.OpenInMemory(nil)
	db.SetObserver(execute.NewDatabaseObserver())
	defer db.Close()
	rf, err := os.Open(tmpFile)
	checkf(err, "open snapshot for in-memory restore")
	defer rf.Close()
	scope := protocol.PartitionUrl(flagBootstrap.Partition)
	netURL := config.NetworkUrl{URL: scope}
	fmt.Fprintf(os.Stderr, "[verify] in-memory restore for keypage lookup ...\n")
	err = snapshot.FullRestore(db, rf, nil, netURL)
	checkf(err, "in-memory restore")

	// Verify against a validator-quorum-signed anchor.
	// This is the trust step: the peer served us snapshot bytes, but
	// the bytes' authenticity comes from our local verification of a
	// signed major-block anchor whose StateTreeAnchor equals our
	// computed root. Without this we'd be trusting the peer.
	if flagBootstrap.PeerWS == "" || flagBootstrap.PeerAnchorPool == "" {
		fmt.Fprintf(os.Stderr, "[snapshot] WARN: --peer + --peer-anchor-pool not set; skipping signature verification.\n")
		fmt.Fprintf(os.Stderr, "[snapshot] node remains in BOOTING; rerun with verification flags or run accumulated bootstrap manually with --anchor-hex %x to promote.\n", root)
		return
	}

	ctx := context.Background()
	ws, err := websocket.NewClient(flagBootstrap.PeerWS, flagBootstrap.Network)
	checkf(err, "dial peer for anchor verification")
	defer ws.Close()
	poolURL, err := url.Parse(flagBootstrap.PeerAnchorPool)
	checkf(err, "parse --peer-anchor-pool")
	as, err := anchorsrc.New(ws, poolURL, db)
	checkf(err, "build anchor source")

	fmt.Fprintf(os.Stderr, "[verify] searching for signed anchor matching local root...\n")
	majorBlock, ok, err := as.FindAnchor(ctx, flagBootstrap.Partition, root)
	checkf(err, "find anchor")
	if !ok {
		fatalf("no validator-quorum-signed anchor matches local root %x — refusing to promote (peer may have served a tampered snapshot)", root)
	}
	fmt.Fprintf(os.Stderr, "[verify] OK — local root matches signed anchor for major block %d\n", majorBlock)

	// Promote the state machine.
	machine := nodestate.New()
	if !machine.PromoteToActive(root, majorBlock) {
		fatalf("PromoteToActive failed (machine in unexpected state)")
	}
	ad := machine.Get()

	// Persist so accumulated run picks it up.
	art := &bootpersist.Artifact{
		Network:   flagBootstrap.Network,
		Partition: flagBootstrap.Partition,
		Resume: bootpersist.ResumeConfig{
			PeerWS:         flagBootstrap.PeerWS,
			PeerAnchorPool: flagBootstrap.PeerAnchorPool,
		},
		State: bootpersist.StateRecord{
			Current:        ad.State.String(),
			SinceBlock:     ad.SinceBlock,
			VerifiedAnchor: ad.VerifiedAnchor,
			EnteredActive:  time.Now().UTC(),
		},
		Phases: bootpersist.Phases{
			SpinePullDone: true,
			EnumerateDone: true,
		},
	}
	if err := bootpersist.Save(dataDir, art); err != nil {
		fatalf("save bootstrap-state.json: %v", err)
	}

	fmt.Fprintf(os.Stderr, "[ACTIVE] node is ACTIVE at major block %d (anchor=%x)\n", majorBlock, root[:16])
	fmt.Fprintf(os.Stderr, "[ACTIVE] persisted to %s\n", dataDir)

	if flagBootstrap.WriteConfig {
		if err := writeDaemonConfig(dataDir, root, snapHeight, snapTime); err != nil {
			fatalf("write daemon config: %v", err)
		}
	} else {
		fmt.Fprintf(os.Stderr, "[ACTIVE] hand off: 'accumulated run' to catch up to current via consensus\n")
	}
}

// writeDaemonConfig produces the layout that `accumulated run`
// expects after a snapshot-based bootstrap:
//
//	<dataDir>/accumulate.toml      — coreValidator config
//	<dataDir>/<dnn|bvnn>/data/accumulate.db   — the BPT db (moved
//	                                            from bootstrap-<P>.db)
//	<dataDir>/<dnn|bvnn>/config/  — created on first run by daemon
//
// The node + validator keys are freshly generated. The validator key
// is NOT registered in any operator keypage on the network, so the
// resulting node will run as a non-validator full node (CometBFT
// follower) which is exactly what we want for a bootstrapped
// follower.
//
// Note: this writes config only. Two follow-up gaps still block an
// actual `accumulated run`:
//   - #4002: daemon must skip InitChain when bootstrap-state.json
//     says ACTIVE (we already have BPT loaded).
//   - #4003: CometBFT block-store catch-up from snapshot height B
//     to current — without state-sync support, the daemon's
//     consensus path needs to know how to reconcile a non-zero
//     starting height.
func writeDaemonConfig(dataDir string, appHash [32]byte, snapHeight uint64, snapTime time.Time) error {
	mode, dir := partitionToCoreMode(flagBootstrap.Partition)

	// We do NOT move the BPT db. CometBFT state-sync (configured by
	// writeStateSyncConfig below) handles the snapshot transfer +
	// FullRestore on first daemon start via the ABCI snapshot hooks.
	// Just ensure the data dir exists so state-sync's tmp dir resolves.
	dataPath := filepath.Join(dataDir, dir, "data")
	if err := os.MkdirAll(dataPath, 0755); err != nil {
		return fmt.Errorf("mkdir db parent: %w", err)
	}

	// Generate fresh ed25519 keys for node (P2P) and validator
	// (consensus). The validator key is unprivileged; node runs as
	// follower.
	nodePub, nodePriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate node key: %w", err)
	}
	valPub, valPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate validator key: %w", err)
	}
	_ = nodePub
	_ = valPub

	listenAddr, err := multiaddr.NewMultiaddr(flagBootstrap.TmListen)
	if err != nil {
		return fmt.Errorf("parse --tm-listen: %w", err)
	}

	var bsPeers []multiaddr.Multiaddr
	if flagBootstrap.TmBootstrap != "" {
		for _, s := range strings.Split(flagBootstrap.TmBootstrap, ",") {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			ma, err := multiaddr.NewMultiaddr(s)
			if err != nil {
				return fmt.Errorf("parse tm bootstrap peer %q: %w", s, err)
			}
			bsPeers = append(bsPeers, ma)
		}
	}

	cfg := new(run.Config)
	cfg.Network = flagBootstrap.Network
	cfg.Logging = new(run.Logging)
	cfg.P2P = new(run.P2P)
	cfg.P2P.Key = &run.RawPrivateKey{Address: address.FromED25519PrivateKey(nodePriv).String()}

	cvc := run.AddConfiguration(cfg, new(run.CoreValidatorConfiguration), nil)
	cfg.Configurations = []run.Configuration{cvc}
	cvc.Mode = mode
	cvc.Listen = listenAddr
	cvc.ValidatorKey = &run.RawPrivateKey{Address: address.FromED25519PrivateKey(valPriv).String()}
	// Set DnGenesis/BvnGenesis to a workdir-level filename so the run
	// framework's genesisDocProvider (run/consensus.go) resolves it
	// against inst.workDir, not the workdir itself. The .json
	// extension routes through the JSON-doc provider, matching how we
	// fetch the genesis doc from the peer's RPC.
	if mode == run.CoreValidatorModeBVN {
		cvc.BVN = flagBootstrap.Partition
		cvc.BvnBootstrapPeers = bsPeers
		cvc.BvnGenesis = strings.ToLower(flagBootstrap.Partition) + "-genesis.json"
	} else {
		cvc.DnBootstrapPeers = bsPeers
		cvc.DnGenesis = "directory-genesis.json"
	}

	tomlPath := filepath.Join(dataDir, "accumulate.toml")
	if err := cfg.SaveTo(tomlPath); err != nil {
		return fmt.Errorf("save accumulate.toml: %w", err)
	}
	fmt.Fprintf(os.Stderr, "[config] wrote %s\n", tomlPath)

	// Pre-populate CometBFT state DB so the daemon's first run sees a
	// node already at snapHeight, not a fresh chain at height 0. This
	// closes the height-mismatch handshake (#4004 option B).
	tmRPCs := splitNonempty(flagBootstrap.TmRpcServers, ",")
	tmP2P := splitNonempty(flagBootstrap.TmP2PPeers, ",")
	if len(tmRPCs) >= 2 {
		nodeDir := filepath.Join(dataDir, dir)
		var genesisFilename string
		if mode == run.CoreValidatorModeBVN {
			genesisFilename = cvc.BvnGenesis
		} else {
			genesisFilename = cvc.DnGenesis
		}
		if err := writeStateSyncConfig(nodeDir, genesisFilename, tmRPCs, tmP2P, appHash, snapHeight, snapTime); err != nil {
			return fmt.Errorf("write state-sync config: %w", err)
		}
	} else {
		fmt.Fprintf(os.Stderr, "[config] WARN: --tm-rpc-servers not set (need ≥2 for state-sync); skipping tendermint.toml.\n")
	}

	fmt.Fprintf(os.Stderr, "[config] hand off: 'accumulated %s' to start the node\n", dataDir)
	return nil
}

// splitNonempty is strings.Split followed by trim+drop-empty.
func splitNonempty(s, sep string) []string {
	var out []string
	for _, x := range strings.Split(s, sep) {
		x = strings.TrimSpace(x)
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}

// partitionToCoreMode returns (mode, nodeDir) for the partition the
// launcher just bootstrapped. nodeDir is the subdir inside the data
// dir where the daemon stores per-partition state ("dnn" or "bvnn").
func partitionToCoreMode(partition string) (run.CoreValidatorMode, string) {
	if strings.EqualFold(partition, protocol.Directory) {
		return run.CoreValidatorModeDN, "dnn"
	}
	return run.CoreValidatorModeBVN, "bvnn"
}

// devAnchorSource — dev-only fixed-anchor source. Production
// replacement is anchorsrc.Source per #3988.
type devAnchorSource struct {
	block  uint64
	anchor [32]byte
}

func newDevAnchorSource(block uint64, hexAnchor string) *devAnchorSource {
	var a [32]byte
	if hexAnchor != "" {
		raw, err := hex.DecodeString(hexAnchor)
		if err != nil || len(raw) != 32 {
			fatalf("--anchor-hex must be 64 hex chars (32 bytes), got %d bytes / err=%v", len(raw), err)
		}
		copy(a[:], raw)
	}
	return &devAnchorSource{block: block, anchor: a}
}

func (s *devAnchorSource) LatestAnchor(_ context.Context, _ string) (uint64, [32]byte, error) {
	return s.block, s.anchor, nil
}

// restoreMachine builds a nodestate.Machine from a persisted
// artifact. Falls back to a fresh BOOTING machine if the persisted
// state is malformed.
func restoreMachine(art *bootpersist.Artifact) *nodestate.Machine {
	st, err := nodestate.ParseState(art.State.Current)
	if err != nil {
		return nodestate.New()
	}
	m, err := nodestate.Restore(st, art.State.SinceBlock, art.State.VerifiedAnchor, art.State.HistoryDepth)
	if err != nil {
		return nodestate.New()
	}
	return m
}

// saveState writes the current advertisement back to disk. Called
// from Machine.OnChange.
func saveState(dataDir string, ad nodestate.Advertisement) error {
	art, err := bootpersist.Load(dataDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if art == nil {
		art = &bootpersist.Artifact{}
	}
	art.Network = flagBootstrap.Network
	art.Partition = flagBootstrap.Partition
	art.Resume.PeerWS = flagBootstrap.PeerWS
	art.Resume.PeerAnchorPool = flagBootstrap.PeerAnchorPool
	art.State.Current = ad.State.String()
	art.State.SinceBlock = ad.SinceBlock
	art.State.VerifiedAnchor = ad.VerifiedAnchor
	art.State.HistoryDepth = ad.HistoryDepth
	return bootpersist.Save(dataDir, art)
}

// savePhase records that a phase boundary completed. Called from
// orchestrator OnPhase. Phase names ("spine", "enumerate", "steady",
// "active") map to the Phases struct's bools.
func savePhase(dataDir, phase string) error {
	art, err := bootpersist.Load(dataDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if art == nil {
		art = &bootpersist.Artifact{
			Network:   flagBootstrap.Network,
			Partition: flagBootstrap.Partition,
			Resume: bootpersist.ResumeConfig{
				PeerWS:         flagBootstrap.PeerWS,
				PeerAnchorPool: flagBootstrap.PeerAnchorPool,
			},
		}
	}
	switch phase {
	case "spine":
		// Marked done when its message is the second spine call (see
		// orchestrator's two phase("spine", …) sites — we record the
		// completion side-effectfully).
		art.Phases.SpinePullDone = true
	case "enumerate":
		art.Phases.EnumerateDone = true
	}
	return bootpersist.Save(dataDir, art)
}
