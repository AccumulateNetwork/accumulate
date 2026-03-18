// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"bytes"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

func TestOnVoteReceivedValid(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   validators[0].priv,
	}

	p := New(config, committee, nil, d, nil)

	// Create a header from validator 0
	header := types.NewHeader(validators[0].pub, 0, 1, nil, nil)
	require.NoError(t, header.Sign(validators[0].priv))

	// Register header as ours
	digest := header.Digest()
	p.pendingMu.Lock()
	p.ourHeaders[digest] = header
	p.pendingVotes[digest] = nil
	p.pendingMu.Unlock()

	// Create vote from validator 1
	vote := types.NewVote(digest, 0, 1, validators[1].pub)
	require.NoError(t, vote.Sign(validators[1].priv))

	// Process vote
	p.OnVoteReceived(vote)

	// Check vote was added
	require.Equal(t, 1, p.PendingVoteCount(digest))
}

func TestOnVoteReceivedDuplicate(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   validators[0].priv,
	}

	p := New(config, committee, nil, d, nil)

	// Create header
	header := types.NewHeader(validators[0].pub, 0, 1, nil, nil)
	require.NoError(t, header.Sign(validators[0].priv))

	digest := header.Digest()
	p.pendingMu.Lock()
	p.ourHeaders[digest] = header
	p.pendingVotes[digest] = nil
	p.pendingMu.Unlock()

	// Create vote from validator 1
	vote := types.NewVote(digest, 0, 1, validators[1].pub)
	require.NoError(t, vote.Sign(validators[1].priv))

	// Process vote twice
	p.OnVoteReceived(vote)
	p.OnVoteReceived(vote)

	// Should only count once
	require.Equal(t, 1, p.PendingVoteCount(digest))
}

func TestOnVoteReceivedUnknownHeader(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   v.priv,
	}

	p := New(config, committee, nil, d, nil)

	// Create vote for a header we don't have
	var unknownDigest types.HeaderDigest
	copy(unknownDigest[:], []byte("unknown header digest here"))

	vote := types.NewVote(unknownDigest, 0, 1, v.pub)
	require.NoError(t, vote.Sign(v.priv))

	// Should not panic or error
	p.OnVoteReceived(vote)

	// No votes should be recorded
	require.Equal(t, 0, p.PendingVoteCount(unknownDigest))
}

func TestOnVoteReceivedInvalidSignature(t *testing.T) {
	validators := make([]*testValidator, 2)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   validators[0].priv,
	}

	p := New(config, committee, nil, d, nil)

	// Create header
	header := types.NewHeader(validators[0].pub, 0, 1, nil, nil)
	require.NoError(t, header.Sign(validators[0].priv))

	digest := header.Digest()
	p.pendingMu.Lock()
	p.ourHeaders[digest] = header
	p.pendingVotes[digest] = nil
	p.pendingMu.Unlock()

	// Create vote with bad signature
	vote := types.NewVote(digest, 0, 1, validators[1].pub)
	vote.Signature = make([]byte, 64) // zeros - invalid

	// Should reject
	p.OnVoteReceived(vote)

	require.Equal(t, 0, p.PendingVoteCount(digest))
}

func TestOnVoteReceivedWrongRound(t *testing.T) {
	validators := make([]*testValidator, 2)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   validators[0].priv,
	}

	p := New(config, committee, nil, d, nil)

	// Create header for round 0
	header := types.NewHeader(validators[0].pub, 0, 1, nil, nil)
	require.NoError(t, header.Sign(validators[0].priv))

	digest := header.Digest()
	p.pendingMu.Lock()
	p.ourHeaders[digest] = header
	p.pendingVotes[digest] = nil
	p.pendingMu.Unlock()

	// Create vote for wrong round
	vote := types.NewVote(digest, 5, 1, validators[1].pub) // round 5 doesn't match
	require.NoError(t, vote.Sign(validators[1].priv))

	p.OnVoteReceived(vote)

	// Vote should be rejected
	require.Equal(t, 0, p.PendingVoteCount(digest))
}

func TestOnVoteReceivedWrongEpoch(t *testing.T) {
	validators := make([]*testValidator, 2)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   validators[0].priv,
	}

	p := New(config, committee, nil, d, nil)

	// Create header for epoch 1
	header := types.NewHeader(validators[0].pub, 0, 1, nil, nil)
	require.NoError(t, header.Sign(validators[0].priv))

	digest := header.Digest()
	p.pendingMu.Lock()
	p.ourHeaders[digest] = header
	p.pendingVotes[digest] = nil
	p.pendingMu.Unlock()

	// Create vote for wrong epoch
	vote := types.NewVote(digest, 0, 99, validators[1].pub) // epoch 99 doesn't match
	require.NoError(t, vote.Sign(validators[1].priv))

	p.OnVoteReceived(vote)

	// Vote should be rejected
	require.Equal(t, 0, p.PendingVoteCount(digest))
}

func TestOnVoteReceivedUnknownValidator(t *testing.T) {
	v1 := newTestValidator(t)
	v2 := newTestValidator(t) // Not in committee

	validators := []*testValidator{v1}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   v1.priv,
	}

	p := New(config, committee, nil, d, nil)

	// Create header
	header := types.NewHeader(v1.pub, 0, 1, nil, nil)
	require.NoError(t, header.Sign(v1.priv))

	digest := header.Digest()
	p.pendingMu.Lock()
	p.ourHeaders[digest] = header
	p.pendingVotes[digest] = nil
	p.pendingMu.Unlock()

	// Create vote from unknown validator
	vote := types.NewVote(digest, 0, 1, v2.pub)
	require.NoError(t, vote.Sign(v2.priv))

	p.OnVoteReceived(vote)

	// Vote should be rejected
	require.Equal(t, 0, p.PendingVoteCount(digest))
}

func TestOnVoteReceivedNil(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   v.priv,
	}

	p := New(config, committee, nil, d, nil)

	// Should not panic
	p.OnVoteReceived(nil)
}

func TestOnHeaderReceivedValid(t *testing.T) {
	validators := make([]*testValidator, 2)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   validators[0].priv,
	}

	p := New(config, committee, nil, d, nil)

	// Create header from another validator
	header := types.NewHeader(validators[1].pub, 0, 1, nil, nil)
	require.NoError(t, header.Sign(validators[1].priv))

	// Process header - this would normally broadcast a vote
	// Since we have no gossip layer, it won't actually send
	p.OnHeaderReceived(header)

	// Check that we marked it as voted
	p.pendingMu.Lock()
	_, voted := p.votedHeaders[header.Digest()]
	p.pendingMu.Unlock()

	require.True(t, voted)
}

func TestOnHeaderReceivedOwnHeader(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   v.priv,
	}

	p := New(config, committee, nil, d, nil)

	// Create header from ourselves
	header := types.NewHeader(v.pub, 0, 1, nil, nil)
	require.NoError(t, header.Sign(v.priv))

	// Process our own header
	p.OnHeaderReceived(header)

	// Should not vote on our own header
	p.pendingMu.Lock()
	_, voted := p.votedHeaders[header.Digest()]
	p.pendingMu.Unlock()

	require.False(t, voted)
}

func TestOnHeaderReceivedInvalidSignature(t *testing.T) {
	validators := make([]*testValidator, 2)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   validators[0].priv,
	}

	p := New(config, committee, nil, d, nil)

	// Create header with invalid signature
	header := types.NewHeader(validators[1].pub, 0, 1, nil, nil)
	header.Signature = make([]byte, 64) // zeros - invalid

	p.OnHeaderReceived(header)

	// Should not vote
	p.pendingMu.Lock()
	_, voted := p.votedHeaders[header.Digest()]
	p.pendingMu.Unlock()

	require.False(t, voted)
}

func TestOnHeaderReceivedWrongEpoch(t *testing.T) {
	validators := make([]*testValidator, 2)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   validators[0].priv,
	}

	p := New(config, committee, nil, d, nil)

	// Create header with wrong epoch
	header := types.NewHeader(validators[1].pub, 0, 99, nil, nil)
	require.NoError(t, header.Sign(validators[1].priv))

	p.OnHeaderReceived(header)

	// Should not vote
	p.pendingMu.Lock()
	_, voted := p.votedHeaders[header.Digest()]
	p.pendingMu.Unlock()

	require.False(t, voted)
}

func TestOnHeaderReceivedUnknownValidator(t *testing.T) {
	v1 := newTestValidator(t)
	v2 := newTestValidator(t) // Not in committee

	validators := []*testValidator{v1}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   v1.priv,
	}

	p := New(config, committee, nil, d, nil)

	// Create header from unknown validator
	header := types.NewHeader(v2.pub, 0, 1, nil, nil)
	require.NoError(t, header.Sign(v2.priv))

	p.OnHeaderReceived(header)

	// Should not vote
	p.pendingMu.Lock()
	_, voted := p.votedHeaders[header.Digest()]
	p.pendingMu.Unlock()

	require.False(t, voted)
}

func TestOnHeaderReceivedOldRound(t *testing.T) {
	validators := make([]*testValidator, 2)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   validators[0].priv,
	}

	p := New(config, committee, nil, d, nil)
	p.SetRound(10) // Far ahead

	// Create header for old round
	header := types.NewHeader(validators[1].pub, 0, 1, nil, nil)
	require.NoError(t, header.Sign(validators[1].priv))

	p.OnHeaderReceived(header)

	// Should not vote on old header
	p.pendingMu.Lock()
	_, voted := p.votedHeaders[header.Digest()]
	p.pendingMu.Unlock()

	require.False(t, voted)
}

func TestOnHeaderReceivedFutureRound(t *testing.T) {
	validators := make([]*testValidator, 2)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   validators[0].priv,
	}

	p := New(config, committee, nil, d, nil)

	// Create header for far future round
	header := types.NewHeader(validators[1].pub, 100, 1, nil, nil)
	require.NoError(t, header.Sign(validators[1].priv))

	p.OnHeaderReceived(header)

	// Should not vote on future header
	p.pendingMu.Lock()
	_, voted := p.votedHeaders[header.Digest()]
	p.pendingMu.Unlock()

	require.False(t, voted)
}

func TestOnHeaderReceivedMissingParent(t *testing.T) {
	validators := make([]*testValidator, 2)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   validators[0].priv,
	}

	p := New(config, committee, nil, d, nil)
	p.SetRound(1)

	// Create header with parent we don't have
	var fakeParent types.CertificateDigest
	copy(fakeParent[:], []byte("fake parent digest here!"))

	header := types.NewHeader(validators[1].pub, 1, 1, nil, []types.CertificateDigest{fakeParent})
	require.NoError(t, header.Sign(validators[1].priv))

	p.OnHeaderReceived(header)

	// Should not vote - missing parent
	p.pendingMu.Lock()
	_, voted := p.votedHeaders[header.Digest()]
	p.pendingMu.Unlock()

	require.False(t, voted)
}

func TestOnHeaderReceivedWithParents(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	// Create genesis certs
	genesisCerts := createGenesisCertificates(t, validators, committee, d)

	config := Config{
		Partition: "test",
		KeyPair:   validators[0].priv,
	}

	p := New(config, committee, nil, d, nil)
	p.SetRound(1)

	// Create header with valid parents
	parents := make([]types.CertificateDigest, len(genesisCerts))
	for i, c := range genesisCerts {
		parents[i] = c.Digest()
	}

	header := types.NewHeader(validators[1].pub, 1, 1, nil, parents)
	require.NoError(t, header.Sign(validators[1].priv))

	p.OnHeaderReceived(header)

	// Should vote - all parents exist
	p.pendingMu.Lock()
	_, voted := p.votedHeaders[header.Digest()]
	p.pendingMu.Unlock()

	require.True(t, voted)
}

func TestOnHeaderReceivedNil(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   v.priv,
	}

	p := New(config, committee, nil, d, nil)

	// Should not panic
	p.OnHeaderReceived(nil)
}

func TestOnHeaderReceivedDuplicate(t *testing.T) {
	validators := make([]*testValidator, 2)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   validators[0].priv,
	}

	p := New(config, committee, nil, d, nil)

	// Create header
	header := types.NewHeader(validators[1].pub, 0, 1, nil, nil)
	require.NoError(t, header.Sign(validators[1].priv))

	// Process twice
	p.OnHeaderReceived(header)
	initialVotesSent := p.votesSent.Load()

	p.OnHeaderReceived(header)
	finalVotesSent := p.votesSent.Load()

	// Should only vote once
	require.Equal(t, initialVotesSent, finalVotesSent)
}

func TestCertificateCreationWithQuorum(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition:           "test",
		KeyPair:             validators[0].priv,
		NewCertsChannelSize: 10,
	}

	p := New(config, committee, nil, d, nil)

	// Create header from validator 0
	header := types.NewHeader(validators[0].pub, 0, 1, nil, nil)
	require.NoError(t, header.Sign(validators[0].priv))

	digest := header.Digest()
	p.pendingMu.Lock()
	p.ourHeaders[digest] = header
	p.pendingVotes[digest] = nil
	p.pendingMu.Unlock()

	// Send votes from all validators (including ourselves)
	for _, v := range validators {
		vote := types.NewVote(digest, 0, 1, v.pub)
		require.NoError(t, vote.Sign(v.priv))
		p.OnVoteReceived(vote)
	}

	// Give time for certificate creation
	time.Sleep(100 * time.Millisecond)

	// Should have created certificate
	require.True(t, p.HasCertificateForRound(0))

	// Certificate should be in DAG
	cert := d.Get(0, validators[0].pub)
	require.NotNil(t, cert)

	// Verify certificate
	require.NoError(t, cert.Verify(committee))
}

func TestCertificateCreationNotEnoughVotes(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   validators[0].priv,
	}

	p := New(config, committee, nil, d, nil)

	// Create header
	header := types.NewHeader(validators[0].pub, 0, 1, nil, nil)
	require.NoError(t, header.Sign(validators[0].priv))

	digest := header.Digest()
	p.pendingMu.Lock()
	p.ourHeaders[digest] = header
	p.pendingVotes[digest] = nil
	p.pendingMu.Unlock()

	// Only send one vote - not enough for quorum
	vote := types.NewVote(digest, 0, 1, validators[1].pub)
	require.NoError(t, vote.Sign(validators[1].priv))
	p.OnVoteReceived(vote)

	// Should not have certificate
	require.False(t, p.HasCertificateForRound(0))
}

func TestCreateCertificateFromVotesEmpty(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   v.priv,
	}

	p := New(config, committee, nil, d, nil)

	header := types.NewHeader(v.pub, 0, 1, nil, nil)

	// Empty votes should return nil
	cert := p.createCertificateFromVotes(header, nil)
	require.Nil(t, cert)

	cert = p.createCertificateFromVotes(header, []*types.Vote{})
	require.Nil(t, cert)
}

func TestCreateCertificateFromVotesValid(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   validators[0].priv,
	}

	p := New(config, committee, nil, d, nil)

	header := types.NewHeader(validators[0].pub, 0, 1, nil, nil)
	require.NoError(t, header.Sign(validators[0].priv))

	// Create votes
	votes := make([]*types.Vote, len(validators))
	for i, v := range validators {
		vote := types.NewVote(header.Digest(), 0, 1, v.pub)
		require.NoError(t, vote.Sign(v.priv))
		votes[i] = vote
	}

	cert := p.createCertificateFromVotes(header, votes)
	require.NotNil(t, cert)

	// Verify certificate
	require.NoError(t, cert.Verify(committee))
	require.Equal(t, types.Round(0), cert.Round())
	require.True(t, bytes.Equal(validators[0].pub, cert.Author()))
}

func TestHexEncode(t *testing.T) {
	// Test hexEncode helper
	pub, _, _ := ed25519.GenerateKey(nil)

	encoded := hexEncode(pub)
	require.NotEmpty(t, encoded)
	require.Len(t, encoded, 8) // First 8 chars of hex

	// Edge case - short key
	shortKey := ed25519.PublicKey([]byte{1, 2})
	encoded = hexEncode(shortKey)
	require.Empty(t, encoded)
}
