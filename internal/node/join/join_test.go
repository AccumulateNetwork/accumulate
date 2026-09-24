// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/p2p"
)

// fakeBuffer stands for the DAG service's collecting mode.
type fakeBuffer struct {
	collecting bool
	overrun    bool
	handedOff  uint64
	handoffs   []uint64 // every block the join handed off at, in order
	handoffErr error
	starts     int // how many times the join started collecting

	// staged is every block the join asked to stage through, in order;
	// stageErrs, if set, is what StageThrough answers each time, in turn,
	// before it answers nil.
	staged    []uint64
	stageErrs []error

	// log, if set, records the join's calls on the buffer and the stage in
	// the order they are made.
	log *[]string

	// restarts, if set, is whether StartCollecting starts a new buffer once
	// this one has overrun; otherwise the overrun stays.
	restarts bool
}

func (b *fakeBuffer) StartCollecting() {
	b.collecting = true
	b.starts++
	if b.restarts {
		b.overrun = false
	}
}
func (b *fakeBuffer) Collecting() bool    { return b.collecting }
func (b *fakeBuffer) BufferOverrun() bool { return b.overrun }
func (b *fakeBuffer) StageThrough(block uint64) error {
	record(b.log, "stage", block)
	b.staged = append(b.staged, block)
	if len(b.stageErrs) > 0 {
		err := b.stageErrs[0]
		b.stageErrs = b.stageErrs[1:]
		return err
	}
	return nil
}
func (b *fakeBuffer) Handoff(q uint64) error {
	record(b.log, "handoff", q)
	if b.handoffErr != nil {
		return b.handoffErr
	}
	b.handedOff = q
	b.handoffs = append(b.handoffs, q)
	b.collecting = false
	return nil
}

// fakeStage stands for the executor's staging half.
type fakeStage struct {
	settled uint64

	// loaded is never written: a Stage has no way to load a peer's staging
	// (#4362 removed it), so a test that says nothing was loaded holds by
	// construction.
	loaded any

	// gaps are the blocks whose collected streams are not contiguous from
	// Delivered + 1; gapAsked is every block HasGap was asked about.
	gaps     map[uint64][]StreamGap
	gapAsked []uint64

	log *[]string
}

func (s *fakeStage) SettleStagingAt(q uint64) error {
	record(s.log, "settle", q)
	s.settled = q
	return nil
}

func record(log *[]string, what string, n uint64) {
	if log != nil {
		*log = append(*log, fmt.Sprintf("%s %d", what, n))
	}
}

// HasGap is the seam granted for #4362 (Stage.HasGap): whether the streams
// collected for a block run contiguously from each stream's Delivered + 1.
func (s *fakeStage) HasGap(block uint64) ([]StreamGap, error) {
	record(s.log, "gap", block)
	s.gapAsked = append(s.gapAsked, block)
	return s.gaps[block], nil
}

// fakeState stands for the state pull and its tracker.
type fakeState struct {
	pulls     int
	matchAt   uint64
	matchFrom int // the pull round at which the root matches

	// collectingAtPull, if set, is read on the first pull: the join must
	// already be collecting by then.
	collectingAtPull Buffer
	wasCollecting    bool

	// overrunAtPull, if set, overruns on the first pull: the network ran
	// past the buffer while the join was converging.
	overrunAtPull *fakeBuffer

	// promoted and demoted are every block the join promoted and demoted
	// the node at.
	promoted []uint64
	demoted  []uint64
}

func (s *fakeState) Promote(block uint64) { s.promoted = append(s.promoted, block) }
func (s *fakeState) Demote(block uint64)  { s.demoted = append(s.demoted, block) }

func (s *fakeState) Pull(context.Context) error {
	if s.pulls == 0 && s.collectingAtPull != nil {
		s.wasCollecting = s.collectingAtPull.Collecting()
	}
	if s.pulls == 0 && s.overrunAtPull != nil {
		s.overrunAtPull.overrun = true
	}
	s.pulls++
	return nil
}

// Ready: this fake's state is never executed from before it matches.
func (s *fakeState) Ready() (uint64, bool) { return 0, false }

func (s *fakeState) Matched(context.Context) (uint64, bool, error) {
	if s.pulls < s.matchFrom {
		return 0, false, nil
	}
	return s.matchAt, true, nil
}

// fakePeers lists the partition's validators.
type fakePeers struct {
	peers []*api.FindServiceResult

	// asked is never written: Peers has no way to ask a peer for its
	// staging (#4362 removed it), so a test that says no peer was asked
	// holds by construction.
	asked []string
}

func (p *fakePeers) Validators(context.Context) ([]*api.FindServiceResult, error) {
	return p.peers, nil
}

// peerID is a distinct, valid peer ID per test peer: an ed25519 public key's
// identity multihash, which is what a real FindService result carries.
func peerID(n byte) p2p.PeerID {
	_, pub, err := crypto.GenerateEd25519Key(bytes.NewReader(bytes.Repeat([]byte{n}, 64)))
	if err != nil {
		panic(err)
	}
	id, err := peer.IDFromPublicKey(pub)
	if err != nil {
		panic(err)
	}
	return id
}

func peerResult(n byte) *api.FindServiceResult {
	r := new(api.FindServiceResult)
	r.PeerID = peerID(n)
	return r
}

func run(t *testing.T, opts Options) (Outcome, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	opts.Retry = time.Millisecond
	return Run(ctx, opts)
}

// The join syncs until the root matches an anchored block, asks whether the
// block after it has a gap, and — none — settles staging there and hands off
// (executor spec, "Sync", step 4). Staging is what the node collected itself:
// no peer is asked for its own.
func TestJoin_HandsOffWhereTheNextBlockHasNoGap(t *testing.T) {
	buf := new(fakeBuffer)
	stage := new(fakeStage)
	state := &fakeState{matchAt: 20, matchFrom: 2}
	peers := &fakePeers{peers: []*api.FindServiceResult{peerResult(1)}}

	outcome, err := run(t, Options{Partition: "BVN1", Buffer: buf, Stage: stage, State: state, Peers: peers})
	require.NoError(t, err)
	require.Equal(t, Joined, outcome)

	require.Equal(t, []uint64{21}, buf.staged, "the node stages what it collected through the block after the state")
	require.Equal(t, []uint64{21}, stage.gapAsked, "the block after the match is checked for a gap")
	require.Equal(t, uint64(20), stage.settled, "staging settles at the block the state is")
	require.Equal(t, []uint64{20}, buf.handoffs, "and the handoff is at that block")
	require.False(t, buf.collecting, "a node that has joined is not collecting")
	require.Equal(t, 2, state.pulls,
		"the join pulls each round; what it pulls is the pull's own business (#4306)")
}

// If the buffer overran, a committed block is missing from it and none of
// the blocks after the state may be produced. While the buffer stays overrun
// the join does not hand off and does not settle; the node stays collecting.
func TestJoin_ABufferOverrunIsNotAHandoff(t *testing.T) {
	buf := &fakeBuffer{overrun: true}
	stage := new(fakeStage)
	state := &fakeState{matchAt: 20, matchFrom: 0}
	peers := &fakePeers{peers: []*api.FindServiceResult{peerResult(1)}}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := Run(ctx, Options{Partition: "BVN1", Buffer: buf, Stage: stage, State: state, Peers: peers, Retry: time.Millisecond})
	require.Error(t, err, "the join does not end at the overrun; only the context ends it")
	require.False(t, errors.Is(err, errors.NotReady), "got %v", err)
	require.Greater(t, buf.starts, 1, "the join asks the buffer to collect again")
	require.Empty(t, buf.handoffs)
	require.Zero(t, stage.settled)
	require.True(t, buf.collecting, "and the node is still collecting")
}

// The daemon runs the join once, so an overrun must not end it. The join
// starts collecting again and hands off from the new buffer.
func TestJoin_StartsAgainAfterABufferOverrun(t *testing.T) {
	buf := &fakeBuffer{restarts: true}
	stage := new(fakeStage)
	state := &fakeState{matchAt: 20, matchFrom: 1, overrunAtPull: buf}
	peers := &fakePeers{peers: []*api.FindServiceResult{peerResult(1)}}

	outcome, err := run(t, Options{Partition: "BVN1", Buffer: buf, Stage: stage, State: state, Peers: peers})
	require.NoError(t, err)
	require.Equal(t, Joined, outcome)
	require.Equal(t, 2, buf.starts, "the join started collecting again after the overrun")
	require.Equal(t, []uint64{21}, buf.staged, "and stages only from the new buffer")
	require.Equal(t, []uint64{20}, buf.handoffs)
}

// Collecting starts before anything is pulled. That ordering is what makes
// the join exact: every block after the state the pull reaches is one this
// node has collected, so a gap can only be an entry from before it listened.
func TestJoin_CollectsBeforeItPulls(t *testing.T) {
	buf := new(fakeBuffer)
	stage := new(fakeStage)
	state := &fakeState{matchAt: 20, matchFrom: 1}
	state.collectingAtPull = buf
	peers := &fakePeers{peers: []*api.FindServiceResult{peerResult(1)}}

	_, err := run(t, Options{Partition: "BVN1", Buffer: buf, Stage: stage, State: state, Peers: peers})
	require.NoError(t, err)
	require.True(t, state.wasCollecting, "the node was collecting before it pulled")
}

// Finding no validator at all is not an answer about staging. A node that
// cannot see its partition cannot know what its peers hold, so it keeps
// collecting and does not execute. Before this, an empty peer list was
// indistinguishable from "every validator has nothing to give", and the join
// executed from its own empty staging -- the divergence of #4290, seen on a
// live network in run 20260918T124530Z, where the lookup named no network
// and so matched a key nobody advertises (#4296).
//
// There is no exception, and there used to appear to be one:
// TestJoin_AFreshNodeStartsWithoutAsking stood here, passed Options.Fresh by
// hand and asserted that such a node executes anyway. No caller could set
// that flag -- the daemon computed it inside a branch where it was false by
// construction -- so it proved the library and said nothing about any node
// (#4304). The node it described does not join at all now, so everything
// that reaches Run has executed a block and this rule is unconditional.
func TestJoin_FindingNoValidatorIsNotAnAnswer(t *testing.T) {
	buf := new(fakeBuffer)
	stage := new(fakeStage)
	state := &fakeState{matchAt: 20, matchFrom: 0}
	peers := &fakePeers{} // nobody found

	_, err := run(t, Options{Partition: "BVN1", Buffer: buf, Stage: stage, State: state, Peers: peers, Rounds: 2})
	require.Error(t, err, "a node that cannot see its partition does not execute")
	require.True(t, errors.Is(err, errors.NotReady), "and says it is not ready")
	require.Zero(t, buf.handedOff)
}

// gapState is a state pull that syncs to b on its first pull, and after that
// advances the sync by one block only once the stage has reported a gap at
// the block after it: the pull of the accounts that block's ledger names, at
// that block's anchor (executor spec, "Sync", step 3).
type gapState struct {
	stage  *fakeStage
	b      uint64
	synced uint64 // zero until the first pull
	pulls  int
}

func (s *gapState) Pull(context.Context) error {
	s.pulls++
	switch {
	case s.synced == 0:
		s.synced = s.b
	case len(s.stage.gaps[s.synced+1]) > 0 && contains(s.stage.gapAsked, s.synced+1):
		s.synced++
	}
	return nil
}

func (s *gapState) Promote(uint64) {}
func (s *gapState) Demote(uint64)  {}

// Ready: this fake's state is never executed from before it matches.
func (s *gapState) Ready() (uint64, bool) { return 0, false }

func (s *gapState) Matched(context.Context) (uint64, bool, error) {
	return s.synced, s.synced > 0, nil
}

func contains(s []uint64, v uint64) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// A peer holds an entry from before this node was listening: B+1 carries a
// stream's sequence 105 while the state synced at B says Delivered is 103,
// and 104 is not in B+1. The gap does not hold the handoff: the node executes
// B+1 with whatever staging holds, and a block executed without an entry it
// needed is wrong only in the accounts its record names, which the root check
// repairs from the block ledger (executor spec, "Sync", "One rule for every
// node"). Before, the join held B+1 back and advanced the sync to a block with
// no gap (#4362).
func TestJoin_AGapDoesNotHoldTheHandoff(t *testing.T) {
	const b = 20
	buf := new(fakeBuffer)
	stage := &fakeStage{gaps: map[uint64][]StreamGap{b + 1: {{Delivered: 103, Missing: 104, MissingTo: 104, Held: 105}}}}
	state := &gapState{stage: stage, b: b}
	peers := &fakePeers{
		peers: []*api.FindServiceResult{peerResult(1)},
	}

	outcome, err := run(t, Options{Partition: "BVN1", Buffer: buf, Stage: stage, State: state, Peers: peers})
	require.NoError(t, err)
	require.Equal(t, Joined, outcome)

	assert.Equal(t, []uint64{b}, buf.handoffs, "the join hands off at B although B+1 has a gap")
	assert.Equal(t, uint64(b), stage.settled, "staging settles at the block the state is")
	assert.Equal(t, []uint64{b + 1}, stage.gapAsked, "the join asked about B+1")
}

// Staging at the handoff holds everything collected through Q + 1 and nothing
// after it (executor spec, "Sync", step 5; #4398). The join stages through
// Q + 1 once the state matches at Q and before it asks whether Q + 1 has a
// gap — the gap check reads what Q + 1 carries — then settles at Q and hands
// off. Before #4398 every collected group was staged as it arrived, and a
// joining node executed Q + 1 holding what Q + 2 … brought (run
// 20260924T052134Z).
func TestJoin_StagesThroughTheBlockAfterTheStateBeforeTheGapCheck(t *testing.T) {
	var log []string
	buf := &fakeBuffer{log: &log}
	stage := &fakeStage{log: &log}
	state := &fakeState{matchAt: 20, matchFrom: 1}
	peers := &fakePeers{peers: []*api.FindServiceResult{peerResult(1)}}

	_, err := run(t, Options{Partition: "BVN1", Buffer: buf, Stage: stage, State: state, Peers: peers})
	require.NoError(t, err)
	require.Equal(t, []string{"stage 21", "gap 21", "settle 20", "handoff 20"}, log)
}

// Q + 1 not collected yet is a wait, not a gap check without it: the join
// asks nothing of the stage and hands nothing off until the block after the
// state is staged.
func TestJoin_WaitsForTheBlockAfterTheStateBeforeItAsksAboutAGap(t *testing.T) {
	var log []string
	buf := &fakeBuffer{log: &log, stageErrs: []error{
		errors.NotReady.With("block 21 has not been collected"),
		errors.Conflict.With("the state is behind"),
	}}
	stage := &fakeStage{log: &log}
	state := &fakeState{matchAt: 20, matchFrom: 1}
	peers := &fakePeers{peers: []*api.FindServiceResult{peerResult(1)}}

	_, err := run(t, Options{Partition: "BVN1", Buffer: buf, Stage: stage, State: state, Peers: peers})
	require.NoError(t, err)
	require.Equal(t, []string{"stage 21", "stage 21", "stage 21", "gap 21", "settle 20", "handoff 20"}, log)
}
