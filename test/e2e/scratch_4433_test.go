package e2e

import (
	"sync"
	"bytes"
	"os"
	"regexp"
	"sort"
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func leaves(t *testing.T, db *database.Database) map[string][32]byte {
	m := map[string][32]byte{}
	b := db.Begin(false)
	defer b.Discard()
	require.NoError(t, b.ForEachAccount(func(a *database.Account, h [32]byte) error {
		m[a.Url().String()] = h
		return nil
	}))
	return m
}

func rawDump(t *testing.T, db *database.Database) map[string][]byte {
	st, err := db.Store()
	require.NoError(t, err)
	cs := st.Begin(nil, false)
	defer cs.Discard()
	m := map[string][]byte{}
	require.NoError(t, cs.ForEach(func(k *record.Key, v []byte) error {
		m[k.String()] = append([]byte(nil), v...)
		return nil
	}))
	return m
}

func keyShape(k string) string {
	return shapeRe.ReplaceAllString(k, "#")
}

var shapeRe = regexp.MustCompile(`[0-9a-f]{64}|\d+|acc://[^.]*\.acme[^\.]*|acc://[A-Za-z0-9-]+`)

func diffStores(t *testing.T, a, b *database.Database) {
	ma, mb := rawDump(t, a), rawDump(t, b)
	type c struct{ onlyA, onlyB, differ int; ex string }
	shapes := map[string]*c{}
	get := func(k string) *c {
		s := keyShape(k)
		if shapes[s] == nil {
			shapes[s] = &c{ex: k}
		}
		return shapes[s]
	}
	for k, v := range ma {
		w, ok := mb[k]
		switch {
		case !ok:
			x := get(k); x.onlyA++; x.ex = k
		case !bytes.Equal(v, w):
			x := get(k); x.differ++; x.ex = k
		}
	}
	for k := range mb {
		if _, ok := ma[k]; !ok {
			x := get(k); x.onlyB++; x.ex = k
		}
	}
	keys := make([]string, 0, len(shapes))
	for k := range shapes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		x := shapes[k]
		t.Logf("DIFF onlyJoiner=%d onlyPeer=%d differ=%d  %s   e.g. %s", x.onlyA, x.onlyB, x.differ, k, x.ex)
	}
}

func TestScratch4433(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	net := simulator.NewSimpleNetwork(t.Name(), 3, 2)
	sim := NewSim(t,
		simulator.WithNetwork(net),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
		simulator.BPTHistoryDepth(1024),
		// simulator.WithDatabase(simulator.BcdbDbOpener(t.TempDir(), func(err error) { require.NoError(t, err) })),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN1")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e16))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	if os.Getenv("STAGGER4433") != "" {
		// The DAG's shape: an anchor's copies reach the Directory over several
		// blocks, so an anchor sits below its quorum across block boundaries
		// and copies arrive after it executed.
		var mu sync.Mutex
		var nDelayed int
		delayed := map[uint64][][]*messaging.Envelope{}
		t.Cleanup(func() { t.Logf("delayed %d anchor envelopes", nDelayed) })
		sim.SetBlockHook(Directory, func(p execute.BlockParams, envs []*messaging.Envelope) ([]*messaging.Envelope, bool) {
			var now, later, later2 []*messaging.Envelope
			for _, env := range envs {
				isAnchor := false
				var h [32]byte
				for _, m := range env.Messages {
					if _, ok := m.(*messaging.BlockAnchor); ok {
						isAnchor = true
						h = m.Hash()
					}
				}
				if !isAnchor {
					now = append(now, env)
					continue
				}
				switch h[0] % 3 {
				case 0:
					now = append(now, env)
				case 1:
					later = append(later, env)
				default:
					later2 = append(later2, env)
				}
			}
			mu.Lock()
			defer mu.Unlock()
			if _, ok := delayed[p.Index]; !ok {
				delayed[p.Index] = [][]*messaging.Envelope{later, later2}
				nDelayed += len(later) + len(later2)
			}
			if d, ok := delayed[p.Index-1]; ok {
				now = append(now, d[0]...)
			}
			if d, ok := delayed[p.Index-2]; ok {
				now = append(now, d[1]...)
			}
			return now, true
		})
	}

	dnPart := sim.S.Partition(Directory)
	dn := DnUrl()

	var ts uint64
	submit := func() {
		ts++
		switch ts % 3 {
		case 0:
			sim.BuildAndSubmitTxnSuccessfully(
				build.Transaction().For(alice, "tokens").
					SendTokens(1, 0).To(bob, "tokens").
					SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		case 1:
			sim.BuildAndSubmitTxnSuccessfully(
				build.Transaction().For(alice, "tokens").
					BurnTokens(1, 0).
					SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		case 2:
			sim.BuildAndSubmitTxnSuccessfully(
				build.Transaction().For(alice, "tokens").
					AddCredits().Spend(1).To(alice, "book", "1").WithOracle(InitialAcmeOracle).
					SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		}
	}
	for i := 0; i < 5; i++ {
		submit()
		sim.StepN(2)
	}
	sim.StepN(10)

	const node = 0
	dnPart.RestartNode(node)
	for i := 0; i < 20; i++ {
		submit()
		sim.StepN(1)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if os.Getenv("SHAPE4433") == "" {
		dnPart.RestartNode(node)
	} else {
		// The soak's shape: the buffer holds every block since the restart,
		// and a pull takes the state past them, so the handoff stages groups
		// that are already in the state.
		for i := 0; i < 5; i++ {
			require.NoError(t, dnPart.NodeJoinState(node).Pull(ctx))
			submit()
			sim.StepN(1)
		}
		t.Logf("after the early pulls: joiner state %d, peer %d", partitionBlock(t, dnPart.NodeDatabase(node), dn), partitionBlock(t, dnPart.NodeDatabase(1), dn))
	}
	stepping := &steppingState{State: dnPart.NodeJoinState(node), step: func(round int) {
		if round%2 == 0 {
			submit()
		}
		sim.StepN(1)
		if round >= 300 {
			cancel()
		}
	}}
	settler, ok := dnPart.NodeExecutor(node).(join.Settler)
	require.True(t, ok)
	outcome, err := join.Run(ctx, join.Options{
		Partition: Directory,
		Buffer:    dnPart.NodeJoin(node),
		Stage:     &join.ExecutorStage{Settler: settler, Staging: dnPart.NodeStaging(node), Database: dnPart.NodeDatabase(node)},
		State:     stepping,
		Peers:     &join.APIPeers{Partition: Directory, Client: sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})
	require.NoError(t, err)
	require.Equal(t, join.Joined, outcome)
	joinedAt := partitionBlock(t, dnPart.NodeDatabase(node), dn)
	t.Logf("joined at %d, peer at %d", joinedAt, partitionBlock(t, dnPart.NodeDatabase(1), dn))
	for _, n := range []int{node, 1} {
		View(t, dnPart.NodeDatabase(n), func(batch *database.Batch) {
			for _, u := range []*url.URL{dn.JoinPath(Synthetic), AcmeUrl()} {
				chains, err := batch.Account(u).Chains().Get()
				require.NoError(t, err)
				for _, c := range chains {
					ch, err := batch.Account(u).ChainByName(c.Name)
					require.NoError(t, err)
					h, err := ch.Head().Get()
					require.NoError(t, err)
					t.Logf("node %d %v chain %s count %d", n, u, c.Name, h.Count)
				}
			}
		})
	}
	if os.Getenv("DIFF4433") != "" {
		diffStores(t, dnPart.NodeDatabase(node), dnPart.NodeDatabase(1))
	}

	prevLeaves := leaves(t, dnPart.NodeDatabase(1))
	bptOnly := map[string]int{}
	t.Cleanup(func() { t.Logf("leaves that moved in blocks with no block-ledger entry for them: %v", bptOnly) })
	for i := 0; i < 100; i++ {
		submit()
		sim.StepN(1)
		{
			cur := leaves(t, dnPart.NodeDatabase(1))
			blk := partitionBlock(t, dnPart.NodeDatabase(1), dn)
			named := map[string]bool{}
			View(t, dnPart.NodeDatabase(1), func(batch *database.Batch) {
				bl, err := batch.Account(dn.JoinPath(Ledger)).BlockLedger(blk).Get()
				if err == nil {
					for _, e := range bl.Entries {
						named[e.Account.String()] = true
					}
				}
			})
			for k, v := range cur {
				if prevLeaves[k] != v && !named[k] {
					bptOnly[k]++
				}
			}
			prevLeaves = cur
		}
		b0 := partitionBlock(t, dnPart.NodeDatabase(node), dn)
		b1 := partitionBlock(t, dnPart.NodeDatabase(1), dn)
		if b0 != b1 {
			continue
		}
		r0, r1 := bptRoot(t, dnPart.NodeDatabase(node)), bptRoot(t, dnPart.NodeDatabase(1))
		if r0 != r1 {
			l0, l1 := leaves(t, dnPart.NodeDatabase(node)), leaves(t, dnPart.NodeDatabase(1))
			for k, v := range l1 {
				if l0[k] != v {
					t.Logf("leaf differs: %s", k)
				}
			}
			for k := range l0 {
				if _, ok := l1[k]; !ok {
					t.Logf("only on joiner: %s", k)
				}
			}
			t.Fatalf("BPT diverged at block %d, %d blocks after joining at %d", b0, b0-joinedAt, joinedAt)
		}
	}
}
