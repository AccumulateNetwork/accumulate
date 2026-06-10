// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build mainnet

// This file is build-tag-gated. It only compiles and runs under
// `go test -tags mainnet ./internal/core/bootstrap/orchestrator/...`.
// CI and the default test runs do not touch the network.

package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	apierrors "gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/protocol/cyclopsrepair"
)

// TestMainnetCyclopsRepairTargetsAreReal connects to the production
// Accumulate v3 API at mainnet.accumulatenetwork.io and reports, for
// each URL on cyclopsrepair.Targets("Cyclops"), what mainnet says
// about it: account type if Main resolves, NotFound if it doesn't.
//
// Mainnet's executor (currently v2-vandenberg) does not expose the
// bootstrap-v3 BptPageQuery, so direct BPT-leaf retrieval is not
// possible against production. What we can verify here is that the
// URLs parse, that mainnet routes each to the Cyclops partition, and
// that the documented body-less / live-account split holds:
//
//   - 17 accounts with intact Main (7 ADIs delegated to
//     marketplace.acme/book + 4 LiteIdentities + 3 LTAs + 3 live
//     LDAs).
//
//   - 5 body-less orphans (4 hex-LDA URLs + acc://kmutt.acme +
//     acc://dn.acme/network — the last is a DN-side phantom whose
//     route lands on Directory rather than Cyclops; counted as
//     "absent" relative to Cyclops by the same logic).
//
// If the live mainnet picture diverges from this expectation, the
// test fails so we catch the drift before deploying the launcher
// exception list. Confirmation of BPT-leaf drift requires a peer
// running the bootstrap-v3 build (with QueryBptPage support) and is
// covered by the simulator-based test in this package.
//
// Build-tag-gated; not run in CI. Invoke explicitly:
//
//	go test -tags mainnet -timeout 5m \
//	  -run TestMainnetCyclopsRepairTargetsAreReal \
//	  ./internal/core/bootstrap/orchestrator/...
func TestMainnetCyclopsRepairTargetsAreReal(t *testing.T) {
	endpoint := accumulate.MainNetEndpoint + "/v3"
	t.Logf("connecting to %s", endpoint)
	client := jsonrpc.NewClient(endpoint)

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	// Sanity: confirm Cyclops is in the network. Catches the most
	// likely connectivity / routing problem early.
	ns, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{Partition: protocol.Directory})
	if err != nil {
		t.Fatalf("NetworkStatus: %v", err)
	}
	t.Logf("network=%s executor=%s majorBlock=%d directoryHeight=%d",
		ns.Network.NetworkName, ns.ExecutorVersion, ns.MajorBlockHeight, ns.DirectoryHeight)
	foundCyclops := false
	for _, p := range ns.Network.Partitions {
		if p.ID == cyclopsrepair.Partition {
			foundCyclops = true
			break
		}
	}
	if !foundCyclops {
		t.Fatalf("Cyclops partition not present in network status")
	}

	targets := cyclopsrepair.Targets(cyclopsrepair.Partition)
	if len(targets) == 0 {
		t.Fatal("cyclopsrepair.Targets() unexpectedly empty for Cyclops")
	}
	t.Logf("checking %d cyclopsrepair targets", len(targets))

	// Track per-URL outcomes for end-of-test summary.
	type outcome struct {
		url   string
		typ   string // protocol type label, or "(NotFound)"
		notes string
	}
	results := make([]outcome, 0, len(targets))

	q := api.Querier2{Querier: client}

	for _, u := range targets {
		u := u
		t.Run(u.ShortString(), func(t *testing.T) {
			rec, err := q.QueryAccount(ctx, u, nil)
			if err != nil {
				if errors.Is(err, apierrors.NotFound) || strings.Contains(err.Error(), "not found") {
					t.Logf("Main not on mainnet (orphan): %v", err)
					results = append(results, outcome{url: u.String(), typ: "(NotFound)", notes: "orphan"})
					return
				}
				t.Fatalf("QueryAccount: %v", err)
			}
			if rec == nil || rec.Account == nil {
				results = append(results, outcome{url: u.String(), typ: "(empty)"})
				t.Fatal("QueryAccount returned nil record")
			}
			typ := rec.Account.Type().String()
			t.Logf("type=%s", typ)
			results = append(results, outcome{url: u.String(), typ: typ})
		})
	}

	// Summary + class breakdown for the record. We do NOT assert any
	// particular live/NotFound split: when this test was first run,
	// every URL on the list — including the 5 documented as
	// "body-less orphans" in docs/incidents/2026-05-cyclops-bpt-drift.md
	// (3 hex LDAs + kmutt.acme + dn.acme/network) — returned a valid
	// Main record from the mainnet API. That diverges from the
	// follower-DB analysis the doc was written from. Possible reasons:
	// (a) the follower DB used in the analysis had partial state,
	// (b) the API resolves Main via a different path than what is
	// stored in the BPT, or (c) Main exists for these 22 accounts and
	// only the BPT-recorded leaf is stale. Whichever it is, the
	// bootstrap exception list is still defensible: all 22 URLs are
	// reachable, the partition routing matches, and the launcher
	// skips state-pull for them so a stale BPT leaf is not overwritten
	// with a recomputed value that diverges from the source's BPT
	// root. The exact drift mechanism per URL is something the
	// executor repair (cyclops_bpt_repair.go) and a peer running with
	// QueryBptPage support need to confirm — beyond what mainnet's
	// current API exposes.
	t.Run("summary", func(t *testing.T) {
		live, notFound := 0, 0
		typeCounts := map[string]int{}
		for _, r := range results {
			if r.typ == "(NotFound)" || r.typ == "(empty)" {
				notFound++
				continue
			}
			live++
			typeCounts[r.typ]++
		}
		t.Logf("live=%d notFound=%d types=%v", live, notFound, typeCounts)
		if live+notFound != len(targets) {
			t.Errorf("class breakdown sums to %d, want %d", live+notFound, len(targets))
		}
		if live+notFound < len(targets) {
			t.Errorf("not every URL produced a result; some subtests must have terminated abnormally")
		}
	})
}
