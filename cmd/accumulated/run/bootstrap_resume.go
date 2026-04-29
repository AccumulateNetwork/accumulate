// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Bootstrap-v3 resume hook for `accumulated run`.
//
// `accumulated bootstrap` (#3986) drives a fresh node to ACTIVE and
// exits. If it crashes mid-bootstrap (state == BOOTING in
// bootpersist), the next `accumulated run` is supposed to pick up
// where it left off — running only the steady-state portion of the
// orchestrator until the tracker promotes.
//
// This file provides ResumeIfBooting, the function the daemon's
// startup path calls AFTER daemon.Start completes (per the
// "after, and sync" decision on #3989). It is synchronous: it
// returns nil only after the machine reaches ACTIVE / COMPLETE, the
// stream closes, or ctx is canceled.
//
// Wiring: the daemon must call this with its DB handle and a v3
// client. The DB handle isn't currently exposed publicly on
// *accumulated.Daemon — only DB_TESTONLY exists. A separate
// daemon-side change is needed before this hook can be wired in;
// see the integration note in #3989.
package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/anchorsrc"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/bootpersist"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/clientsrc"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/orchestrator"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/tracker"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/websocket"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ResumeIfBooting reads the bootstrap-v3 persisted state from
// dataDir. If absent or already ACTIVE/COMPLETE, returns nil
// immediately. If BOOTING, runs the orchestrator's steady-state
// phase synchronously until promotion or ctx cancel.
//
// Returns nil when:
//   - No persisted state exists (the node isn't a bootstrap-v3 node).
//   - Persisted state is ACTIVE/COMPLETE (already done).
//   - Resume completes promotion.
//   - ctx is canceled cleanly.
//
// Returns an error when:
//   - Loading or persistence fails for non-NotExist reasons.
//   - The peer dial fails.
//   - The orchestrator's steady-state loop returns an error.
func ResumeIfBooting(ctx context.Context, dataDir string, db *database.Database) error {
	art, err := bootpersist.Load(dataDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil // not a bootstrap-v3 node
	}
	if err != nil {
		return fmt.Errorf("load bootstrap state: %w", err)
	}

	state, err := nodestate.ParseState(art.State.Current)
	if err != nil {
		return fmt.Errorf("parse persisted state %q: %w", art.State.Current, err)
	}
	if state != nodestate.StateBooting {
		return nil // already past BOOTING
	}

	if art.Resume.PeerWS == "" {
		return fmt.Errorf("BOOTING state but no Resume.PeerWS — re-run accumulated bootstrap to initialize")
	}
	if art.Partition == "" {
		return fmt.Errorf("BOOTING state but no Partition recorded")
	}

	// Connect to the peer.
	ws, err := websocket.NewClient(art.Resume.PeerWS, art.Network)
	if err != nil {
		return fmt.Errorf("dial bootstrap peer %s: %w", art.Resume.PeerWS, err)
	}
	defer ws.Close()

	// Build the AnchorSource per the persisted config.
	var anchors orchestrator.AnchorSource
	if art.Resume.PeerAnchorPool != "" {
		poolURL, perr := url.Parse(art.Resume.PeerAnchorPool)
		if perr != nil {
			return fmt.Errorf("parse PeerAnchorPool: %w", perr)
		}
		as, aerr := anchorsrc.New(ws, poolURL, db)
		if aerr != nil {
			return fmt.Errorf("build AnchorSource: %w", aerr)
		}
		anchors = as
	} else {
		return fmt.Errorf("BOOTING state but Resume.PeerAnchorPool is empty — production AnchorSource cannot be wired")
	}

	src := clientsrc.New(ws, anchors)

	// Restore machine + tracker.
	machine, err := nodestate.Restore(state, art.State.SinceBlock, art.State.VerifiedAnchor, art.State.HistoryDepth)
	if err != nil {
		return fmt.Errorf("restore machine: %w", err)
	}
	machine.OnChange(func(ad nodestate.Advertisement) {
		if err := saveStateAt(dataDir, art, ad); err != nil {
			fmt.Fprintf(os.Stderr, "[bootstrap-resume] persist warning: %v\n", err)
		}
	})

	tr, err := tracker.New(db, machine)
	if err != nil {
		return fmt.Errorf("tracker: %w", err)
	}
	for _, o := range art.ObservedAnchors {
		tr.Observe(o.Block, o.Anchor)
	}

	// Subscribe to events for steady-state ingestion.
	evCh, err := ws.Subscribe(ctx, api.SubscribeOptions{
		Partition: art.Partition,
	})
	if err != nil {
		return fmt.Errorf("subscribe to %s events: %w", art.Partition, err)
	}

	scope := protocol.PartitionUrl(art.Partition)
	opts := orchestrator.Options{
		Partition:          art.Partition,
		PartitionURL:       scope,
		IsDirectory:        art.Partition == protocol.Directory,
		AnchorPollInterval: 5 * time.Second,
		OnPhase: func(phase, msg string) {
			fmt.Fprintf(os.Stderr, "[bootstrap-resume %s] %s\n", phase, msg)
		},
	}
	return orchestrator.RunSteady(ctx, src, evCh, db, machine, tr, opts)
}

// saveStateAt updates the persisted artifact's state record. Used
// from the Machine.OnChange callback during resume.
func saveStateAt(dataDir string, base *bootpersist.Artifact, ad nodestate.Advertisement) error {
	art := *base // shallow copy
	art.State.Current = ad.State.String()
	art.State.SinceBlock = ad.SinceBlock
	art.State.VerifiedAnchor = ad.VerifiedAnchor
	art.State.HistoryDepth = ad.HistoryDepth
	return bootpersist.Save(dataDir, &art)
}
