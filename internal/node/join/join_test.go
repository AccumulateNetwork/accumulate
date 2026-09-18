// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// fakeBuffer stands for the DAG service's collecting mode.
type fakeBuffer struct {
	collecting bool
	overrun    bool
	accounts   []*url.URL
	handedOff  uint64
	handoffErr error
	applied    int // how many blocks were applied to the staging that was taken
	starts     int // how many times the join started collecting

	// clearOverrunAfter is the number of StartCollecting calls after which
	// the buffer stops reporting an overrun.
	clearOverrunAfter int
}

func (b *fakeBuffer) StartCollecting() {
	b.collecting = true
	b.starts++
	if b.clearOverrunAfter > 0 && b.starts >= b.clearOverrunAfter {
		b.overrun = false
	}
}
func (b *fakeBuffer) Collecting() bool    { return b.collecting }
func (b *fakeBuffer) BufferOverrun() bool { return b.overrun }
func (b *fakeBuffer) NamedAccounts() []*url.URL {
	out := b.accounts
	b.accounts = nil
	return out
}
func (b *fakeBuffer) ApplyStaging(load func() error) error {
	err := load()
	if err != nil {
		return err
	}
	b.applied++
	return nil
}
func (b *fakeBuffer) Handoff(q uint64) error {
	if b.handoffErr != nil {
		return b.handoffErr
	}
	b.handedOff = q
	b.collecting = false
	return nil
}

// fakeStage stands for the executor's staging half.
type fakeStage struct {
	loaded  *private.StagingSnapshot
	settled uint64
	loadErr error
}

func (s *fakeStage) LoadStaging(snap *private.StagingSnapshot) error {
	if s.loadErr != nil {
		return s.loadErr
	}
	s.loaded = snap
	return nil
}
func (s *fakeStage) SettleStagingAt(q uint64) error { s.settled = q; return nil }

// fakeState stands for the state pull and its tracker.
type fakeState struct {
	pulls     [][]*url.URL
	matchAt   uint64
	matchFrom int // the pull round at which the root matches
}

func (s *fakeState) Pull(_ context.Context, accounts []*url.URL) error {
	s.pulls = append(s.pulls, accounts)
	return nil
}
func (s *fakeState) Matched(context.Context) (uint64, bool, error) {
	if len(s.pulls) < s.matchFrom {
		return 0, false, nil
	}
	return s.matchAt, true, nil
}

// fakePeers serves a snapshot per peer, in order.
type fakePeers struct {
	peers []*api.FindServiceResult
	snaps map[string]*private.StagingSnapshot
	errs  map[string]error
	asked []string

	// collectingWhenAsked, if set, is read the first time a peer is asked for
	// its staging: the join must already be collecting by then.
	collectingWhenAsked Buffer
	wasCollecting       bool
}

func (p *fakePeers) Validators(context.Context) ([]*api.FindServiceResult, error) {
	return p.peers, nil
}

func (p *fakePeers) Staging(peer *api.FindServiceResult) private.StagingSnapshotter {
	return &fakeSnapshotter{p: p, key: peer.PeerID.String()}
}

type fakeSnapshotter struct {
	p   *fakePeers
	key string
}

func (f *fakeSnapshotter) Sequence(context.Context, *url.URL, *url.URL, uint64, private.SequenceOptions) (*api.MessageRecord[messaging.Message], error) {
	return nil, errors.NotAllowed.With("not used")
}

func (f *fakeSnapshotter) StagingSnapshot(_ context.Context, _ *private.StagingSnapshotRequest) (*private.StagingSnapshot, error) {
	f.p.asked = append(f.p.asked, f.key)
	if f.p.collectingWhenAsked != nil && len(f.p.asked) == 1 {
		f.p.wasCollecting = f.p.collectingWhenAsked.Collecting()
	}
	if err := f.p.errs[f.key]; err != nil {
		return nil, err
	}
	return f.p.snaps[f.key], nil
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

// The join takes a peer's staging, pulls until the root matches an anchored
// block at or above the snapshot's, settles there and hands off (executor
// spec, "Sync").
func TestJoin_TakesStagingPullsThenHandsOff(t *testing.T) {
	buf := &fakeBuffer{accounts: []*url.URL{protocol.AccountUrl("alice")}}
	stage := new(fakeStage)
	state := &fakeState{matchAt: 20, matchFrom: 2}
	peers := &fakePeers{
		peers: []*api.FindServiceResult{peerResult(1)},
		snaps: map[string]*private.StagingSnapshot{peerID(1).String(): {Block: 17}},
	}

	outcome, err := run(t, Options{Partition: "BVN1", Buffer: buf, Stage: stage, State: state, Peers: peers})
	require.NoError(t, err)
	require.Equal(t, Joined, outcome)

	require.NotNil(t, stage.loaded, "staging was taken from the peer")
	require.Equal(t, 1, buf.applied, "and the blocks buffered since were applied to it")
	require.Equal(t, uint64(17), stage.loaded.Block)
	require.Equal(t, uint64(20), stage.settled, "staging settles at the block the state is")
	require.Equal(t, uint64(20), buf.handedOff, "and the handoff is at that block")
	require.False(t, buf.collecting, "a node that has joined is not collecting")
	require.Equal(t, [][]*url.URL{{protocol.AccountUrl("alice")}, nil}, state.pulls,
		"the first pull is what the collected blocks named")
}

// A validator that cannot serve its staging — because it is joining itself,
// or is too busy to finish a paged read — is passed over, not waited on. The
// join asks the next one (#4295's refusal is what makes that answerable).
func TestJoin_AsksTheNextValidator(t *testing.T) {
	buf := new(fakeBuffer)
	stage := new(fakeStage)
	state := &fakeState{matchAt: 9, matchFrom: 1}
	peers := &fakePeers{
		peers: []*api.FindServiceResult{peerResult(1), peerResult(2), peerResult(3)},
		errs:  map[string]error{peerID(1).String(): errors.NotReady.With("booting")},
		snaps: map[string]*private.StagingSnapshot{
			peerID(2).String(): {Block: 0}, // has executed no block
			peerID(3).String(): {Block: 8},
		},
	}

	outcome, err := run(t, Options{Partition: "BVN1", Buffer: buf, Stage: stage, State: state, Peers: peers})
	require.NoError(t, err)
	require.Equal(t, Joined, outcome)

	require.Equal(t, []string{peerID(1).String(), peerID(2).String(), peerID(3).String()}, peers.asked)
	require.Equal(t, uint64(8), stage.loaded.Block, "the third validator's staging is the one taken")
}

// A root that matches a block BELOW the staging taken is not a handoff: the
// node would be executing from staging as of P against state as of Q < P,
// which is a pairing no node ever held (#4290).
func TestJoin_WillNotHandOffBelowTheStagingItTook(t *testing.T) {
	buf := new(fakeBuffer)
	stage := new(fakeStage)
	state := &fakeState{matchAt: 5, matchFrom: 1} // always matches, at 5
	peers := &fakePeers{
		peers: []*api.FindServiceResult{peerResult(1)},
		snaps: map[string]*private.StagingSnapshot{peerID(1).String(): {Block: 17}},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := Run(ctx, Options{Partition: "BVN1", Buffer: buf, Stage: stage, State: state, Peers: peers, Retry: time.Millisecond})
	require.Error(t, err, "the join does not hand off; it runs until the context ends")
	require.Zero(t, buf.handedOff)
	require.Zero(t, stage.settled)
	require.NotEmpty(t, state.pulls, "and it keeps pulling")
}

// If the buffer overran, the blocks since the snapshot are no longer all in
// hand and the join cannot be exact: it starts again from a newer snapshot
// rather than handing off or giving up. A node that gave up here would keep
// up with consensus and execute nothing, for ever.
func TestJoin_StartsAgainAfterABufferOverrun(t *testing.T) {
	buf := &fakeBuffer{overrun: true}
	stage := new(fakeStage)
	state := &fakeState{matchAt: 20, matchFrom: 0}
	peers := &fakePeers{
		peers: []*api.FindServiceResult{peerResult(1)},
		snaps: map[string]*private.StagingSnapshot{peerID(1).String(): {Block: 17}},
	}

	// The overrun clears the second time staging is taken, so the join gets
	// through on its next attempt.
	buf.clearOverrunAfter = 2

	outcome, err := run(t, Options{Partition: "BVN1", Buffer: buf, Stage: stage, State: state, Peers: peers})
	require.NoError(t, err)
	require.Equal(t, Joined, outcome)
	require.GreaterOrEqual(t, buf.starts, 2, "the join started over")
	require.Equal(t, uint64(20), buf.handedOff)
}

// Every validator asked, none able to serve: that is an answer, not a
// failure — a network that restarted as a whole holds no staging anywhere —
// and it must be told apart from every other NotReady the join meets, or a
// routine "not yet" would pair a peer's staging with this node's older state.
func TestJoin_NoPeerHasStagingIsAnAnswer(t *testing.T) {
	buf := new(fakeBuffer)
	stage := new(fakeStage)
	state := &fakeState{matchAt: 20, matchFrom: 0}
	peers := &fakePeers{
		peers: []*api.FindServiceResult{peerResult(1)},
		errs:  map[string]error{peerID(1).String(): errors.NotReady.With("booting")},
	}

	outcome, err := run(t, Options{Partition: "BVN1", Buffer: buf, Stage: stage, State: state, Peers: peers, Rounds: 2})
	require.NoError(t, err, "not a failure")
	require.Equal(t, NoPeerHasStaging, outcome)
	require.Zero(t, buf.handedOff, "and nothing was handed off")
	require.Nil(t, stage.loaded)
}

// Collecting starts before anything is asked of a peer. That ordering is what
// makes the join exact: every block above the snapshot's is one this node has
// collected.
func TestJoin_CollectsBeforeItAsks(t *testing.T) {
	buf := new(fakeBuffer)
	stage := new(fakeStage)
	state := &fakeState{matchAt: 20, matchFrom: 0}
	peers := &fakePeers{
		peers: []*api.FindServiceResult{peerResult(1)},
		snaps: map[string]*private.StagingSnapshot{peerID(1).String(): {Block: 17}},
	}
	peers.collectingWhenAsked = buf

	_, err := run(t, Options{Partition: "BVN1", Buffer: buf, Stage: stage, State: state, Peers: peers})
	require.NoError(t, err)
	require.True(t, peers.wasCollecting, "the node was collecting before it asked for staging")
}
