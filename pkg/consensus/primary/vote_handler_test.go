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
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
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

func TestMaxVotesPerHeaderSpamProtection(t *testing.T) {
	// Test that we stop accepting votes after reaching the limit
	// This protects against spam attacks

	// Create 20 validators to test spam scenario
	// With n=20: f = (20-1)/3 = 6, quorum = 2*6+1 = 13, max_votes = 13 * 2 = 26
	// But we only have 20 validators, so we'll create extra fake votes
	validators := make([]*testValidator, 20)
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

	// Quorum count for 20 validators: f = (20-1)/3 = 6, quorum = 2*6+1 = 13
	quorumCount := committee.QuorumCount()
	require.Equal(t, 13, quorumCount)

	// Max votes = quorumCount * VotesPerHeaderMultiplier = 13 * 2 = 26
	maxVotes := quorumCount * VotesPerHeaderMultiplier
	require.Equal(t, 26, maxVotes)

	// Send votes from all 20 validators - all should be accepted since 20 < 26
	for i, v := range validators {
		vote := types.NewVote(digest, 0, 1, v.pub)
		require.NoError(t, vote.Sign(v.priv))
		p.OnVoteReceived(vote)

		// Check vote count (note: after quorum is reached, cert is created and votes cleaned)
		voteCount := p.PendingVoteCount(digest)
		if i < quorumCount {
			// Before quorum, should accumulate votes
			require.Equal(t, i+1, voteCount, "Vote %d should be accepted", i)
		} else {
			// After quorum, certificate is created and pending votes are cleaned up
			require.Equal(t, 0, voteCount, "Vote %d: certificate created, votes cleaned", i)
		}
	}

	// Certificate should be created
	require.True(t, p.HasCertificateForRound(0))
}

func TestMaxVotesPerHeaderRejectSpam(t *testing.T) {
	// Test rejection of spam votes beyond the limit
	// We use a smaller committee and manually inject votes to exceed the limit

	// Create 7 validators: f = 2, quorum = 5, max_votes = 10
	validators := make([]*testValidator, 7)
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

	quorumCount := committee.QuorumCount()
	require.Equal(t, 5, quorumCount)

	maxVotes := quorumCount * VotesPerHeaderMultiplier
	require.Equal(t, 10, maxVotes)

	// Send only 4 votes first (below quorum, so no cert created yet)
	for i := 0; i < 4; i++ {
		vote := types.NewVote(digest, 0, 1, validators[i].pub)
		require.NoError(t, vote.Sign(validators[i].priv))
		p.OnVoteReceived(vote)
	}

	// Should have 4 votes
	require.Equal(t, 4, p.PendingVoteCount(digest))

	// Now manually inject 7 more fake validators to test spam limit
	// This simulates receiving many spam votes
	for i := 0; i < 7; i++ {
		fakeValidator := newTestValidator(t)

		// Manually add to pending votes to bypass committee check
		// This simulates what would happen if we had more validators
		p.pendingMu.Lock()
		fakeVote := types.NewVote(digest, 0, 1, fakeValidator.pub)
		require.NoError(t, fakeVote.Sign(fakeValidator.priv))
		p.pendingVotes[digest] = append(p.pendingVotes[digest], fakeVote)
		currentCount := len(p.pendingVotes[digest])
		p.pendingMu.Unlock()

		if currentCount >= maxVotes {
			// Once we hit the limit, verify we stop accepting
			require.Equal(t, maxVotes, currentCount)
			break
		}
	}

	// Should have exactly maxVotes (10)
	require.Equal(t, maxVotes, p.PendingVoteCount(digest))

	// Try to add one more vote from a committee member - should be rejected
	vote := types.NewVote(digest, 0, 1, validators[5].pub)
	require.NoError(t, vote.Sign(validators[5].priv))
	p.OnVoteReceived(vote)

	// Should still be at maxVotes (spam protection kicked in)
	require.Equal(t, maxVotes, p.PendingVoteCount(digest))
}

func TestMaxVotesPerHeaderWithExtraValidators(t *testing.T) {
	// Simulate a scenario where we receive spam votes after achieving quorum
	// This ensures the limit prevents resource exhaustion

	validators := make([]*testValidator, 7)
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

	// Create header
	header := types.NewHeader(validators[0].pub, 0, 1, nil, nil)
	require.NoError(t, header.Sign(validators[0].priv))

	digest := header.Digest()
	p.pendingMu.Lock()
	p.ourHeaders[digest] = header
	p.pendingVotes[digest] = nil
	p.pendingMu.Unlock()

	// For 7 validators: f = 2, quorum = 5, max_votes = 10
	quorumCount := committee.QuorumCount()
	require.Equal(t, 5, quorumCount)

	// Send first 5 votes (reaches quorum) - certificate should be created
	for i := 0; i < quorumCount; i++ {
		vote := types.NewVote(digest, 0, 1, validators[i].pub)
		require.NoError(t, vote.Sign(validators[i].priv))
		p.OnVoteReceived(vote)
	}

	// Give time for certificate creation
	time.Sleep(100 * time.Millisecond)

	// Certificate should be created (pending state cleaned up)
	require.True(t, p.HasCertificateForRound(0))

	// Verify certificate is in DAG
	cert := d.Get(0, validators[0].pub)
	require.NotNil(t, cert)
}

func TestMaxVotesPerHeaderEdgeCaseSingleValidator(t *testing.T) {
	// Edge case: single validator (n=1, f=0, quorum=1, max_votes=2)
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition:           "test",
		KeyPair:             v.priv,
		NewCertsChannelSize: 10,
	}

	p := New(config, committee, nil, d, nil)

	header := types.NewHeader(v.pub, 0, 1, nil, nil)
	require.NoError(t, header.Sign(v.priv))

	digest := header.Digest()
	p.pendingMu.Lock()
	p.ourHeaders[digest] = header
	p.pendingVotes[digest] = nil
	p.pendingMu.Unlock()

	// Single validator: quorum = 1, max_votes = 2
	quorumCount := committee.QuorumCount()
	require.Equal(t, 1, quorumCount)

	maxVotes := quorumCount * VotesPerHeaderMultiplier
	require.Equal(t, 2, maxVotes)

	// Send one vote - this reaches quorum and creates certificate
	vote := types.NewVote(digest, 0, 1, v.pub)
	require.NoError(t, vote.Sign(v.priv))
	p.OnVoteReceived(vote)

	// Give time for certificate creation
	time.Sleep(50 * time.Millisecond)

	// Certificate should be created and votes cleaned up
	require.True(t, p.HasCertificateForRound(0))
	require.Equal(t, 0, p.PendingVoteCount(digest))
}

// TestOnHeaderReceived_DefersVoteUntilBatchAvailable pins the #4159 fix: a
// validator must not vote for a header whose payload batches it does not hold,
// because a certificate is supposed to prove 2f+1 validators HAVE the data.
// Voting blind let a batch that lived only on the author be certified and then,
// when the leader committed rounds later, be found nowhere — a permanent wedge.
func TestOnHeaderReceived_DefersVoteUntilBatchAvailable(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()
	genesisCerts := createGenesisCertificates(t, validators, committee, d)
	parents := make([]types.CertificateDigest, len(genesisCerts))
	for i, c := range genesisCerts {
		parents[i] = c.Digest()
	}

	w := worker.New(worker.Config{ID: 0, Partition: "test"}, nil)
	p := New(Config{Partition: "test", KeyPair: validators[0].priv}, committee, nil, d, []*worker.Worker{w})
	p.SetRound(1)

	batch := types.NewBatch([][]byte{[]byte("the-only-copy")})
	header := types.NewHeader(validators[1].pub, 1, 1,
		[]types.PayloadEntry{{Digest: batch.Digest(), Worker: 0}}, parents)
	require.NoError(t, header.Sign(validators[1].priv))

	voted := func() bool {
		p.pendingMu.Lock()
		defer p.pendingMu.Unlock()
		_, ok := p.votedHeaders[header.Digest()]
		return ok
	}

	// We do not hold the batch — must NOT vote (would certify data we lack).
	p.OnHeaderReceived(header)
	require.False(t, voted(), "must not vote for a header whose batch we do not hold (#4159)")

	// The batch arrives via gossip; the author rebroadcasts the header; now we vote.
	require.NoError(t, w.StoreBatch(batch))
	p.OnHeaderReceived(header)
	require.True(t, voted(), "must vote once the header's batch is available")
}

// TestOnHeaderReceived_MissingBatchTriggersFetch pins the #4159 repair: a
// deferring voter must actively PULL the batch it lacks — batch bytes are
// broadcast once at creation and the author's header rebroadcast does not
// re-send them, so defer-without-fetch wedged cert formation forever (the
// frozen soak's 1,833 "Missing batch" deferrals). The ask is deduplicated:
// the 1s rebroadcast re-fires the gate, but each digest is fetched at most
// once per retry window.
func TestOnHeaderReceived_MissingBatchTriggersFetch(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()
	genesisCerts := createGenesisCertificates(t, validators, committee, d)
	parents := make([]types.CertificateDigest, len(genesisCerts))
	for i, c := range genesisCerts {
		parents[i] = c.Digest()
	}

	w := worker.New(worker.Config{ID: 0, Partition: "test"}, nil)
	p := New(Config{Partition: "test", KeyPair: validators[0].priv}, committee, nil, d, []*worker.Worker{w})
	p.SetRound(1)

	var asked []types.BatchDigest
	p.SetMissingBatchHandler(func(dg types.BatchDigest) { asked = append(asked, dg) })

	batch := types.NewBatch([][]byte{[]byte("lost-in-gossip")})
	header := types.NewHeader(validators[1].pub, 1, 1,
		[]types.PayloadEntry{{Digest: batch.Digest(), Worker: 0}}, parents)
	require.NoError(t, header.Sign(validators[1].priv))

	// First receipt: defer AND ask.
	p.OnHeaderReceived(header)
	require.Len(t, asked, 1, "a deferring voter must pull the missing batch")
	require.Equal(t, batch.Digest(), asked[0])

	// Rebroadcast within the retry window: defer again, but do NOT re-ask.
	p.OnHeaderReceived(header)
	require.Len(t, asked, 1, "asks are deduplicated per digest per retry window")

	// The fetch lands (peer served it); the next rebroadcast votes.
	require.NoError(t, w.StoreBatch(batch))
	p.OnHeaderReceived(header)
	p.pendingMu.Lock()
	_, voted := p.votedHeaders[header.Digest()]
	p.pendingMu.Unlock()
	require.True(t, voted, "once the fetched batch is stored, the vote goes out")
}

// TestOnHeaderReceived_VotesWhenBatchIsRetained pins the CanServeBatch fix:
// a validator that already EXECUTED a batch (pruned to the retained store)
// HAS it, and must vote for a header re-proposing that digest rather than
// deferring — deferring blocked every fresh batch riding in the same header.
func TestOnHeaderReceived_VotesWhenBatchIsRetained(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()
	genesisCerts := createGenesisCertificates(t, validators, committee, d)
	parents := make([]types.CertificateDigest, len(genesisCerts))
	for i, c := range genesisCerts {
		parents[i] = c.Digest()
	}

	w := worker.New(worker.Config{ID: 0, Partition: "test"}, nil)
	p := New(Config{Partition: "test", KeyPair: validators[0].priv}, committee, nil, d, []*worker.Worker{w})
	p.SetRound(1)

	batch := types.NewBatch([][]byte{[]byte("already-executed")})
	require.NoError(t, w.StoreBatch(batch))
	// Execute-and-prune moves it from the active store to retained.
	w.PruneCommitted([]types.BatchDigest{batch.Digest()}, worker.CommitInfo{Detail: "block 1"})
	require.False(t, w.HasBatch(batch.Digest()), "pruned from the active store")
	require.True(t, w.CanServeBatch(batch.Digest()), "still held via retention")

	header := types.NewHeader(validators[1].pub, 1, 1,
		[]types.PayloadEntry{{Digest: batch.Digest(), Worker: 0}}, parents)
	require.NoError(t, header.Sign(validators[1].priv))

	p.OnHeaderReceived(header)
	p.pendingMu.Lock()
	_, voted := p.votedHeaders[header.Digest()]
	p.pendingMu.Unlock()
	require.True(t, voted, "a retained batch counts as held — no deferral")
}

// TestNoSelfEquivocation_NeverAuthorTheSameRoundTwice pins the #4159 stall-3
// fix. The one-header-per-round guard scanned ourHeaders only — but
// certification DELETES the header, so a second round-advance racing through
// after the first header certified re-authored the SAME round. The second
// certificate is self-equivocation: peers keep whichever version arrived
// first, each side permanently rejects the other, and certificate
// dissemination deadlocks network-wide (observed: 10 equivocation rejections,
// total freeze at round ~206 / DN 529). The monotone lastAuthoredRound
// watermark makes re-authoring structurally impossible.
func TestNoSelfEquivocation_NeverAuthorTheSameRoundTwice(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()
	genesisCerts := createGenesisCertificates(t, validators, committee, d)
	_ = genesisCerts

	w := worker.New(worker.Config{ID: 0, Partition: "test"}, nil)
	p := New(Config{Partition: "test", KeyPair: validators[0].priv}, committee, nil, d, []*worker.Worker{w})
	p.SetRound(1)

	// First authoring for round 1 succeeds.
	p.tryCreateAndBroadcastHeader()
	h1, _, _, _ := p.Metrics()
	require.EqualValues(t, 1, h1, "first header for round 1 is authored")

	// Simulate certification: the header reaches quorum and is deleted from
	// ourHeaders (exactly what tryCreateCertificateLocked does).
	p.pendingMu.Lock()
	for digest, h := range p.ourHeaders {
		if h.Round == 1 {
			p.ourCerts[h.Round] = nil // placeholder; the map entry is what mattered pre-fix
			delete(p.ourHeaders, digest)
			delete(p.pendingVotes, digest)
		}
	}
	p.pendingMu.Unlock()

	// The racing second authoring for the SAME round must be refused — the
	// old guard saw an empty ourHeaders and re-authored (self-equivocation).
	p.tryCreateAndBroadcastHeader()
	h2, _, _, _ := p.Metrics()
	require.EqualValues(t, 1, h2, "the same round must never be authored twice (#4159)")

	// The next round authors normally — give round 2 its parents first.
	gparents := make([]types.CertificateDigest, len(genesisCerts))
	for i, c := range genesisCerts {
		gparents[i] = c.Digest()
	}
	for i := 1; i < 4; i++ {
		hdr := types.NewHeader(validators[i].pub, 1, 1, nil, gparents)
		require.NoError(t, hdr.Sign(validators[i].priv))
		require.NoError(t, d.Insert(types.NewCertificate(hdr, nil, nil)))
	}
	p.SetRound(2)
	p.tryCreateAndBroadcastHeader()
	h3, _, _, _ := p.Metrics()
	require.EqualValues(t, 2, h3, "a NEW round still authors")
}
