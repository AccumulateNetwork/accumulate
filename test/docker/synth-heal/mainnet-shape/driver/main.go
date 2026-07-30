// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Command driver wedges and heals the DN <-> BVN synthetic stream on a
// mainnet-shaped network: ONE node hosting the Directory and a single BVN.
//
// The sibling driver (../../driver) cannot be used here. It looks for a
// destination partition of type BlockValidator distinct from the faucet's, and
// on a one-BVN network no such partition exists, so it exits before sending
// anything. That assumption — at least two BVNs — is why mainnet's actual shape
// has never been covered.
//
// Traffic: AddCredits. Buying credits burns ACME through the ACME issuer, which
// lives on the Directory, and returns a credit deposit to the buyer's partition.
// So each purchase crosses the boundary in BOTH directions:
//
//	Cyclops -> DN    SyntheticBurnTokens
//	DN -> Cyclops    SyntheticDepositCredits   <-- the direction #4064 wedged
//
// The network drops the first cross-partition synthetic it emits, wedging the
// stream. Delivery is strictly ordered, so every later purchase queues behind
// the lost one and the credit balance stops rising. If healing works the gap
// closes on its own and the balance reaches the target; if it does not, this
// times out — which is precisely the #4064 incident, reproduced.
package main

import (
	"context"
	"crypto/ed25519"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	endpoint := flag.String("endpoint", "http://localhost:26660", "node JSON-RPC endpoint")
	faucetSeed := flag.String("faucet-seed", "FAUCET", "genesis faucet seed (matches init --faucet-seed)")
	count := flag.Int("count", 5, "number of credit purchases")
	spend := flag.Uint64("spend", 100, "ACME (whole tokens) to spend per purchase")
	timeout := flag.Duration("timeout", 7*time.Minute, "overall deadline")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	c := jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint(*endpoint, "v3"))
	c.Client.Timeout = 30 * time.Second
	Q := api.Querier2{Querier: c}

	ns, err := c.NetworkStatus(ctx, api.NetworkStatusOptions{Partition: protocol.Directory})
	fatalIf(err, "network status")
	log.Printf("executor version: %v", ns.ExecutorVersion)
	var parts []string
	for _, p := range ns.Network.Partitions {
		parts = append(parts, fmt.Sprintf("%s(%v)", p.ID, p.Type))
	}
	log.Printf("partitions: %s", strings.Join(parts, " "))

	key, acct := faucetAccount(*faucetSeed)
	lid := acct.RootIdentity()
	log.Printf("buyer: %v", acct)

	// Assert on the synthetic ledger, not on a derived credit balance. This is
	// the exact evidence #4064 was diagnosed from:
	//   received = 5398  delivered = 5314  pending = 84
	// A wedge is received > delivered with pending backed up behind it.
	// Watch BOTH directions. A credit purchase crosses twice — Cyclops->DN to
	// burn, DN->Cyclops to deposit — and the drop hook fires on whichever
	// envelope the node emits first, which in practice is the burn. Watching one
	// direction only reports a false failure while the other one wedges.
	type stream struct {
		name    string
		rxLedge *url.URL // the RECEIVER's synthetic ledger
		txLedge *url.URL // the SOURCE's synthetic ledger
		source  *url.URL
		dest    *url.URL
		d0      uint64
	}
	dn, cyc := protocol.DnUrl(), protocol.PartitionUrl("Cyclops")
	streams := []*stream{
		{name: "DN->Cyclops", rxLedge: cyc.JoinPath(protocol.Synthetic), txLedge: dn.JoinPath(protocol.Synthetic), source: dn, dest: cyc},
		{name: "Cyclops->DN", rxLedge: dn.JoinPath(protocol.Synthetic), txLedge: cyc.JoinPath(protocol.Synthetic), source: cyc, dest: dn},
	}
	for _, s := range streams {
		d, r, p := seq(ctx, Q, s.rxLedge, s.source)
		s.d0 = d
		log.Printf("%s before: produced=%d delivered=%d received=%d pending=%d",
			s.name, produced(ctx, Q, s.txLedge, s.dest), d, r, p)
	}

	nonce := uint64(time.Now().UTC().UnixMilli())
	oracle := float64(ns.Oracle.Price) / protocol.AcmeOraclePrecision
	for i := 0; i < *count; i++ {
		env, err := build.Transaction().For(acct).
			AddCredits().To(lid).WithOracle(oracle).Spend(float64(*spend)).
			SignWith(lid).Version(1).Timestamp(&nonce).PrivateKey(key).Done()
		fatalIf(err, "build purchase %d", i)
		submit(ctx, c, env)
		log.Printf("submitted purchase %d/%d", i+1, *count)
		time.Sleep(time.Second)
	}

	log.Printf("watching both streams (a wedge must open, then close)...")
	// Leave headroom: the watch deadline must expire before ctx, or the final
	// queries fail with "context deadline exceeded" instead of reporting state.
	deadline := time.Now().Add(*timeout - time.Minute)
	sawWedge := map[string]bool{}
	for {
		allClear, advanced := true, false
		var report []string
		for _, s := range streams {
			d, r, p := seq(ctx, Q, s.rxLedge, s.source)
			prod := produced(ctx, Q, s.txLedge, s.dest)
			// Two distinct failure shapes:
			//   visible   — received ran ahead of delivered; #4064 handles this.
			//   INVISIBLE — the source produced more than the receiver ever
			//               delivered, while received == delivered throughout.
			//               Every gap-based check reports healthy. Only #4073's
			//               interval reconcile can find it.
			gap := int64(r) - int64(d)
			shortfall := int64(prod) - int64(d)
			report = append(report, fmt.Sprintf("%s produced=%d d=%d r=%d p=%d", s.name, prod, d, r, p))
			if (gap > 0 || p > 0 || shortfall > 0) && !sawWedge[s.name] {
				kind := "visible gap"
				if gap <= 0 && p == 0 {
					kind = "INVISIBLE hole — received==delivered, only reconcile can see it"
				}
				log.Printf("WEDGE on %s [%s]: produced=%d delivered=%d received=%d pending=%d",
					s.name, kind, prod, d, r, p)
				sawWedge[s.name] = true
			}
			if gap > 0 || p > 0 || shortfall > 0 {
				allClear = false
			}
			if d > s.d0 {
				advanced = true
			}
		}
		if allClear && advanced {
			log.Printf("after: %s", strings.Join(report, " | "))
			if len(sawWedge) > 0 {
				var names []string
				for n := range sawWedge {
					names = append(names, n)
				}
				fmt.Printf("PASS: wedge opened on %s at v2-vandenberg and healed itself\n", strings.Join(names, ","))
				return
			}
			fmt.Printf("INCONCLUSIVE: streams advanced but no wedge was ever observed — the drop may not have landed\n")
			os.Exit(2)
		}
		if time.Now().After(deadline) {
			fmt.Printf("FAIL: still wedged: %s\n", strings.Join(report, " | "))
			os.Exit(1)
		}
		time.Sleep(3 * time.Second)
	}
}

// seq returns delivered/received/pending for one source on a synthetic ledger.
func seq(ctx context.Context, Q api.Querier2, ledger, source *url.URL) (delivered, received uint64, pending int) {
	e := entry(ctx, Q, ledger, source)
	if e == nil {
		return 0, 0, 0
	}
	return e.Delivered, e.Received, len(e.Pending)
}

// produced returns how many messages `ledger`'s partition has PRODUCED for peer.
//
// This is the only number that exposes an invisible hole. On a stream that
// loses its first or last messages, the receiver's received and delivered both
// stay put and stay equal, so every gap-based check reports healthy — the
// receiver cannot distinguish "nothing was sent" from "everything was lost".
// Comparing the source's produced count against the receiver's delivered count
// is what #4073's interval reconcile does, and it is what this driver must
// assert on.
func produced(ctx context.Context, Q api.Querier2, ledger, peer *url.URL) uint64 {
	e := entry(ctx, Q, ledger, peer)
	if e == nil {
		return 0
	}
	return e.Produced
}

func entry(ctx context.Context, Q api.Querier2, ledger, peer *url.URL) *protocol.PartitionSyntheticLedger {
	var acct *protocol.SyntheticLedger
	_, err := Q.QueryAccountAs(ctx, ledger, nil, &acct)
	if errors.Is(err, errors.NotFound) {
		return nil
	}
	fatalIf(err, "query %v", ledger)
	for _, s := range acct.Sequence {
		if s.Url != nil && s.Url.Equal(peer) {
			return s
		}
	}
	return nil
}

// faucetAccount derives the genesis faucet key and ACME account from its seed,
// matching cmd/accumulated's createFaucet.
func faucetAccount(seedStr string) (ed25519.PrivateKey, *url.URL) {
	var seed storage.Key
	for _, s := range strings.Split(seedStr, " ") {
		seed = seed.Append(s)
	}
	sk := ed25519.NewKeyFromSeed(seed[:])
	u, err := protocol.LiteTokenAddress(sk[32:], "ACME", protocol.SignatureTypeED25519)
	fatalIf(err, "faucet lite address")
	return sk, u
}

func submit(ctx context.Context, c *jsonrpc.Client, env *messaging.Envelope) {
	subs, err := c.Submit(ctx, env, api.SubmitOptions{})
	fatalIf(err, "submit")
	for _, s := range subs {
		if !s.Success {
			if s.Status != nil && s.Status.Error != nil {
				log.Fatalf("submit rejected: %v", s.Status.Error)
			}
			log.Fatalf("submit rejected: %s", s.Message)
		}
	}
}

func fatalIf(err error, format string, args ...any) {
	if err != nil {
		log.Fatalf("%s: %v", fmt.Sprintf(format, args...), err)
	}
}
