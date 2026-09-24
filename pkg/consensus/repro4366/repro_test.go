// Throwaway reproduction for #4366 (debugger, 2026-09-19). Not for merge as-is.
package repro4366

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
)

const (
	nVals     = 4
	part      = "BVN3"
	runFor    = 25 * time.Second
	valPrefix = "VAL-TX-"
	folPrefix = "FOL-TX-"
)

type node struct {
	name string
	n    *consensus.Node

	mu        sync.Mutex
	committed map[string]bool // transaction bytes seen in a committed block
	blocks    int
}

// TestFollowerAcceptsAndStrands: five nodes on one partition, four of them the
// committee. The fifth's key is in no committee — the soak's follower. Both
// are handed valid transactions through the same entry point the API's
// Submit calls. The committee's transactions are committed; the follower's
// are accepted and never appear in any node's committed block, the follower's
// own included, while the follower executes every block the committee makes.
func TestFollowerAcceptsAndStrands(t *testing.T) {
	keys := make([]ed25519.PrivateKey, nVals+1)
	infos := make([]types.ValidatorInfo, nVals)
	for i := range keys {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		keys[i] = priv
		if i < nVals {
			infos[i] = types.ValidatorInfo{PublicKey: pub, Stake: 100}
		}
	}
	// The committee is the ACTIVE validators, exactly as the NetworkDefinition
	// gives it. The follower holds the same committee; it is simply not in it.
	committee := types.NewCommittee(infos, 1)

	hosts := make([]host.Host, nVals+1)
	for i := range hosts {
		h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
		if err != nil {
			t.Fatal(err)
		}
		hosts[i] = h
		defer h.Close()
	}
	ctx0 := context.Background()
	for i := range hosts {
		for j := i + 1; j < len(hosts); j++ {
			if err := hosts[i].Connect(ctx0, peer.AddrInfo{ID: hosts[j].ID(), Addrs: hosts[j].Addrs()}); err != nil {
				t.Fatal(err)
			}
		}
	}

	nodes := make([]*node, nVals+1)
	for i := range nodes {
		ps, err := pubsub.NewGossipSub(ctx0, hosts[i])
		if err != nil {
			t.Fatal(err)
		}
		cn, err := consensus.NewNode(consensus.NodeConfig{
			Partition:  part,
			KeyPair:    keys[i],
			NumWorkers: 4,
			WorkerConfig: worker.Config{
				Partition:    part,
				BatchSize:    20,
				BatchTimeout: 200 * time.Millisecond,
			},
			MinRoundInterval:    50 * time.Millisecond,
			BatchCollectTimeout: 10 * time.Second,
		}, committee, hosts[i], ps)
		if err != nil {
			t.Fatal(err)
		}
		name := fmt.Sprintf("val%d", i)
		if i == nVals {
			name = "follower"
		}
		nodes[i] = &node{name: name, n: cn, committed: map[string]bool{}}
	}

	// Genesis is signed by the COMMITTEE's keys; every node, follower
	// included, starts from the same genesis DAG.
	for _, nd := range nodes {
		if err := nd.n.InsertGenesisForAll(keys[:nVals]); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithCancel(ctx0)
	defer cancel()
	var wg sync.WaitGroup
	for _, nd := range nodes {
		nd := nd
		wg.Add(1)
		go func() { defer wg.Done(); _ = nd.n.Start(ctx) }()
		wg.Add(1)
		go func() { defer wg.Done(); consume(ctx, nd) }()
	}
	defer func() {
		cancel()
		wg.Wait()
		for _, nd := range nodes {
			nd.n.Stop()
		}
	}()

	time.Sleep(2 * time.Second) // mesh

	// Load: half to a validator, half to the follower, same call the API's
	// SubmitterService makes (Node.SubmitUserTransaction).
	var sent, refusedVal, refusedFol int
	stop := time.After(runFor)
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
loop:
	for {
		select {
		case <-stop:
			break loop
		case <-tick.C:
			sent++
			if err := nodes[0].n.SubmitUserTransaction([]byte(fmt.Sprintf("%s%d", valPrefix, sent))); err != nil {
				refusedVal++
			}
			if err := nodes[nVals].n.SubmitUserTransaction([]byte(fmt.Sprintf("%s%d", folPrefix, sent))); err != nil {
				refusedFol++
			}
		}
	}
	time.Sleep(5 * time.Second) // drain

	var valSeen, folSeen int
	for _, nd := range nodes {
		nd.mu.Lock()
		v, f := 0, 0
		for tx := range nd.committed {
			if len(tx) > len(valPrefix) && tx[:len(valPrefix)] == valPrefix {
				v++
			}
			if len(tx) > len(folPrefix) && tx[:len(folPrefix)] == folPrefix {
				f++
			}
		}
		t.Logf("%-9s blocks=%d committed txs=%d  from-validator=%d  from-follower=%d",
			nd.name, nd.blocks, len(nd.committed), v, f)
		valSeen += v
		folSeen += f
		nd.mu.Unlock()
	}
	t.Logf("offered %d to the validator (%d refused), %d to the follower (%d refused)",
		sent, refusedVal, sent, refusedFol)

	if refusedFol != 0 {
		t.Errorf("the follower refused %d submissions; the claim is that it ACCEPTS", refusedFol)
	}
	if valSeen == 0 {
		t.Fatalf("the validator's transactions never committed either - the network did not run")
	}
	if folSeen != 0 {
		t.Errorf("the follower's transactions DID commit (%d) - refuted", folSeen)
	}
	// Stop one at a time, named, so the library's own "Primary stopped
	// ... headersCreated=N certificatesCreated=M" line is attributable.
	for _, nd := range nodes {
		t.Logf("=== stopping %s ===", nd.name)
		nd.n.Stop()
		time.Sleep(200 * time.Millisecond)
	}

	nodes[nVals].mu.Lock()
	fb := nodes[nVals].blocks
	nodes[nVals].mu.Unlock()
	if fb == 0 {
		t.Errorf("the follower executed no blocks at all; it must follow, or the test proves nothing")
	}
}

func consume(ctx context.Context, nd *node) {
	for {
		select {
		case <-ctx.Done():
			return
		case group, ok := <-nd.n.Committed():
			if !ok {
				return
			}
			for _, cert := range group {
				batches, err := nd.n.CollectBatches(ctx, cert)
				if err != nil {
					continue
				}
				nd.mu.Lock()
				for _, b := range batches {
					for _, tx := range b.Transactions {
						nd.committed[string(tx)] = true
					}
				}
				nd.blocks++
				nd.mu.Unlock()
				digests := make([]types.BatchDigest, 0, len(cert.Header.Payload))
				for _, e := range cert.Header.Payload {
					digests = append(digests, e.Digest)
				}
				for _, w := range nd.n.Workers() {
					w.PruneCommitted(digests, worker.CommitInfo{Detail: "repro", Cert: cert.Digest().String()})
				}
			}
			nd.n.ReportExecuted()
		}
	}
}
