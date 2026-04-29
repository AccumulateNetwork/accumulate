// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/clientsrc"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/orchestrator"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/websocket"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// cmdBootstrap is `accumulated bootstrap`. It drives the
// bootstrap-v3 orchestrator from BOOTING to ACTIVE against a peer
// over the v3 WebSocket transport, then exits. accumulated run
// picks up from there (per #3989).
//
// The AnchorSource is currently a development-only stub configured
// via --anchor-hex. The real production AnchorSource ships per #3988
// and #3994 (signed-anchor retrieval); this CLI will switch to it
// without an interface change.
var cmdBootstrap = &cobra.Command{
	Use:   "bootstrap",
	Short: "Run bootstrap-v3: sync a node from BOOTING to ACTIVE",
	Long: `Bootstrap-v3 syncs a fresh node from a peer that already advertises
ACTIVE or COMPLETE. It pulls the partition's BPT, applies live block
events, and flips the node-state machine to ACTIVE when the local
BPT root matches a verified signed major-block anchor.

This subcommand is a foundation: production AnchorSource verification
is wired separately (issue #3988). For development, pass --anchor-hex
with a known-good BPT root to drive the orchestrator to completion.`,
	Run: runBootstrap,
}

var flagBootstrap = struct {
	Network    string
	Partition  string
	PeerWS     string
	DataDir    string
	PageSize   uint64
	AnchorHex  string
	AnchorBlk  uint64
	IsDirectoryPart bool
}{}

func init() {
	cmdMain.AddCommand(cmdBootstrap)
	f := cmdBootstrap.Flags()
	f.StringVar(&flagBootstrap.Network, "network", "", "Network identifier (mainnet/testnet/devnet/...)")
	f.StringVar(&flagBootstrap.Partition, "partition", "Directory", "Partition to bootstrap (Directory or BVN name)")
	f.StringVar(&flagBootstrap.PeerWS, "peer", "", "WebSocket URL of an ACTIVE/COMPLETE peer (e.g. ws://host:port/v3)")
	f.StringVar(&flagBootstrap.DataDir, "data-dir", "", "Bootstrap data directory (defaults to --work-dir)")
	f.Uint64Var(&flagBootstrap.PageSize, "page-size", 256, "Paginated query page size")
	f.StringVar(&flagBootstrap.AnchorHex, "anchor-hex", "", "DEV: hex-encoded expected BPT root that promotes ACTIVE on match")
	f.Uint64Var(&flagBootstrap.AnchorBlk, "anchor-block", 0, "DEV: block height associated with --anchor-hex")
}

func runBootstrap(cmd *cobra.Command, _ []string) {
	if flagBootstrap.Partition == "" {
		fatalf("--partition required (Directory or BVN name)")
	}
	if flagBootstrap.PeerWS == "" {
		fatalf("--peer required (WebSocket URL of an ACTIVE peer)")
	}
	if flagBootstrap.Network == "" {
		fatalf("--network required (mainnet/testnet/devnet/...)")
	}

	dataDir := flagBootstrap.DataDir
	if dataDir == "" {
		dataDir = flagMain.WorkDir
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		fatalf("create data dir: %v", err)
	}

	// Wire signal handling so Ctrl-C produces a clean cancel-and-exit.
	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigs
		fmt.Fprintln(os.Stderr, "received interrupt; shutting down…")
		cancel()
	}()
	defer cancel()

	// Connect to the peer over WebSocket. The Client implements
	// api.Querier and api.EventService; clientsrc composes the
	// Querier half with our AnchorSource into orchestrator.Source.
	ws, err := websocket.NewClient(flagBootstrap.PeerWS, flagBootstrap.Network)
	checkf(err, "dial peer %s", flagBootstrap.PeerWS)
	defer ws.Close()

	// Open a local DB next to the data dir.
	dbPath := filepath.Join(dataDir, "bootstrap-"+flagBootstrap.Partition+".db")
	db, err := database.OpenBadger(dbPath, nil)
	checkf(err, "open local db at %s", dbPath)

	// Construct the dev anchor source. Production replacement: #3988.
	anchors := newDevAnchorSource(flagBootstrap.AnchorBlk, flagBootstrap.AnchorHex)

	src := clientsrc.New(ws, anchors)

	// Subscribe to block events for the chosen partition.
	evCh, err := ws.Subscribe(ctx, api.SubscribeOptions{
		Partition: flagBootstrap.Partition,
	})
	checkf(err, "subscribe to %s events", flagBootstrap.Partition)

	// Compose orchestrator inputs.
	machine := nodestate.New()
	scope := protocol.PartitionUrl(flagBootstrap.Partition)
	isDN := flagBootstrap.Partition == protocol.Directory

	opts := orchestrator.Options{
		Partition:    flagBootstrap.Partition,
		PartitionURL: scope,
		IsDirectory:  isDN,
		PageSize:     flagBootstrap.PageSize,
		OnPhase: func(phase, msg string) {
			fmt.Fprintf(os.Stderr, "[%s] %s\n", phase, msg)
		},
	}

	if err := orchestrator.Run(ctx, src, evCh, db, machine, opts); err != nil {
		fatalf("bootstrap: %v", err)
	}

	ad := machine.Get()
	if ad.State == nodestate.StateActive {
		fmt.Fprintf(os.Stderr, "ACTIVE at block %d (anchor=%x)\n", ad.SinceBlock, ad.VerifiedAnchor[:8])
		return
	}
	fmt.Fprintf(os.Stderr, "exited in state %s (anchor poll did not produce a match — wire #3988 AnchorSource for production)\n", ad.State)
	os.Exit(2)
}

// devAnchorSource is the development-only AnchorSource: it always
// returns the configured (block, anchor) pair from --anchor-hex /
// --anchor-block. If no anchor is configured, it returns zero (which
// the tracker treats as "no anchor yet"), producing an indefinite
// run that exits only on context cancel.
//
// Production replacement: #3988. The orchestrator interface stays
// the same; only this concrete implementation gets swapped.
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
