// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// restoredPrimary is a primary of the given committee whose node stopped
// after it authored a header at round 1 (persisted, #4448) and before that
// header's certificate reached the DAG it restored: RestoreAuthored puts the
// header back into vote collection with this primary's own vote.
func restoredPrimary(t *testing.T, n int) (*Primary, types.HeaderDigest) {
	p, d, _ := restoredPrimaryWith(t, n)
	return p, d
}

func restoredPrimaryWith(t *testing.T, n int) (*Primary, types.HeaderDigest, []*testValidator) {
	t.Helper()
	var validators []*testValidator
	for i := 0; i < n; i++ {
		validators = append(validators, newTestValidator(t))
	}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()
	genesis := createGenesisCertificates(t, validators, committee, d)
	var parents []types.CertificateDigest
	for _, c := range genesis {
		parents = append(parents, c.Digest())
	}
	v := validators[0]
	h := types.NewHeader(v.pub, 1, committee.Epoch, nil, parents)
	require.NoError(t, h.Sign(v.priv))

	p := New(Config{Partition: "restore", KeyPair: v.priv}, committee, nil, d, nil)
	p.RestoreAuthored(h)
	return p, h.Digest(), validators
}

// TestARestoredOwnHeaderOfASoleValidatorIsCertified: a committee of one holds
// a quorum in its own vote. Restarted with its last header uncertified, the
// primary certifies it at start, as it does when it authors a header; before,
// it waited for votes no peer could send, and the partition stalled at the
// restored round with ownVotes=1 and certsThisRound=0.
func TestARestoredOwnHeaderOfASoleValidatorIsCertified(t *testing.T) {
	p, _ := restoredPrimary(t, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = p.Start(ctx) }()
	require.Eventually(t, func() bool { return p.dag.RoundCount(1) > 0 }, 5*time.Second, 10*time.Millisecond,
		"the restored header of a committee of one was never certified")
}

// TestARestoredOwnHeaderWithoutQuorumWaitsForVotes: in a committee of four
// the primary's own vote is no quorum, and nothing changes: the restored
// header waits for its peers' votes as before. Nothing persists the peers'
// votes, so this -- the own vote alone -- is what every restart of a
// multi-validator primary restores.
func TestARestoredOwnHeaderWithoutQuorumWaitsForVotes(t *testing.T) {
	p, digest := restoredPrimary(t, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = p.Start(ctx) }()
	time.Sleep(200 * time.Millisecond)
	require.Zero(t, p.dag.RoundCount(1), "a certificate formed without a quorum")
	p.pendingMu.Lock()
	_, waiting := p.ourHeaders[digest]
	votes := len(p.pendingVotes[digest])
	p.pendingMu.Unlock()
	require.True(t, waiting, "the restored header no longer waits for votes")
	require.Equal(t, 1, votes)
}

// TestARestoredOwnHeaderWithAQuorumOfVotesIsCertifiedAtStart: were a quorum
// of votes held for the restored header at Start -- in a committee of four,
// its own and two peers' -- the certificate is formed from them at Start. That
// is intended: it is the certificate the primary would have formed when the
// last of those votes arrived, of the header it already signed, from
// signatures it holds, and it signs nothing new. No code restores the peers'
// votes today, so a multi-validator restart is (i) above; this pins what
// certifyRestoredHeaders does if they ever are.
func TestARestoredOwnHeaderWithAQuorumOfVotesIsCertifiedAtStart(t *testing.T) {
	p, digest, validators := restoredPrimaryWith(t, 4)
	p.pendingMu.Lock()
	h := p.ourHeaders[digest]
	for _, v := range validators[1:3] {
		vote := types.NewVote(digest, h.Round, h.Epoch, v.pub)
		require.NoError(t, vote.Sign(v.priv))
		p.pendingVotes[digest] = append(p.pendingVotes[digest], vote)
	}
	p.pendingMu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = p.Start(ctx) }()
	require.Eventually(t, func() bool { return p.dag.RoundCount(1) > 0 }, 5*time.Second, 10*time.Millisecond,
		"a restored header with a quorum of votes was not certified at start")
}

// restoredSolePrimary is a primary whose held committee is v alone, of epoch
// 1, restored with a header by v at round 1 of the given epoch naming
// parents. d is the DAG the primary holds.
func restoredSolePrimary(t *testing.T, v *testValidator, d *dag.DAG, epoch uint64, parents []types.CertificateDigest) (*Primary, types.HeaderDigest) {
	t.Helper()
	committee := newTestCommittee([]*testValidator{v}, 1)
	h := types.NewHeader(v.pub, 1, epoch, nil, parents)
	require.NoError(t, h.Sign(v.priv))
	p := New(Config{Partition: "restore", KeyPair: v.priv}, committee, nil, d, nil)
	p.RestoreAuthored(h)
	return p, h.Digest()
}

// requireStillCollecting: the start-time attempt left the restored header as
// RestoreAuthored left it, with its own vote, and certified nothing.
func requireStillCollecting(t *testing.T, p *Primary, digest types.HeaderDigest) {
	t.Helper()
	p.certifyRestoredHeaders()
	p.wg.Wait() // any certificate's insert has run
	p.pendingMu.Lock()
	_, waiting := p.ourHeaders[digest]
	votes := len(p.pendingVotes[digest])
	certs := len(p.ourCerts)
	p.pendingMu.Unlock()
	require.True(t, waiting, "the restored header no longer waits for votes")
	require.Equal(t, 1, votes)
	require.Zero(t, certs, "a certificate was claimed for the restored round")
	require.Zero(t, p.certificatesCreated.Load(), "a certificate was created")
	require.Zero(t, p.dag.RoundCount(1), "a certificate was inserted")
}

// TestARestoredOwnHeaderOfALaterEpochIsNotCertifiedAtStart: the schedule
// held at start still describes an older epoch whose committee is this node
// alone, and the restored header is of the next epoch, of four validators.
// Counted against the old committee its own vote is a quorum; it is not one
// of the committee the header was made for, so the header is not certified
// at start and waits for votes.
func TestARestoredOwnHeaderOfALaterEpochIsNotCertifiedAtStart(t *testing.T) {
	var validators []*testValidator
	for i := 0; i < 4; i++ {
		validators = append(validators, newTestValidator(t))
	}
	v := validators[0]
	d := newTestDAG()
	genesis := createGenesisCertificates(t, []*testValidator{v}, newTestCommittee([]*testValidator{v}, 1), d)
	p, digest := restoredSolePrimary(t, v, d, 2, []types.CertificateDigest{genesis[0].Digest()})
	require.Equal(t, uint64(1), p.committeeAt(1).Epoch)
	requireStillCollecting(t, p, digest)
}

// TestARestoredOwnHeaderWithAMissingParentIsNotCertifiedAtStart: a parent
// the restored header names is not in the restored DAG. Certified at start,
// the header and its vote were dropped before the DAG refused the
// certificate, and the header was lost; it is left waiting instead.
func TestARestoredOwnHeaderWithAMissingParentIsNotCertifiedAtStart(t *testing.T) {
	v := newTestValidator(t)
	elsewhere := newTestDAG()
	genesis := createGenesisCertificates(t, []*testValidator{v}, newTestCommittee([]*testValidator{v}, 1), elsewhere)
	d := newTestDAG()
	p, digest := restoredSolePrimary(t, v, d, 1, []types.CertificateDigest{genesis[0].Digest()})
	require.False(t, d.Contains(genesis[0].Digest()))
	requireStillCollecting(t, p, digest)
}
