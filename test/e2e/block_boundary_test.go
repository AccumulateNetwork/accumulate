// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

// Must a node reproduce its peers' BLOCK BOUNDARIES, or only arrive at the same
// STATE?
//
// The experiment: two runs of the same network execute the same three
// transactions in the same order. In the baseline, tx1 and tx2 execute in block
// K and tx3 in block K+1. In the shifted run, tx1 executes in block K and tx2
// and tx3 in block K+1. Nothing else differs: same keys, same genesis time,
// same funding, same transaction order, same block times (the simulator
// advances block time by exactly one second per block from genesis, see
// test/simulator/consensus/state_block.go:120).
//
// Two runs of this network are not bit-identical even with no shift: the anchor
// a partition signs carries a wall-clock timestamp
// (crosschain/anchoring.go:147 -> signing.Builder.SetTimestampToNow,
// pkg/client/signing/builder.go:186). TestBlockBoundariesControl measures that
// noise floor -- exactly which accounts move run to run when nothing is
// shifted -- so that TestBlockBoundariesShifted can say which differences the
// shift caused.

import (
	"crypto/sha256"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	execute "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// bbNoisy are the accounts whose state embeds the wall-clock anchor timestamp.
// TestBlockBoundariesControl proves this list is exactly right: every other
// account is bit-identical across two runs, and these two are not.
var bbNoisy = map[string]bool{
	"bvn-BVN0.acme/ledger":  true,
	"bvn-BVN0.acme/anchors": true,
}

type bbSnap struct {
	K           uint64            // the block at which the release schedule starts
	Schedule    map[uint64]string // block index -> which of the three executed there
	BPTRoot     string
	Digest      string // sha256 over every BPT leaf except the noisy ones
	LedgerIndex uint64
	RootHeight  int64
	RootAnchor  string
	Leaves      map[string]string // account -> BPT leaf hash
	Balance     map[string]string
	MainHeight  map[string]int64
	MainAnchor  map[string]string
	MainEntries map[string][]string // account -> main chain entries (transaction hashes)
	IndexChain  map[string][]string // account -> decoded main-index entries
	Chains      map[string]string   // "account:chain" -> height and anchor
	RootEntries []string            // every entry of the partition's root chain
}

// bbIsMine reports whether an envelope carries one of the three tagged sends.
// A built envelope carries Transaction+Signatures, not Messages, so both forms
// have to be checked.
func bbIsMine(e *messaging.Envelope) (int, bool) {
	tag := func(body TransactionBody) (int, bool) {
		st, ok := body.(*SendTokens)
		if !ok {
			return 0, false
		}
		// The three transactions are tagged by amount: 100, 101, 102.
		amt := st.To[0].Amount.Int64()
		if amt >= 100 && amt <= 102 {
			return int(amt - 100), true
		}
		return 0, false
	}
	for _, txn := range e.Transaction {
		if i, ok := tag(txn.Body); ok {
			return i, true
		}
	}
	for _, m := range e.Messages {
		tm, ok := m.(*messaging.TransactionMessage)
		if !ok {
			continue
		}
		if i, ok := tag(tm.Transaction.Body); ok {
			return i, true
		}
	}
	return 0, false
}

func bbRun(t *testing.T, shift bool, blocks int) *bbSnap {
	t.Helper()
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	sim := NewSim(t,
		simulator.Deterministic(),
		simulator.SimpleNetwork("BlockBoundary", 1, 1),
		simulator.Genesis(GenesisTime),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN0")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// K is the first block at which all three transactions are in hand. It is
	// the same absolute block index in both runs because everything up to that
	// point is identical; the snapshot records it so that can be checked rather
	// than assumed.
	var k uint64
	held := map[int]*messaging.Envelope{}
	released := 0
	schedule := map[uint64]string{}

	sim.SetBlockHook("BVN0", func(p execute.BlockParams, in []*messaging.Envelope) ([]*messaging.Envelope, bool) {
		var out []*messaging.Envelope
		for _, e := range in {
			if i, ok := bbIsMine(e); ok {
				held[i] = e
			} else {
				out = append(out, e)
			}
		}
		if k == 0 && len(held) == 3 {
			k = p.Index
		}

		var want int
		switch {
		case k == 0 || p.Index < k:
			want = 0
		case p.Index == k:
			if shift {
				want = 1
			} else {
				want = 2
			}
		default:
			want = 3
		}
		for released < want {
			out = append(out, held[released])
			if schedule[p.Index] != "" {
				schedule[p.Index] += ","
			}
			schedule[p.Index] += fmt.Sprintf("tx%d", released+1)
			released++
		}
		return out, true
	})

	for _, e := range envsOf(t, sim, alice, bob, aliceKey) {
		st, err := sim.SubmitTo("BVN0", e)
		require.NoError(t, err)
		for _, s := range st {
			require.False(t, s.Failed(), "submission failed: %v", s.Error)
		}
	}

	sim.StepN(blocks)
	require.Equal(t, 3, released, "all three transactions were released")

	snap := &bbSnap{
		K:           k,
		Schedule:    schedule,
		Leaves:      map[string]string{},
		Balance:     map[string]string{},
		MainHeight:  map[string]int64{},
		MainAnchor:  map[string]string{},
		MainEntries: map[string][]string{},
		IndexChain:  map[string][]string{},
		Chains:      map[string]string{},
	}
	View(t, sim.Database("BVN0"), func(batch *database.Batch) {
		ledger := batch.Account(PartitionUrl("BVN0").JoinPath(Ledger))

		root, err := batch.BPT().GetRootHash()
		require.NoError(t, err)
		snap.BPTRoot = fmt.Sprintf("%x", root)

		var sl *SystemLedger
		require.NoError(t, ledger.Main().GetAs(&sl))
		snap.LedgerIndex = sl.Index

		rc, err := ledger.RootChain().Get()
		require.NoError(t, err)
		snap.RootHeight = rc.Height()
		snap.RootAnchor = fmt.Sprintf("%x", rc.Anchor())
		for i := int64(0); i < rc.Height(); i++ {
			e, err := rc.Entry(i)
			require.NoError(t, err)
			snap.RootEntries = append(snap.RootEntries, fmt.Sprintf("%x", e))
		}

		require.NoError(t, batch.ForEachAccount(func(a *database.Account, hash [32]byte) error {
			snap.Leaves[a.Url().ShortString()] = fmt.Sprintf("%x", hash)
			return nil
		}))

		// A digest over every leaf but the two that carry the wall clock. This
		// is "the state", minus the harness's own irreproducibility.
		var names []string
		for n := range snap.Leaves {
			if !bbNoisy[n] {
				names = append(names, n)
			}
		}
		sort.Strings(names)
		h := sha256.New()
		for _, n := range names {
			fmt.Fprintf(h, "%s=%s\n", n, snap.Leaves[n])
		}
		snap.Digest = fmt.Sprintf("%x", h.Sum(nil))

		for _, u := range []*url.URL{alice.JoinPath("tokens"), bob.JoinPath("tokens"), alice.JoinPath("book", "1"), PartitionUrl("BVN0").JoinPath(Ledger)} {
			n := u.ShortString()
			a := batch.Account(u)

			// Every chain on the account, which is exactly what the BPT leaf
			// hashes (observer_prod.go:64, hashChains).
			metas, err := a.Chains().Get()
			require.NoError(t, err)
			for _, m := range metas {
				c, err := a.GetChainByName(m.Name)
				require.NoError(t, err)
				snap.Chains[n+":"+m.Name] = fmt.Sprintf("h=%d anchor=%x", c.Height(), c.Anchor())
			}
			if n != alice.JoinPath("tokens").ShortString() && n != bob.JoinPath("tokens").ShortString() {
				continue
			}

			var ta *TokenAccount
			require.NoError(t, a.Main().GetAs(&ta))
			snap.Balance[n] = ta.Balance.String()

			mc, err := a.MainChain().Get()
			require.NoError(t, err)
			snap.MainHeight[n] = mc.Height()
			snap.MainAnchor[n] = fmt.Sprintf("%x", mc.Anchor())
			for i := int64(0); i < mc.Height(); i++ {
				e, err := mc.Entry(i)
				require.NoError(t, err)
				snap.MainEntries[n] = append(snap.MainEntries[n], fmt.Sprintf("%x", e))
			}

			ic, err := a.MainChain().Index().Get()
			require.NoError(t, err)
			for i := int64(0); i < ic.Height(); i++ {
				data, err := ic.Entry(i)
				require.NoError(t, err)
				ie := new(IndexEntry)
				require.NoError(t, ie.UnmarshalBinary(data))
				snap.IndexChain[n] = append(snap.IndexChain[n],
					fmt.Sprintf("{block:%d source:%d anchor:%d}", ie.BlockIndex, ie.Source, ie.Anchor))
			}
		}
	})
	return snap
}

func envsOf(t *testing.T, _ *Sim, alice, bob *url.URL, aliceKey []byte) []*messaging.Envelope {
	t.Helper()
	envs := make([]*messaging.Envelope, 3)
	for i := range envs {
		envs[i] = MustBuild(t, build.Transaction().
			For(alice.JoinPath("tokens")).
			Body(&SendTokens{To: []*TokenRecipient{{Url: bob.JoinPath("tokens"), Amount: *big.NewInt(int64(100 + i))}}}).
			SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(uint64(i+1)).PrivateKey(aliceKey))
	}
	return envs
}

// bbDiffLeaves returns the accounts whose BPT leaf differs, sorted.
func bbDiffLeaves(a, b *bbSnap) []string {
	var out []string
	for n, v := range a.Leaves {
		if b.Leaves[n] != v {
			out = append(out, n)
		}
	}
	for n := range b.Leaves {
		if _, ok := a.Leaves[n]; !ok {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

func bbLog(t *testing.T, name string, s *bbSnap) {
	t.Helper()
	var blocks []uint64
	for b := range s.Schedule {
		blocks = append(blocks, b)
	}
	sort.Slice(blocks, func(i, j int) bool { return blocks[i] < blocks[j] })
	var sched []string
	for _, b := range blocks {
		sched = append(sched, fmt.Sprintf("block %d: %s", b, s.Schedule[b]))
	}
	t.Logf("%s: K=%d  %s", name, s.K, strings.Join(sched, " | "))
	t.Logf("%s: ledgerIndex=%d rootHeight=%d", name, s.LedgerIndex, s.RootHeight)
	t.Logf("%s: bptRoot=%s", name, s.BPTRoot)
	t.Logf("%s: digest (all leaves but %v) = %s", name, keysOf(bbNoisy), s.Digest)
	for _, n := range []string{"alice/tokens", "bob/tokens"} {
		t.Logf("%s: %-13s balance=%-14s mainHeight=%d mainAnchor=%s", name, n, s.Balance[n], s.MainHeight[n], s.MainAnchor[n])
		t.Logf("%s: %-13s main-entries=%v", name, n, s.MainEntries[n])
		t.Logf("%s: %-13s leaf=%s", name, n, s.Leaves[n])
		t.Logf("%s: %-13s main-index=%v", name, n, s.IndexChain[n])
	}
}

// bbFirstDiff returns the index of the first element that differs, or -1.
func bbFirstDiff(a, b []string) int {
	for i := range a {
		if i >= len(b) || a[i] != b[i] {
			return i
		}
	}
	if len(b) > len(a) {
		return len(a)
	}
	return -1
}

func keysOf(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestBlockBoundariesControl establishes the noise floor: two runs with
// identical block partitioning agree on every account except the two that carry
// the wall-clock anchor timestamp.
func TestBlockBoundariesControl(t *testing.T) {
	a := bbRun(t, false, 60)
	b := bbRun(t, false, 60)
	bbLog(t, "A ", a)
	bbLog(t, "A'", b)

	require.Equal(t, a.K, b.K)
	require.Equal(t, a.Schedule, b.Schedule)

	diff := bbDiffLeaves(a, b)
	t.Logf("control: accounts that differ run to run: %v", diff)
	t.Logf("control: first differing root chain entry: %d of %d", bbFirstDiff(a.RootEntries, b.RootEntries), len(a.RootEntries))
	var cd []string
	for n, v := range a.Chains {
		if b.Chains[n] != v {
			cd = append(cd, n)
		}
	}
	sort.Strings(cd)
	t.Logf("control: chains that differ run to run: %v", cd)
	for _, n := range diff {
		require.True(t, bbNoisy[n], "unexpected nondeterministic account %s", n)
	}

	// Everything else is bit-identical, including the digest and both token
	// accounts' chains.
	require.Equal(t, a.Digest, b.Digest, "state digest")
	require.Equal(t, a.Balance, b.Balance)
	require.Equal(t, a.MainHeight, b.MainHeight)
	require.Equal(t, a.MainAnchor, b.MainAnchor)
	require.Equal(t, a.MainEntries, b.MainEntries)
	require.Equal(t, a.IndexChain, b.IndexChain)
	require.Equal(t, a.LedgerIndex, b.LedgerIndex)
	require.Equal(t, a.RootHeight, b.RootHeight)
}

// TestBlockBoundariesShifted executes the same transactions in the same order
// with one of them moved a single block later.
func TestBlockBoundariesShifted(t *testing.T) {
	base := bbRun(t, false, 60)
	shifted := bbRun(t, true, 60)
	bbLog(t, "baseline", base)
	bbLog(t, "shifted ", shifted)

	require.Equal(t, base.K, shifted.K, "both runs start releasing at the same block")

	// What agrees.
	require.Equal(t, base.Balance, shifted.Balance, "balances")
	require.Equal(t, base.MainHeight, shifted.MainHeight, "main chain heights")
	require.Equal(t, base.LedgerIndex, shifted.LedgerIndex, "ledger index")

	// What does not.
	diff := bbDiffLeaves(base, shifted)
	t.Logf("shifted: accounts whose BPT leaf differs: %v", diff)
	var beyondNoise []string
	for _, n := range diff {
		if !bbNoisy[n] {
			beyondNoise = append(beyondNoise, n)
		}
	}
	t.Logf("shifted: accounts differing beyond the control's noise floor: %v", beyondNoise)

	// Which chain, exactly, moved?
	var chainsSame, chainsDiff []string
	for n, v := range base.Chains {
		if shifted.Chains[n] == v {
			chainsSame = append(chainsSame, n)
		} else {
			chainsDiff = append(chainsDiff, n)
			t.Logf("  chain %s\n    baseline %s\n    shifted  %s", n, v, shifted.Chains[n])
		}
	}
	sort.Strings(chainsSame)
	sort.Strings(chainsDiff)
	t.Logf("shifted: chains that agree:  %v", chainsSame)
	t.Logf("shifted: chains that differ: %v", chainsDiff)

	t.Logf("shifted: first differing root chain entry: %d of %d",
		bbFirstDiff(base.RootEntries, shifted.RootEntries), len(base.RootEntries))

	// Every leaf the BPT holds, so the list of accounts is on the record.
	var all []string
	for n := range base.Leaves {
		all = append(all, n)
	}
	sort.Strings(all)
	t.Logf("BPT holds %d accounts: %v", len(all), all)

	require.NotEqual(t, base.Digest, shifted.Digest,
		"the state digest must differ if block boundaries are baked into the BPT")
	require.Contains(t, beyondNoise, "alice/tokens")
	require.NotEqual(t, base.IndexChain["alice/tokens"], shifted.IndexChain["alice/tokens"],
		"the main-index chain is what carries the block number")
}

// TestBlockBoundariesNeverConverge runs the shifted network far past the shift
// and asks whether the difference washes out.
func TestBlockBoundariesNeverConverge(t *testing.T) {
	for _, n := range []int{60, 150, 300} {
		base := bbRun(t, false, n)
		shifted := bbRun(t, true, n)
		require.Equal(t, base.LedgerIndex, shifted.LedgerIndex, "same height at %d blocks", n)
		same := base.Leaves["alice/tokens"] == shifted.Leaves["alice/tokens"]
		t.Logf("after %3d blocks (height %d): alice/tokens leaf equal = %v  digest equal = %v",
			n, base.LedgerIndex, same, base.Digest == shifted.Digest)
		t.Logf("  baseline main-index = %v", base.IndexChain["alice/tokens"])
		t.Logf("  shifted  main-index = %v", shifted.IndexChain["alice/tokens"])
		require.False(t, same, "alice/tokens converged after %d blocks", n)
	}
}
