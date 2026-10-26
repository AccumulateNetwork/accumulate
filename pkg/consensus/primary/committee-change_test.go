// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// signedCert is a certificate of author at round in epoch, signed by every
// one of signers. It names a parent the DAG does not hold: a certificate
// above round 0 names parents (#4591), and which ones is not what these
// tests are about.
func signedCert(t *testing.T, author *testValidator, round types.Round, epoch uint64, signers []*testValidator) *types.Certificate {
	t.Helper()
	header := types.NewHeader(author.pub, round, epoch, nil, []types.CertificateDigest{{1}})
	require.NoError(t, header.Sign(author.priv))
	sigs := make([][]byte, len(signers))
	authors := make([]uint16, len(signers))
	for i, v := range signers {
		vote := types.NewVote(header.Digest(), round, epoch, v.pub)
		require.NoError(t, vote.Sign(v.priv))
		sigs[i] = vote.Signature
		authors[i] = uint16(i)
	}
	return types.NewCertificate(header, sigs, authors)
}

// A certificate is verified with the committee of its own round (#4593). One
// of the previous epoch below the activation round still verifies after the
// node has scheduled the change -- it is needed to catch up the rounds below
// it -- and one of the previous epoch at or above that round does not.
func TestACertificateIsVerifiedWithTheCommitteeOfItsRound(t *testing.T) {
	old := make([]*testValidator, 4)
	for i := range old {
		old[i] = newTestValidator(t)
	}
	next := append([]*testValidator{newTestValidator(t)}, old[1:]...)
	const activation = 10
	nextCommittee := newTestCommittee(next, 2)
	nextCommittee.ActivationRound = activation

	p := New(Config{Partition: "test", KeyPair: old[1].priv}, newTestCommittee(old, 1), nil, newTestDAG(), nil)
	require.NoError(t, p.UpdateCommittee(nextCommittee))
	p.SetRound(activation + 2)

	require.NotEqual(t, CertInvalid, p.receiveCertificate(signedCert(t, old[0], activation-2, 1, old), false),
		"the previous epoch below the activation round")
	require.Equal(t, CertInvalid, p.receiveCertificate(signedCert(t, old[0], activation, 1, old), false),
		"the previous epoch at the activation round")
	require.NotEqual(t, CertInvalid, p.receiveCertificate(signedCert(t, next[0], activation, 2, next), false),
		"the new epoch at the activation round")
	require.Equal(t, CertInvalid, p.receiveCertificate(signedCert(t, next[0], activation-2, 2, next), false),
		"the new epoch below it")
}

// A primary does not advance past its round limit: the next round may belong
// to a committee change its node has not executed yet (#4593).
func TestAPrimaryDoesNotAdvancePastItsRoundLimit(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()
	createGenesisCertificates(t, validators, committee, d)

	p := New(Config{Partition: "test", KeyPair: validators[0].priv}, committee, nil, d, nil)
	var limit atomic.Uint64
	p.SetRoundLimit(func() (types.Round, bool) { return types.Round(limit.Load()), true })

	p.tryAdvanceRound()
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, types.Round(0), p.CurrentRound(), "round 0 has its quorum, and round 1 is past the limit")

	limit.Store(1)
	p.tryAdvanceRound()
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, types.Round(1), p.CurrentRound())
}

// A primary does not vote above its round limit: the round may belong to a
// committee change its node has not executed yet. The author rebroadcasts,
// and the vote goes out once the limit has passed the round (#4593).
func TestAPrimaryDoesNotVoteAboveItsRoundLimit(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()
	createGenesisCertificates(t, validators, committee, d)

	p := New(Config{Partition: "test", KeyPair: validators[0].priv}, committee, nil, d, nil)
	var limit atomic.Uint64
	p.SetRoundLimit(func() (types.Round, bool) { return types.Round(limit.Load()), true })
	p.SetRound(1)

	var parents []types.CertificateDigest
	for _, c := range d.GetRound(0) {
		parents = append(parents, c.Digest())
	}
	header := types.NewHeader(validators[1].pub, 1, committee.Epoch, nil, parents)
	require.NoError(t, header.Sign(validators[1].priv))
	voted := func() bool {
		p.pendingMu.Lock()
		defer p.pendingMu.Unlock()
		_, ok := p.votedHeaders[header.Digest()]
		return ok
	}

	p.handleHeader(header, false)
	require.False(t, voted(), "round 1 is above the limit")

	limit.Store(1)
	p.handleHeader(header, false)
	require.True(t, voted(), "the rebroadcast, once the limit has passed the round")
}
