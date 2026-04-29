// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/anchorsrc"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/bootpersist"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/clientsrc"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/orchestrator"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/websocket"
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
}

func runBootstrap(cmd *cobra.Command, _ []string) {
	if flagBootstrap.Partition == "" {
		fatalf("--partition required")
	}
	if flagBootstrap.PeerWS == "" {
		fatalf("--peer required")
	}
	if flagBootstrap.Network == "" {
		fatalf("--network required")
	}
	if flagBootstrap.PeerAnchorPool == "" && flagBootstrap.AnchorHex == "" {
		fatalf("either --peer-anchor-pool (production AnchorSource) or --anchor-hex (dev) required")
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
		},
	}

	if err := orchestrator.Run(ctx, src, evCh, db, machine, opts); err != nil {
		fatalf("bootstrap: %v", err)
	}

	ad := machine.Get()
	if ad.State == nodestate.StateActive {
		fmt.Fprintf(os.Stderr, "ACTIVE at block %d (anchor=%x…)\n", ad.SinceBlock, ad.VerifiedAnchor[:8])
		return
	}
	fmt.Fprintf(os.Stderr, "exited in state %s\n", ad.State)
	os.Exit(2)
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
