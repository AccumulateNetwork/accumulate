// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Command driver reproduces a wedged synthetic stream over a real (dockerized)
// network and verifies receiver-pull healing (#4064).
//
// It picks a sender lite account that routes to BVN1 and a recipient that routes
// to BVN2 (using the network's own routing table), funds the sender, and sends
// ACME sender -> recipient N times. Each send produces a BVN1->BVN2 synthetic;
// the network is configured to DROP the first one (ACC_DEBUG_DROP_SYNTHETIC),
// which wedges the stream. With synthetic healing enabled on BVN2, the missing
// message is pulled from BVN1 and every send is eventually delivered — which is
// what this driver asserts by polling the recipient's balance.
//
//	go run ./test/docker/synth-heal/driver -endpoint http://localhost:8080 \
//	    -source BVN1 -dest BVN2 -count 5
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
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
	destID := flag.String("dest", "", "partition the recipient must route to (default: any partition other than the faucet's)")
	faucetSeed := flag.String("faucet-seed", "FAUCET", "genesis faucet seed (matches init --faucet-seed)")
	count := flag.Int("count", 5, "number of cross-partition sends")
	timeout := flag.Duration("timeout", 4*time.Minute, "overall timeout")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	c := jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint(*endpoint, "v3"))
	c.Client.Timeout = 30 * time.Second
	Q := api.Querier2{Querier: c}

	// Learn the routing table so we can place accounts on specific partitions.
	ns, err := c.NetworkStatus(ctx, api.NetworkStatusOptions{Partition: protocol.Directory})
	fatalIf(err, "network status")
	tree, err := routing.NewRouteTree(ns.Routing)
	fatalIf(err, "route tree")

	// The genesis faucet account is pre-funded with ACME and infinite credits;
	// use it as the sender. It must not live on the destination partition, or the
	// sends would be intra-partition and produce no synthetic.
	senderKey, senderURL := faucetAccount(*faucetSeed)
	lid := senderURL.RootIdentity()
	srcPart, err := tree.Route(senderURL)
	fatalIf(err, "route faucet")

	// Choose a destination partition different from the faucet's, so the sends
	// are cross-partition and produce synthetics.
	dest := *destID
	if dest == "" {
		for _, p := range ns.Network.Partitions {
			if p.Type == protocol.PartitionTypeBlockValidator && !strings.EqualFold(p.ID, srcPart) {
				dest = p.ID
				break
			}
		}
	}
	if dest == "" || strings.EqualFold(dest, srcPart) {
		log.Fatalf("could not pick a destination partition distinct from the faucet's (%s)", srcPart)
	}

	_, recipientURL := keyRoutingTo(tree, dest)
	log.Printf("sender (faucet) %v -> %s", senderURL, srcPart)
	log.Printf("recipient       %v -> %s", recipientURL, dest)

	// Send ACME across the partition boundary N times. The first synthetic is
	// dropped by the network; the rest wedge behind it until healing recovers.
	nonce := uint64(time.Now().UTC().UnixMilli())
	const amount = 1 // whole ACME per send
	for i := 0; i < *count; i++ {
		env, err := build.Transaction().For(senderURL).
			SendTokens(amount, protocol.AcmePrecisionPower).To(recipientURL).
			SignWith(lid).Version(1).Timestamp(&nonce).PrivateKey(senderKey).Done()
		fatalIf(err, "build send %d", i)
		submit(ctx, c, env)
		log.Printf("submitted send %d/%d", i+1, *count)
	}

	// The recipient must eventually receive all N deposits. Without healing the
	// stream stays wedged and this never reaches the target.
	want := new(big.Int).SetUint64(uint64(*count) * amount * protocol.AcmePrecision)
	log.Printf("waiting for recipient balance to reach %v (healing must clear the wedge)...", want)
	deadline := time.Now().Add(*timeout)
	for {
		var acct *protocol.LiteTokenAccount
		_, err := Q.QueryAccountAs(ctx, recipientURL, nil, &acct)
		switch {
		case err == nil:
			bal := acct.TokenBalance()
			log.Printf("recipient balance = %v / %v", bal, want)
			if bal.Cmp(want) >= 0 {
				fmt.Println("PASS: synthetic stream wedged and healed; all deposits delivered")
				return
			}
		case errors.Is(err, errors.NotFound):
			log.Printf("recipient not yet created (all deposits still pending)")
		default:
			fatalIf(err, "query recipient")
		}

		if time.Now().After(deadline) {
			fmt.Println("FAIL: recipient did not receive all deposits before timeout (stream did not heal)")
			os.Exit(1)
		}
		time.Sleep(3 * time.Second)
	}
}

// keyRoutingTo brute-forces an ed25519 key whose ACME lite account routes to the
// given partition.
func keyRoutingTo(tree *routing.RouteTree, partition string) (ed25519.PrivateKey, *url.URL) {
	for i := 0; i < 1_000_000; i++ {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		fatalIf(err, "generate key")
		kh := sha256.Sum256(pub)
		u := protocol.LiteAuthorityForHash(kh[:]).JoinPath(protocol.ACME)
		part, err := tree.Route(u)
		fatalIf(err, "route")
		if part == partition {
			return priv, u
		}
	}
	log.Fatalf("could not find a key routing to %s", partition)
	return nil, nil
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

func submit(ctx context.Context, c *jsonrpc.Client, env *messaging.Envelope) []*url.TxID {
	subs, err := c.Submit(ctx, env, api.SubmitOptions{})
	fatalIf(err, "submit")
	var ids []*url.TxID
	for _, s := range subs {
		if !s.Success {
			if s.Status != nil && s.Status.Error != nil {
				log.Fatalf("submit rejected: %v", s.Status.Error)
			}
			log.Fatalf("submit rejected: %s", s.Message)
		}
		if s.Status != nil {
			ids = append(ids, s.Status.TxID)
		}
	}
	return ids
}

func waitFor(ctx context.Context, Q api.Querier2, id *url.TxID) {
	for i := 0; i < 120; i++ {
		r, err := Q.QueryMessage(ctx, id, nil)
		switch {
		case errors.Is(err, errors.NotFound):
		case err != nil:
			fatalIf(err, "query %v", id)
		case r.Status.Delivered():
			if r.Error != nil {
				log.Fatalf("%v failed: %v", id, r.Error)
			}
			return
		}
		time.Sleep(time.Second)
	}
	log.Fatalf("timed out waiting for %v", id)
}

func fatalIf(err error, format string, args ...any) {
	if err != nil {
		log.Fatalf("%s: %v", fmt.Sprintf(format, args...), err)
	}
}
