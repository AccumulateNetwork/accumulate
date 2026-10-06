// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// seedingBuffer is a fakeBuffer whose consensus waits for a seed.
type seedingBuffer struct {
	fakeBuffer
	awaiting bool
	seeded   []uint64
}

func (b *seedingBuffer) AwaitingSeed() bool { return b.awaiting }
func (b *seedingBuffer) SeedAt(round uint64) bool {
	record(b.log, "seed", round)
	b.seeded = append(b.seeded, round)
	b.awaiting = false
	return true
}

// provingState is a fakeState that proves a round; errs, if set, is what it
// answers each time, in turn, before it proves.
type provingState struct {
	fakeState
	block, round uint64
	errs         []error
	asked        int
	log          *[]string
}

func (s *provingState) ProvenRound(context.Context) (uint64, uint64, error) {
	s.asked++
	if len(s.errs) > 0 {
		err := s.errs[0]
		s.errs = s.errs[1:]
		return 0, 0, err
	}
	return s.block, s.round, nil
}

func (s *provingState) Pull(ctx context.Context) error {
	record(s.log, "pull", uint64(s.pulls))
	return s.fakeState.Pull(ctx)
}

// A follower's consensus is seeded before the first pull, at the round the
// state proves with nothing pulled (#4530). The groups committed while the
// node pulls are then collected, and the state the pull ends at is above the
// rescue window: seeded at that state instead, the first handoff was always
// refused.
func TestJoin_SeedsConsensusBeforeItPulls(t *testing.T) {
	var log []string
	buf := &seedingBuffer{awaiting: true}
	buf.log = &log
	state := &provingState{fakeState: fakeState{matchAt: 20, matchFrom: 1}, block: 7, round: 14, log: &log}
	peers := &fakePeers{peers: []*api.FindServiceResult{peerResult(1)}}

	_, err := run(t, Options{Partition: "BVN1", Buffer: buf, Stage: new(fakeStage), State: state, Peers: peers})
	require.NoError(t, err)
	require.Equal(t, []uint64{14}, buf.seeded)
	require.Equal(t, []string{"seed 14", "pull 0", "stage 21", "handoff 20"}, log)
	require.Equal(t, 1, state.asked, "a seeded node is not asked again")
}

// A node that is not waiting for a seed -- one that restored its checkpoint,
// or a validator, which is seeded at its first proven state -- proves no
// round and is given none.
func TestJoin_ANodeNotAwaitingASeedIsGivenNone(t *testing.T) {
	buf := &seedingBuffer{awaiting: false}
	state := &provingState{fakeState: fakeState{matchAt: 20, matchFrom: 1}, block: 7, round: 14}
	peers := &fakePeers{peers: []*api.FindServiceResult{peerResult(1)}}

	_, err := run(t, Options{Partition: "BVN1", Buffer: buf, Stage: new(fakeStage), State: state, Peers: peers})
	require.NoError(t, err)
	require.Empty(t, buf.seeded)
	require.Zero(t, state.asked)
}

// Nothing proven seeds nothing and stops nothing: the join pulls, asks again
// before its next round, and hands off whether or not it was ever seeded
// early.
func TestJoin_AnUnprovenRoundIsAskedAgainAndDoesNotHoldTheJoin(t *testing.T) {
	var log []string
	buf := &seedingBuffer{awaiting: true}
	buf.log = &log
	state := &provingState{fakeState: fakeState{matchAt: 20, matchFrom: 2}, block: 7, round: 14, log: &log,
		errs: []error{errors.NotReady.With("no signed anchor yet")}}
	peers := &fakePeers{peers: []*api.FindServiceResult{peerResult(1)}}

	_, err := run(t, Options{Partition: "BVN1", Buffer: buf, Stage: new(fakeStage), State: state, Peers: peers})
	require.NoError(t, err)
	require.Equal(t, []string{"pull 0", "seed 14", "pull 1", "stage 21", "handoff 20"}, log)
}

// steppingState is a provingState whose pull round asks again for the round
// between its steps, as PulledState's does.
type steppingState struct {
	provingState
	try func(context.Context)
}

func (s *steppingState) SeedBetweenSteps(try func(context.Context)) { s.try = try }

func (s *steppingState) Pull(ctx context.Context) error {
	err := s.provingState.Pull(ctx)
	if s.try != nil {
		s.try(ctx)
	}
	record(s.log, "pulled", uint64(s.pulls-1))
	return err
}

// A node that could prove no round when its join started is seeded inside its
// first pull round, as soon as a round is proven, and not a round later. In
// Docker run 20261006T060807Z a follower's first anchor read met two of four
// validators; it was seeded 110 s later, two seconds before its state was
// proven, and waited another 70 s round for consensus to reach that state.
func TestJoin_ANodeNotSeededAtTheStartIsSeededInsideItsFirstPullRound(t *testing.T) {
	var log []string
	buf := &seedingBuffer{awaiting: true}
	buf.log = &log
	state := &steppingState{provingState: provingState{fakeState: fakeState{matchAt: 20, matchFrom: 1}, block: 7, round: 14, log: &log,
		errs: []error{errors.NotReady.With("no signed anchor yet")}}}
	peers := &fakePeers{peers: []*api.FindServiceResult{peerResult(1)}}

	_, err := run(t, Options{Partition: "BVN1", Buffer: buf, Stage: new(fakeStage), State: state, Peers: peers})
	require.NoError(t, err)
	require.Equal(t, []string{"pull 0", "seed 14", "pulled 0", "stage 21", "handoff 20"}, log,
		"the node was seeded only after its first pull round ended")
}

// ledgerAnswer is a peer's answer for the system ledger as of block: the
// body, and a receipt from its hash to a root.
func ledgerAnswer(t *testing.T, u *url.URL, block, round uint64) (*api.AccountRecord, [32]byte) {
	t.Helper()
	ledger := &protocol.SystemLedger{Url: u, Index: block, LeaderRound: round}
	body, err := ledger.MarshalBinary()
	require.NoError(t, err)
	start := sha256.Sum256(body)
	sibling := sha256.Sum256([]byte("the rest of the tree"))
	root := sha256.Sum256(append(start[:], sibling[:]...))

	rec := &api.AccountRecord{Account: ledger, Receipt: new(api.Receipt)}
	rec.Receipt.Start = start[:]
	rec.Receipt.Anchor = root[:]
	rec.Receipt.Entries = []*merkle.ReceiptEntry{{Right: true, Hash: sibling[:]}}
	rec.Receipt.ForHeight = block
	rec.Receipt.StartsAtMainState = true
	rec.Receipt.LocalBlock = block + 2
	require.True(t, rec.Receipt.Validate(nil), "precondition: the receipt checks")
	return rec, root
}

// The round consensus is seeded at is a peer's answer, so every part of it is
// checked against the root the signed anchor carries. An answer that passes
// with any part wrong lets one peer name the round a follower orders from.
func TestProvenLedger_TakesOnlyAnAnswerTheSignedAnchorProves(t *testing.T) {
	u := protocol.PartitionUrl("BVN1").JoinPath(protocol.Ledger)

	rec, root := ledgerAnswer(t, u, 50, 100)
	ledger, err := provenLedger(rec, u, 50, root)
	require.NoError(t, err)
	require.Equal(t, uint64(100), ledger.LeaderRound)

	cases := map[string]func(rec *api.AccountRecord, root *[32]byte){
		"another round in the body": func(rec *api.AccountRecord, _ *[32]byte) {
			rec.Account.(*protocol.SystemLedger).LeaderRound = 9000
		},
		"a root the anchor does not carry": func(_ *api.AccountRecord, root *[32]byte) { root[0] ^= 1 },
		"a receipt that does not check":    func(rec *api.AccountRecord, _ *[32]byte) { rec.Receipt.Entries[0].Hash[0] ^= 1 },
		"a receipt for another block":      func(rec *api.AccountRecord, _ *[32]byte) { rec.Receipt.ForHeight = 49 },
		"the ledger of another block": func(rec *api.AccountRecord, _ *[32]byte) {
			// Consistently: the body, and a receipt that starts at it and
			// checks, of a block that is not the anchor's.
			other, _ := ledgerAnswer(t, u, 49, 98)
			other.Receipt.ForHeight = 50
			*rec = *other
		},
		"a receipt not from the body":  func(rec *api.AccountRecord, _ *[32]byte) { rec.Receipt.StartsAtMainState = false },
		"no receipt":                   func(rec *api.AccountRecord, _ *[32]byte) { rec.Receipt = nil },
		"another account's body":       func(rec *api.AccountRecord, _ *[32]byte) { rec.Account = &protocol.TokenAccount{Url: u} },
		"the ledger under another url": func(rec *api.AccountRecord, _ *[32]byte) { rec.Account.(*protocol.SystemLedger).Url = u.JoinPath("x") },
	}
	for name, bend := range cases {
		t.Run(name, func(t *testing.T) {
			rec, root := ledgerAnswer(t, u, 50, 100)
			bend(rec, &root)
			_, err := provenLedger(rec, u, 50, root)
			require.Error(t, err)
			require.ErrorIs(t, err, errors.Conflict)
		})
	}
}

// An anchored block far behind the peer that answers is a collector held at
// an anchor no quorum signed. Its round is proven and is still not one to
// order from: the certificates and batches above it may be gone.
func TestProvenLedger_AnAnchorFarBehindThePeerSeedsNothing(t *testing.T) {
	u := protocol.PartitionUrl("BVN1").JoinPath(protocol.Ledger)
	rec, root := ledgerAnswer(t, u, 50, 100)

	rec.Receipt.LocalBlock = 50 + SeedWithin
	_, err := provenLedger(rec, u, 50, root)
	require.NoError(t, err)

	rec.Receipt.LocalBlock = 50 + SeedWithin + 1
	_, err = provenLedger(rec, u, 50, root)
	require.ErrorIs(t, err, errors.NotReady)
}
