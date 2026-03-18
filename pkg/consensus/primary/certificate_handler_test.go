// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

func TestOnCertificateReceivedValid(t *testing.T) {
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

	// Create a valid certificate
	header := types.NewHeader(validators[1].pub, 0, 1, nil, nil)
	require.NoError(t, header.Sign(validators[1].priv))

	// Create signatures from all validators
	sigs := make([][]byte, len(validators))
	authors := make([]uint16, len(validators))

	headerDigest := header.Digest()
	for i, v := range validators {
		vote := types.NewVote(headerDigest, 0, 1, v.pub)
		require.NoError(t, vote.Sign(v.priv))
		sigs[i] = vote.Signature
		authors[i] = uint16(i)
	}

	cert := types.NewCertificate(*header, sigs, authors)

	// Process certificate
	p.OnCertificateReceived(cert)

	// Certificate should be in DAG
	stored := d.GetByDigest(cert.Digest())
	require.NotNil(t, stored)
	require.Equal(t, cert.Digest(), stored.Digest())
}

func TestOnCertificateReceivedSignalsBullshark(t *testing.T) {
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

	// Create a valid certificate
	header := types.NewHeader(validators[1].pub, 0, 1, nil, nil)
	require.NoError(t, header.Sign(validators[1].priv))

	sigs := make([][]byte, len(validators))
	authors := make([]uint16, len(validators))

	for i, v := range validators {
		vote := types.NewVote(header.Digest(), 0, 1, v.pub)
		require.NoError(t, vote.Sign(v.priv))
		sigs[i] = vote.Signature
		authors[i] = uint16(i)
	}

	cert := types.NewCertificate(*header, sigs, authors)

	// Start listening on new certs channel
	certChan := p.NewCertificates()

	// Process certificate
	p.OnCertificateReceived(cert)

	// Should receive notification
	select {
	case received := <-certChan:
		require.Equal(t, cert.Digest(), received.Digest())
	case <-time.After(100 * time.Millisecond):
		t.Fatal("did not receive certificate notification")
	}
}

func TestOnCertificateReceivedInvalid(t *testing.T) {
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

	// Create certificate with invalid signatures
	header := types.NewHeader(validators[1].pub, 0, 1, nil, nil)
	require.NoError(t, header.Sign(validators[1].priv))

	// Bad signatures
	sigs := make([][]byte, 4)
	authors := make([]uint16, 4)
	for i := range sigs {
		sigs[i] = make([]byte, 64) // zeros - invalid
		authors[i] = uint16(i)
	}

	cert := types.NewCertificate(*header, sigs, authors)

	// Process certificate
	p.OnCertificateReceived(cert)

	// Should not be in DAG
	stored := d.GetByDigest(cert.Digest())
	require.Nil(t, stored)
}

func TestOnCertificateReceivedMissingParent(t *testing.T) {
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

	// Create certificate with missing parent
	var fakeParent types.CertificateDigest
	copy(fakeParent[:], []byte("missing parent digest!!"))

	header := types.NewHeader(validators[1].pub, 1, 1, nil, []types.CertificateDigest{fakeParent})
	require.NoError(t, header.Sign(validators[1].priv))

	sigs := make([][]byte, len(validators))
	authors := make([]uint16, len(validators))

	for i, v := range validators {
		vote := types.NewVote(header.Digest(), 1, 1, v.pub)
		require.NoError(t, vote.Sign(v.priv))
		sigs[i] = vote.Signature
		authors[i] = uint16(i)
	}

	cert := types.NewCertificate(*header, sigs, authors)

	// Process certificate
	p.OnCertificateReceived(cert)

	// Should not be in DAG (missing parent)
	stored := d.GetByDigest(cert.Digest())
	require.Nil(t, stored)
}

func TestOnCertificateReceivedNil(t *testing.T) {
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
	p.OnCertificateReceived(nil)
}

func TestOnCertificateReceivedWrongEpoch(t *testing.T) {
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

	// Create certificate with wrong epoch
	header := types.NewHeader(validators[1].pub, 0, 99, nil, nil) // wrong epoch
	require.NoError(t, header.Sign(validators[1].priv))

	sigs := make([][]byte, len(validators))
	authors := make([]uint16, len(validators))

	for i, v := range validators {
		vote := types.NewVote(header.Digest(), 0, 99, v.pub)
		require.NoError(t, vote.Sign(v.priv))
		sigs[i] = vote.Signature
		authors[i] = uint16(i)
	}

	cert := types.NewCertificate(*header, sigs, authors)

	// Process certificate - should fail verification
	p.OnCertificateReceived(cert)

	// Should not be in DAG
	stored := d.GetByDigest(cert.Digest())
	require.Nil(t, stored)
}

func TestTryAdvanceRoundNoQuorum(t *testing.T) {
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

	initialRound := p.CurrentRound()

	// Try advance - should not advance without quorum
	p.tryAdvanceRound()

	require.Equal(t, initialRound, p.CurrentRound())
}

func TestTryAdvanceRoundWithQuorum(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	// Create genesis certs for all validators
	createGenesisCertificates(t, validators, committee, d)

	config := Config{
		Partition: "test",
		KeyPair:   validators[0].priv,
	}

	p := New(config, committee, nil, d, nil)

	require.Equal(t, types.Round(0), p.CurrentRound())

	// Now should advance since we have quorum in round 0
	p.tryAdvanceRound()

	// Give time for async operations
	time.Sleep(50 * time.Millisecond)

	require.Equal(t, types.Round(1), p.CurrentRound())
}

func TestAdvanceRoundForced(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   v.priv,
	}

	p := New(config, committee, nil, d, nil)

	require.Equal(t, types.Round(0), p.CurrentRound())

	p.AdvanceRound()
	require.Equal(t, types.Round(1), p.CurrentRound())

	p.AdvanceRound()
	p.AdvanceRound()
	require.Equal(t, types.Round(3), p.CurrentRound())
}

func TestSetRound(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   v.priv,
	}

	p := New(config, committee, nil, d, nil)

	p.SetRound(100)
	require.Equal(t, types.Round(100), p.CurrentRound())

	p.SetRound(0)
	require.Equal(t, types.Round(0), p.CurrentRound())
}

func TestSetEpoch(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   v.priv,
	}

	p := New(config, committee, nil, d, nil)

	require.Equal(t, uint64(1), p.CurrentEpoch())

	p.SetEpoch(100)
	require.Equal(t, uint64(100), p.CurrentEpoch())
}

func TestRoundAdvancementTriggersNewHeader(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	// Create genesis certs
	createGenesisCertificates(t, validators, committee, d)

	config := Config{
		Partition: "test",
		KeyPair:   validators[0].priv,
	}

	p := New(config, committee, nil, d, nil)

	// Advance round
	p.tryAdvanceRound()

	// Give time for header creation
	time.Sleep(50 * time.Millisecond)

	// Should be in round 1 now
	require.Equal(t, types.Round(1), p.CurrentRound())
}

func TestMultipleCertificatesInRound(t *testing.T) {
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

	// Create certificates for all validators in round 0
	for _, v := range validators {
		header := types.NewHeader(v.pub, 0, 1, nil, nil)
		require.NoError(t, header.Sign(v.priv))

		sigs := make([][]byte, len(validators))
		authors := make([]uint16, len(validators))

		for i, voter := range validators {
			vote := types.NewVote(header.Digest(), 0, 1, voter.pub)
			require.NoError(t, vote.Sign(voter.priv))
			sigs[i] = vote.Signature
			authors[i] = uint16(i)
		}

		cert := types.NewCertificate(*header, sigs, authors)
		p.OnCertificateReceived(cert)
	}

	// Give time for processing
	time.Sleep(50 * time.Millisecond)

	// All 4 certs should be in DAG
	certs := d.GetRound(0)
	require.Len(t, certs, 4)

	// Should have advanced to round 1 since we have quorum
	require.Equal(t, types.Round(1), p.CurrentRound())
}

func TestCleanupOldHeaders(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   v.priv,
	}

	p := New(config, committee, nil, d, nil)

	// Add headers for multiple rounds
	for r := types.Round(0); r < 10; r++ {
		header := types.NewHeader(v.pub, r, 1, nil, nil)
		require.NoError(t, header.Sign(v.priv))

		p.pendingMu.Lock()
		p.ourHeaders[header.Digest()] = header
		p.pendingVotes[header.Digest()] = nil
		p.pendingMu.Unlock()
	}

	require.Equal(t, 10, p.OurHeadersCount())

	// Set round far ahead
	p.SetRound(10)

	// Cleanup
	p.cleanupOldHeaders()

	// Old headers should be cleaned up
	// Only headers from rounds 8+ should remain (within 2 rounds of current)
	p.pendingMu.Lock()
	remaining := len(p.ourHeaders)
	for _, h := range p.ourHeaders {
		require.GreaterOrEqual(t, h.Round, types.Round(8))
	}
	p.pendingMu.Unlock()

	require.LessOrEqual(t, remaining, 3) // rounds 8, 9, (possibly 10)
}

func TestDuplicateCertificateHandling(t *testing.T) {
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

	// Create certificate
	header := types.NewHeader(validators[1].pub, 0, 1, nil, nil)
	require.NoError(t, header.Sign(validators[1].priv))

	sigs := make([][]byte, len(validators))
	authors := make([]uint16, len(validators))

	for i, v := range validators {
		vote := types.NewVote(header.Digest(), 0, 1, v.pub)
		require.NoError(t, vote.Sign(v.priv))
		sigs[i] = vote.Signature
		authors[i] = uint16(i)
	}

	cert := types.NewCertificate(*header, sigs, authors)

	// Process same certificate multiple times
	p.OnCertificateReceived(cert)
	p.OnCertificateReceived(cert)
	p.OnCertificateReceived(cert)

	// Should only be one entry in DAG
	certs := d.GetRound(0)
	require.Len(t, certs, 1)
}

func TestConcurrentCertificateProcessing(t *testing.T) {
	validators := make([]*testValidator, 4)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	committee := newTestCommittee(validators, 1)
	d := dag.NewDAG(10)

	config := Config{
		Partition:           "test",
		KeyPair:             validators[0].priv,
		NewCertsChannelSize: 100,
	}

	p := New(config, committee, nil, d, nil)

	// Process certificates concurrently
	done := make(chan struct{}, len(validators))

	for _, v := range validators {
		go func(validator *testValidator) {
			header := types.NewHeader(validator.pub, 0, 1, nil, nil)
			_ = header.Sign(validator.priv)

			sigs := make([][]byte, len(validators))
			authors := make([]uint16, len(validators))

			for i, voter := range validators {
				vote := types.NewVote(header.Digest(), 0, 1, voter.pub)
				_ = vote.Sign(voter.priv)
				sigs[i] = vote.Signature
				authors[i] = uint16(i)
			}

			cert := types.NewCertificate(*header, sigs, authors)
			p.OnCertificateReceived(cert)
			done <- struct{}{}
		}(v)
	}

	// Wait for all
	for i := 0; i < len(validators); i++ {
		<-done
	}

	// All certs should be in DAG
	certs := d.GetRound(0)
	require.Len(t, certs, 4)
}
