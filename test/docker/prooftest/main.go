// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Command prooftest verifies the two-call account proof (#4272, #4274) against
// a running network, over the wire, as an external verifier would.
//
// The in-process tests build the receipts from the simulator's databases. This
// builds them from JSON-RPC responses on a deployed node, which is the only way
// to find out whether the service is served, routed and serialised correctly —
// three things a simulator test cannot fail on.
//
//	go run ./test/docker/prooftest -endpoint http://localhost:26680/v3
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var (
	flagEndpoint = flag.String("endpoint", "http://localhost:26680/v3", "a node's v3 JSON-RPC endpoint")
	flagBvn      = flag.String("bvn", "BVN1", "the BVN whose account to prove")
	flagAccount  = flag.String("account", "", "the account to prove (default: the BVN's ledger, which changes every block)")
	flagWait     = flag.Duration("wait", 3*time.Minute, "how long to wait for the anchor to reach the directory")
	flagSettle   = flag.Duration("settle", 90*time.Second, "how long to let the chain move on, to prove the second call is stable")
)

var failed bool

func main() {
	flag.Parse()
	ctx := context.Background()
	c := jsonrpc.NewClient(*flagEndpoint)

	account := protocol.PartitionUrl(*flagBvn).JoinPath(protocol.Ledger)
	if *flagAccount != "" {
		var err error
		account, err = url.Parse(*flagAccount)
		check(err, "parse account")
	}

	// A directory account needs no second call: its BPT is already the
	// directory's.
	dn := receiptFor(ctx, c, protocol.DnUrl().JoinPath(protocol.Ledger))
	say("DN  %v: complete=%v partition=%q", protocol.DnUrl().JoinPath(protocol.Ledger), dn.Complete, dn.Partition)
	expect(dn.Complete, "a directory account's receipt must be complete")
	expect(dn.Partition == protocol.Directory, "a directory receipt must name the directory")

	// A BVN account's does. The flag must describe the account, not the node
	// that answered — the same node answered both.
	first := receiptFor(ctx, c, account)
	say("BVN %v: complete=%v partition=%q anchor=%x", account, first.Complete, first.Partition, first.Receipt.Anchor[:8])
	expect(!first.Complete, "a BVN account's receipt must not be complete")
	expect(first.Partition == *flagBvn, "the receipt must name the BVN it terminates at, got %q", first.Partition)
	expect(first.Receipt.Validate(nil), "an incomplete receipt must still validate on its own")

	var bptRoot [32]byte
	copy(bptRoot[:], first.Receipt.Anchor)

	// CALL 2 — bind that BPT root to a directory root. Anchored=false is a
	// wait, not a failure: the anchor carrying it has not arrived yet.
	say("waiting up to %v for the anchor to reach the directory...", *flagWait)
	deadline := time.Now().Add(*flagWait)
	var second *api.AnchorReceiptRecord
	for {
		var err error
		second, err = c.AnchorReceipt(ctx, api.AnchorReceiptOptions{
			Partition: first.Partition, BptRoot: bptRoot,
		})
		check(err, "anchor-receipt")
		if second.Anchored {
			break
		}
		if time.Now().After(deadline) {
			fail("the anchor never reached the directory within %v", *flagWait)
			report()
			return
		}
		time.Sleep(2 * time.Second)
	}
	say("call 2: anchored at directory block %d, terminates at %x", second.DirectoryBlock, second.Receipt.Anchor[:8])

	// COMPOSE — call 1 ends where call 2 starts.
	expect(fmt.Sprintf("%x", first.Receipt.Anchor) == fmt.Sprintf("%x", second.Receipt.Start),
		"the two calls must meet at the BPT root")
	joined, err := first.Receipt.Combine(second.Receipt)
	check(err, "combine")
	expect(joined.Validate(nil), "the joined receipt must validate")
	say("joined: %x .. %x  (%d entries)", joined.Start[:8], joined.Anchor[:8], len(joined.Entries))

	// The second call must be stable and must still work later. A caller may
	// record the pair once and come back much later; the chain moves on but the
	// terminus does not.
	say("letting the chain run for %v...", *flagSettle)
	time.Sleep(*flagSettle)

	late, err := c.AnchorReceipt(ctx, api.AnchorReceiptOptions{Partition: first.Partition, BptRoot: bptRoot})
	check(err, "anchor-receipt (late)")
	expect(late.Anchored, "the same root must still be found later")
	expect(late.DirectoryBlock == second.DirectoryBlock,
		"the terminus must not drift: was DN block %d, now %d", second.DirectoryBlock, late.DirectoryBlock)
	lateJoined, err := first.Receipt.Combine(late.Receipt)
	check(err, "combine (late)")
	expect(lateJoined.Validate(nil), "the late receipt must still compose and validate")
	say("still provable: DN block %d, same terminus", late.DirectoryBlock)

	// A caller that already trusts a later directory root can ask for one
	// reaching that instead.
	after := second.DirectoryBlock + 10
	later, err := c.AnchorReceipt(ctx, api.AnchorReceiptOptions{
		Partition: first.Partition, BptRoot: bptRoot, AtOrAfter: after,
	})
	check(err, "anchor-receipt (AtOrAfter)")
	expect(later.Anchored && later.Receipt != nil, "AtOrAfter must still find the root")
	expect(later.DirectoryBlock >= after, "AtOrAfter %d must be honoured, got %d", after, later.DirectoryBlock)
	atJoined, err := first.Receipt.Combine(later.Receipt)
	check(err, "combine (AtOrAfter)")
	expect(atJoined.Validate(nil), "the later receipt must compose and validate too")
	say("AtOrAfter %d: terminates at DN block %d, %x", after, later.DirectoryBlock, later.Receipt.Anchor[:8])

	// The spine methods (!1225) share the service. They are served by the
	// partition that owns the spine, not by the directory, so this also checks
	// that the three calls are not all routed to the same place.
	spine(ctx, c)

	report()
}

// spine exercises the major-block spine (!1225), which shares the service. It
// is the directory's and only the directory's, so a BVN must be refused rather
// than answered.
func spine(ctx context.Context, c *jsonrpc.Client) {
	run, err := c.MinorRootRange(ctx, api.MinorRootRangeOptions{Partition: protocol.Directory, Since: 1, Until: 20})
	if err != nil {
		fail("minor-root-range: %v", err)
	} else {
		expect(run.Anchor != nil, "minor-root-range returned no anchor")
		expect(run.RootProof != nil, "minor-root-range returned no root proof")
		say("spine: minor roots 1..20 -> anchor=%v sigs=%d updates=%d",
			run.Anchor != nil, len(run.Signatures), len(run.Updates))
	}

	// A network younger than its major-block schedule has no major blocks,
	// which is a refusal from the sequencer, not a failure here.
	heads, err := c.MajorHeaderRange(ctx, api.MajorHeaderRangeOptions{Partition: protocol.Directory, Start: 1, End: 4})
	if err != nil {
		say("spine: no major headers (%v)", err)
	} else {
		say("spine: major headers 1..4 -> %d", len(heads))
	}

	// The refusal must reach the client, not be turned into a routing failure.
	_, err = c.MinorRootRange(ctx, api.MinorRootRangeOptions{Partition: *flagBvn, Since: 1, Until: 20})
	expect(err != nil, "the spine must refuse a BVN, not answer for one")
	say("spine: %s refused, as it must (%v)", *flagBvn, err)
}

func receiptFor(ctx context.Context, c *jsonrpc.Client, u *url.URL) *api.Receipt {
	r, err := c.Query(ctx, u, &api.DefaultQuery{IncludeReceipt: &api.ReceiptOptions{ForAny: true}})
	check(err, "query %v", u)
	acct, ok := r.(*api.AccountRecord)
	if !ok {
		die("query %v returned %T, not an account record", u, r)
	}
	if acct.Receipt == nil {
		die("query %v returned no receipt", u)
	}
	return acct.Receipt
}

func say(format string, args ...any) { fmt.Printf(format+"\n", args...) }
func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "FATAL: "+format+"\n", args...)
	os.Exit(2)
}
func fail(format string, args ...any) { failed = true; fmt.Printf("FAIL: "+format+"\n", args...) }

func check(err error, format string, args ...any) {
	if err != nil {
		die("%s: %v", fmt.Sprintf(format, args...), err)
	}
}

func expect(ok bool, format string, args ...any) {
	if !ok {
		fail(format, args...)
	}
}

func report() {
	if failed {
		fmt.Println("\nFAILED")
		os.Exit(1)
	}
	fmt.Println("\nPASS — the two-call proof works over the wire")
}
