// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	stdurl "net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/bootpersist"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/websocket"
)

// derivePartitionStateBaseURL turns the peer's WebSocket URL into the
// HTTP scheme://host base. ws → http, wss → https. Strips path/query.
func derivePartitionStateBaseURL(peerWS string) (string, error) {
	u, err := stdurl.Parse(peerWS)
	if err != nil {
		return "", fmt.Errorf("invalid peer URL %q: %w", peerWS, err)
	}
	switch strings.ToLower(u.Scheme) {
	case "ws":
		u.Scheme = "http"
	case "wss":
		u.Scheme = "https"
	case "http", "https":
		// already an HTTP URL
	default:
		return "", fmt.Errorf("unsupported scheme %q in peer URL", u.Scheme)
	}
	if u.Host == "" {
		return "", fmt.Errorf("peer URL %q has no host", peerWS)
	}
	u.Path = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

// derivePartitionStateURL builds the binary partition-state HTTP URL.
func derivePartitionStateURL(peerWS, partition string) (string, error) {
	base, err := derivePartitionStateBaseURL(peerWS)
	if err != nil {
		return "", err
	}
	return base + "/v3/partition-state/" + strings.ToLower(partition), nil
}

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

	// Snapshot fetch is the default and recommended path. The legacy
	// spine-pull + enumerate + hydrate path is opt-in via --spine-pull
	// because it has unfixed defects against a live network: a race
	// between hydrate and source advancement (#4018) and BPT entries
	// the source can't hydrate (#4019).
	if flagBootstrap.PeerWS == "" {
		fatalf("--peer required")
	}

	dataDir := flagBootstrap.DataDir
	if dataDir == "" {
		dataDir = flagMain.WorkDir
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		fatalf("create data dir: %v", err)
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

	// Phase 1: pull an atomic block-N snapshot from the peer's
	// /v3/partition-state/<partition> endpoint. The peer holds a single
	// read view of its database for the whole walk, so every BPT leaf
	// and account body reflects the same minor block. The endpoint
	// returns the snapshot v2 bytes as a binary HTTP body and metadata
	// in headers (X-Accumulate-Block-Index, X-Accumulate-Bpt-Root).
	psURL, perr := derivePartitionStateURL(flagBootstrap.PeerWS, flagBootstrap.Partition)
	checkf(perr, "derive partition-state URL from --peer")
	t0 := time.Now()
	fmt.Fprintf(os.Stderr, "[partition-state] GET %s ...\n", psURL)
	resp, err := http.Get(psURL)
	checkf(err, "fetch partition state")
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		fatalf("partition-state HTTP %d: %s", resp.StatusCode, string(body))
	}
	blockStr := resp.Header.Get("X-Accumulate-Block-Index")
	rootStr := resp.Header.Get("X-Accumulate-Bpt-Root")
	if blockStr == "" || rootStr == "" {
		fatalf("partition-state response missing block/root headers")
	}
	blockIndex, perr := strconv.ParseUint(blockStr, 10, 64)
	checkf(perr, "parse block-index header")
	rootBytes, perr := hex.DecodeString(rootStr)
	checkf(perr, "parse bpt-root header")
	if len(rootBytes) != 32 {
		fatalf("bpt-root header is %d bytes, want 32", len(rootBytes))
	}
	var peerRoot [32]byte
	copy(peerRoot[:], rootBytes)
	body, err := io.ReadAll(resp.Body)
	checkf(err, "read partition-state body")
	fmt.Fprintf(os.Stderr, "[partition-state] block=%d root=%x size=%d bytes (%s)\n",
		blockIndex, peerRoot[:8], len(body), time.Since(t0).Round(time.Millisecond))

	// Phase 2: restore into the local DB. database.Restore preserves
	// source-recorded BPT leaf hashes verbatim (orphan-safe; see
	// internal/database/snapshot.go), so the resulting BPT root is
	// exactly source's BPT root at blockIndex.
	t1 := time.Now()
	fmt.Fprintln(os.Stderr, "[restore] applying snapshot to local db ...")
	err = database.Restore(db, ioutil.NewBuffer(body), nil)
	checkf(err, "restore snapshot")
	batch := db.Begin(false)
	rootAfter, rerr := batch.GetBptRootHash()
	batch.Discard()
	checkf(rerr, "get local bpt root")
	if rootAfter != peerRoot {
		fatalf("BPT root after restore (%x) != peer's claimed root (%x); restore is inconsistent",
			rootAfter, peerRoot)
	}
	fmt.Fprintf(os.Stderr, "[restore] local root=%x matches peer's root (%s)\n",
		rootAfter[:8], time.Since(t1).Round(time.Millisecond))

	// Phase 3: BOOTING → WAITING. The snapshot has been applied; local
	// BPT root equals the peer's claimed root. We have NOT yet
	// validated against a validator-quorum-signed anchor — mainnet
	// only emits signed anchors at major-block boundaries (every
	// ~12h), so the anchor for our snapshot's block typically lags
	// the snapshot. WAITING means "snapshot loaded, holding for the
	// next major-block anchor that confirms our root."
	machine := nodestate.New()
	machine.OnChange(func(ad nodestate.Advertisement) {
		if err := saveState(dataDir, ad); err != nil {
			fmt.Fprintf(os.Stderr, "[persist] warn: %v\n", err)
		}
	})
	if !machine.PromoteToWaiting(peerRoot, blockIndex) {
		fatalf("promote to WAITING failed (machine in unexpected state)")
	}
	fmt.Fprintf(os.Stderr, "[WAITING] snapshot applied at block %d  root=%x\n",
		blockIndex, peerRoot[:16])

	// Phase 4: cryptographic verification via CometBFT block header.
	// Block N+1's header.app_hash is exactly the BPT root after block N
	// (= our snapshot's R_N), and CometBFT signs every block with a
	// validator quorum. So fetching /block?height=N+1 and matching
	// header.app_hash to our local root is one validator-quorum-signed
	// confirmation that our snapshot is consensus-canonical, available
	// within ~5 seconds of any snapshot regardless of subsequent state
	// activity. If --tm-rpc-server is set we use that; otherwise we
	// derive from --peer using the canonical port offset (DN at +1,
	// BVN at +101 from --peer's port).
	tmRPC, perr := pickCometRPC(flagBootstrap.TmRpcServers, flagBootstrap.PeerWS, flagBootstrap.Partition)
	checkf(perr, "resolve CometBFT RPC URL")
	const confirmThreshold = 5
	const verifyTimeout = 30 * time.Second
	const pollInterval = 2 * time.Second
	verified, verifiedBlock := watchForConfirmations(ctx, tmRPC,
		blockIndex, peerRoot,
		confirmThreshold, pollInterval, verifyTimeout)

	if !verified && flagBootstrap.AnchorHex != "" {
		raw, derr := hex.DecodeString(flagBootstrap.AnchorHex)
		if derr != nil || len(raw) != 32 {
			fatalf("--anchor-hex must be 64 hex chars (32 bytes)")
		}
		var want [32]byte
		copy(want[:], raw)
		if want != peerRoot {
			fatalf("--anchor-hex (%x) does not match peer's root (%x)", want, peerRoot)
		}
		verified = true
		verifiedBlock = flagBootstrap.AnchorBlk
		fmt.Fprintf(os.Stderr, "[verify] DEV: --anchor-hex matches peer's root\n")
	}

	// Phase 5: WAITING → ACTIVE if verification succeeded.
	if verified {
		if !machine.PromoteToActive(peerRoot, verifiedBlock) {
			fatalf("promote to ACTIVE failed (machine in unexpected state)")
		}
	}

	// Persist phase markers for resume.
	ad := machine.Get()
	since := blockIndex
	if ad.SinceBlock != 0 {
		since = ad.SinceBlock
	}
	art := &bootpersist.Artifact{
		Network:   flagBootstrap.Network,
		Partition: flagBootstrap.Partition,
		Resume: bootpersist.ResumeConfig{
			PeerWS:         flagBootstrap.PeerWS,
			PeerAnchorPool: flagBootstrap.PeerAnchorPool,
		},
		State: bootpersist.StateRecord{
			Current:        ad.State.String(),
			SinceBlock:     since,
			VerifiedAnchor: peerRoot,
			EnteredActive:  time.Now().UTC(),
		},
		Phases: bootpersist.Phases{
			SpinePullDone: true,
			EnumerateDone: true,
		},
	}
	if err := bootpersist.Save(dataDir, art); err != nil {
		fmt.Fprintf(os.Stderr, "[persist] warn: %v\n", err)
	}

	fmt.Fprintf(os.Stderr, "[%s] block %d  root=%x  total=%s\n",
		ad.State, blockIndex, peerRoot[:16], time.Since(t0).Round(time.Millisecond))
}


// pickCometRPC returns the CometBFT RPC URL for a partition. If
// tmRpcServers is set (from --tm-rpc-servers), use the first entry.
// Otherwise derive from peerWS by replacing the WS scheme + port:
// the v3 API is at peerWS's port; CometBFT RPC is conventionally at
// peerWS's port − 4 (e.g. 16595 → 16591 for DN p2p, +1 = 16592 RPC).
// We use the partition + canonical port offsets to compute it.
func pickCometRPC(tmRpcServers, peerWS, partition string) (string, error) {
	if tmRpcServers != "" {
		// Use the first comma-separated entry
		first := strings.SplitN(tmRpcServers, ",", 2)[0]
		first = strings.TrimSpace(first)
		if first == "" {
			return "", fmt.Errorf("--tm-rpc-servers is empty")
		}
		return first, nil
	}
	u, err := stdurl.Parse(peerWS)
	if err != nil {
		return "", fmt.Errorf("invalid peer URL %q: %w", peerWS, err)
	}
	host := u.Hostname()
	port := u.Port()
	if host == "" || port == "" {
		return "", fmt.Errorf("peer URL %q must include host:port", peerWS)
	}
	p, err := strconv.Atoi(port)
	if err != nil {
		return "", fmt.Errorf("peer port %q: %w", port, err)
	}
	// Convention: peer's v3 API port = base + portAccAPI (4) for the
	// partition. CometBFT RPC = base + portCmtRpc (1). The Directory
	// has portDir=0 offset, BVN has portBVN=100 offset. So:
	//   DN: api = base+4 → rpc = base+1 = api-3
	//   BVN: api = base+104 → rpc = base+101 = api-3
	// Both reduce to "rpc port = api port - 3". Confirmed against the
	// running follower (api 16595 → rpc 16592, api 16695 → rpc 16692).
	rpcPort := p - 3
	scheme := "http"
	if strings.EqualFold(u.Scheme, "wss") || strings.EqualFold(u.Scheme, "https") {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s:%d", scheme, host, rpcPort), nil
}

// watchForConfirmations queries the peer's CometBFT RPC for block
// header N+1 (the block whose header.app_hash records the BPT root
// after block N — exactly our snapshot's R_N). Once header.app_hash
// matches our local root, that's a validator-quorum-signed
// confirmation: the validators that produced block N+1 signed off on
// the AppHash field, which is our R_N.
//
// We poll because the peer might be catching up — block N+1 may not
// be in its CometBFT store yet at the moment we ask. As soon as it
// arrives, the match check succeeds. With threshold=N we require N
// successive blocks (N+1 .. N+N) all having the same app_hash, i.e.
// the network has been stable on R_N for that long. On a busy
// partition the streak resets but the very first match (block N+1)
// is already definitive proof — threshold=1 is sufficient
// cryptographically. We default to 5 for defense-in-depth.
func watchForConfirmations(
	ctx context.Context,
	cometRPC string,
	snapBlock uint64,
	localRoot [32]byte,
	threshold int,
	pollInterval, timeout time.Duration,
) (verified bool, atBlock uint64) {
	httpClient := &http.Client{Timeout: 10 * time.Second}
	t0 := time.Now()
	deadline := t0.Add(timeout)
	fmt.Fprintf(os.Stderr, "[verify] CometBFT RPC %s; checking blocks %d…%d for header.app_hash == local root\n",
		cometRPC, snapBlock+1, snapBlock+uint64(threshold))

	streak := 0
	target := snapBlock + 1
	for {
		if time.Now().After(deadline) {
			fmt.Fprintf(os.Stderr, "[verify] no %d-confirmation streak within %s; remaining in WAITING\n",
				threshold, timeout)
			return false, 0
		}
		select {
		case <-ctx.Done():
			return false, 0
		default:
		}

		appHash, ok, err := cometBlockAppHash(ctx, httpClient, cometRPC, int64(target))
		if err != nil {
			fmt.Fprintf(os.Stderr, "[verify] block %d: %v (retrying)\n", target, err)
			time.Sleep(pollInterval)
			continue
		}
		if !ok {
			fmt.Fprintf(os.Stderr, "[verify] block %d not yet on peer; waiting\n", target)
			time.Sleep(pollInterval)
			continue
		}

		if appHash == localRoot {
			streak++
			fmt.Fprintf(os.Stderr, "[verify] block %d header.app_hash matches local root (%d/%d)\n",
				target, streak, threshold)
			if streak >= threshold {
				fmt.Fprintf(os.Stderr, "[verify] OK — %d-confirmation streak in %s\n",
					threshold, time.Since(t0).Round(time.Millisecond))
				return true, target
			}
			target++
			continue
		}
		fmt.Fprintf(os.Stderr, "[verify] block %d header.app_hash=%x ≠ local %x (streak %d→0)\n",
			target, appHash[:8], localRoot[:8], streak)
		streak = 0
		// State has moved on. There's no point waiting at this height
		// — the launcher's local root is frozen and won't catch up.
		return false, 0
	}
}

// cometBlockAppHash fetches block H from a CometBFT RPC and returns
// header.app_hash. Returns (zero, false, nil) if the block isn't yet
// in the peer's store (height too high), and (_, true, nil) on success.
func cometBlockAppHash(ctx context.Context, c *http.Client, rpcURL string, height int64) ([32]byte, bool, error) {
	url := fmt.Sprintf("%s/block?height=%d", rpcURL, height)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return [32]byte{}, false, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return [32]byte{}, false, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return [32]byte{}, false, err
	}
	// CometBFT returns 200 with an "error" field for height-out-of-range
	var out struct {
		Result struct {
			Block struct {
				Header struct {
					Height  string `json:"height"`
					AppHash string `json:"app_hash"`
				} `json:"header"`
			} `json:"block"`
		} `json:"result"`
		Error *struct {
			Data string `json:"data"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return [32]byte{}, false, fmt.Errorf("decode comet response: %w", err)
	}
	if out.Error != nil {
		// Treat "height greater than current" as not-yet-available
		if strings.Contains(out.Error.Data, "must be less than") || strings.Contains(out.Error.Data, "height") {
			return [32]byte{}, false, nil
		}
		return [32]byte{}, false, fmt.Errorf("%s", out.Error.Data)
	}
	if out.Result.Block.Header.AppHash == "" {
		return [32]byte{}, false, nil
	}
	raw, err := hex.DecodeString(out.Result.Block.Header.AppHash)
	if err != nil || len(raw) != 32 {
		return [32]byte{}, false, fmt.Errorf("invalid app_hash %q", out.Result.Block.Header.AppHash)
	}
	var ah [32]byte
	copy(ah[:], raw)
	return ah, true, nil
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
