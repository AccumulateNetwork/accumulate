// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// testValidator holds a test validator's keys.
type testValidator struct {
	pub  ed25519.PublicKey
	priv ed25519.PrivateKey
}

// newTestValidator creates a new test validator with random keys.
func newTestValidator(t *testing.T) *testValidator {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return &testValidator{pub: pub, priv: priv}
}

// newTestCommittee creates a committee with the given validators.
func newTestCommittee(validators []*testValidator, epoch uint64) *types.Committee {
	infos := make([]types.ValidatorInfo, len(validators))
	for i, v := range validators {
		infos[i] = types.ValidatorInfo{
			PublicKey: v.pub,
			Stake:     100, // equal stake
		}
	}
	return types.NewCommittee(infos, epoch)
}

// newTestDAG creates a new DAG for testing.
func newTestDAG() *dag.DAG {
	return dag.NewDAG(10)
}

// createGenesisCertificates creates round 0 certificates for all validators.
func createGenesisCertificates(t *testing.T, validators []*testValidator, committee *types.Committee, d *dag.DAG) []*types.Certificate {
	certs := make([]*types.Certificate, len(validators))

	for i, v := range validators {
		// Create header
		header := types.NewHeader(v.pub, 0, committee.Epoch, nil, nil)
		require.NoError(t, header.Sign(v.priv))

		// Create signatures from all validators
		sigs := make([][]byte, len(validators))
		authors := make([]uint16, len(validators))

		headerDigest := header.Digest()
		for j, voter := range validators {
			vote := types.NewVote(headerDigest, 0, committee.Epoch, voter.pub)
			require.NoError(t, vote.Sign(voter.priv))
			sigs[j] = vote.Signature
			authors[j] = uint16(j)
		}

		cert := types.NewCertificate(header, sigs, authors)
		require.NoError(t, d.InsertGenesis(cert))
		certs[i] = cert
	}

	return certs
}

func TestPrimaryNew(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   v.priv,
	}

	p := New(config, committee, nil, d, nil)

	require.NotNil(t, p)
	require.Equal(t, types.Round(0), p.CurrentRound())
	require.Equal(t, uint64(1), p.CurrentEpoch())
	require.Equal(t, v.pub, ed25519.PublicKey(p.PublicKey()))
}

func TestPrimaryConfig(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	t.Run("defaults applied", func(t *testing.T) {
		config := Config{
			Partition: "test",
			KeyPair:   v.priv,
		}

		p := New(config, committee, nil, d, nil)
		require.NotNil(t, p)

		// Check defaults were applied
		require.Equal(t, DefaultRoundAdvanceInterval, p.config.RoundAdvanceInterval)
		require.Equal(t, DefaultNewCertsChannelSize, p.config.NewCertsChannelSize)
	})

	t.Run("custom values", func(t *testing.T) {
		config := Config{
			Partition:            "test",
			KeyPair:              v.priv,
			RoundAdvanceInterval: 100 * time.Millisecond,
			NewCertsChannelSize:  50,
		}

		p := New(config, committee, nil, d, nil)
		require.NotNil(t, p)

		require.Equal(t, 100*time.Millisecond, p.config.RoundAdvanceInterval)
		require.Equal(t, 50, p.config.NewCertsChannelSize)
	})
}

func TestPrimaryCurrentRoundEpoch(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 5)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   v.priv,
	}

	p := New(config, committee, nil, d, nil)

	require.Equal(t, types.Round(0), p.CurrentRound())
	require.Equal(t, uint64(5), p.CurrentEpoch())

	// Set round and epoch
	p.SetRound(10)
	p.SetEpoch(7)

	require.Equal(t, types.Round(10), p.CurrentRound())
	require.Equal(t, uint64(7), p.CurrentEpoch())
}

func TestPrimaryPublicKey(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   v.priv,
	}

	p := New(config, committee, nil, d, nil)

	pubKey := p.PublicKey()
	require.Equal(t, v.pub, pubKey)
}

func TestPrimaryStop(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   v.priv,
	}

	p := New(config, committee, nil, d, nil)

	// Stop should not panic even without Start
	p.Stop()

	// Multiple stops should be safe
	p.Stop()
	p.Stop()
}

func TestPrimaryNewCertificates(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition:           "test",
		KeyPair:             v.priv,
		NewCertsChannelSize: 5,
	}

	p := New(config, committee, nil, d, nil)

	ch := p.NewCertificates()
	require.NotNil(t, ch)

	// Test signaling
	header := types.NewHeader(v.pub, 0, 1, nil, nil)
	require.NoError(t, header.Sign(v.priv))

	cert := types.NewCertificate(header, nil, nil)

	p.signalNewCertificate(cert)

	select {
	case received := <-ch:
		require.Equal(t, cert.Digest(), received.Digest())
	case <-time.After(100 * time.Millisecond):
		t.Fatal("did not receive certificate")
	}
}

func TestPrimaryMetrics(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   v.priv,
	}

	p := New(config, committee, nil, d, nil)

	headers, certs, votesRecv, votesSent := p.Metrics()
	require.Equal(t, uint64(0), headers)
	require.Equal(t, uint64(0), certs)
	require.Equal(t, uint64(0), votesRecv)
	require.Equal(t, uint64(0), votesSent)

	// Manually increment
	p.headersCreated.Add(5)
	p.certificatesCreated.Add(3)
	p.votesReceived.Add(10)
	p.votesSent.Add(7)

	headers, certs, votesRecv, votesSent = p.Metrics()
	require.Equal(t, uint64(5), headers)
	require.Equal(t, uint64(3), certs)
	require.Equal(t, uint64(10), votesRecv)
	require.Equal(t, uint64(7), votesSent)
}

func TestPrimaryUpdateCommittee(t *testing.T) {
	v1 := newTestValidator(t)
	v2 := newTestValidator(t)

	validators1 := []*testValidator{v1}
	validators2 := []*testValidator{v1, v2}

	committee1 := newTestCommittee(validators1, 1)
	committee2 := newTestCommittee(validators2, 2)

	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   v1.priv,
	}

	p := New(config, committee1, nil, d, nil)

	require.Equal(t, uint64(1), p.CurrentEpoch())
	require.Equal(t, 1, p.committee.Len())

	p.UpdateCommittee(committee2)

	require.Equal(t, uint64(2), p.CurrentEpoch())
	require.Equal(t, 2, p.committee.Len())
}

func TestPrimaryAdvanceRound(t *testing.T) {
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
	require.Equal(t, types.Round(2), p.CurrentRound())
}

func TestPrimaryOurHeadersTracking(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   v.priv,
	}

	p := New(config, committee, nil, d, nil)

	require.Equal(t, 0, p.OurHeadersCount())

	// Add a header manually
	header := types.NewHeader(v.pub, 0, 1, nil, nil)
	require.NoError(t, header.Sign(v.priv))

	p.pendingMu.Lock()
	p.ourHeaders[header.Digest()] = header
	p.pendingVotes[header.Digest()] = nil
	p.pendingMu.Unlock()

	require.Equal(t, 1, p.OurHeadersCount())
	require.Equal(t, 0, p.PendingVoteCount(header.Digest()))
}

func TestPrimaryHasCertificateForRound(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   v.priv,
	}

	p := New(config, committee, nil, d, nil)

	require.False(t, p.HasCertificateForRound(0))

	// Add a certificate
	header := types.NewHeader(v.pub, 0, 1, nil, nil)
	require.NoError(t, header.Sign(v.priv))

	cert := types.NewCertificate(header, nil, nil)

	p.pendingMu.Lock()
	p.ourCerts[0] = cert
	p.pendingMu.Unlock()

	require.True(t, p.HasCertificateForRound(0))
	require.False(t, p.HasCertificateForRound(1))

	retrieved := p.GetOurCertificate(0)
	require.NotNil(t, retrieved)
	require.Equal(t, cert.Digest(), retrieved.Digest())
}

func TestPrimaryStartStop(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	// Create genesis certs so we can advance rounds
	createGenesisCertificates(t, validators, committee, d)

	config := Config{
		Partition:            "test",
		KeyPair:              v.priv,
		RoundAdvanceInterval: 10 * time.Millisecond,
	}

	// We need a mock gossip layer, but for this test we'll just verify
	// that Start returns properly when context is canceled
	p := New(config, committee, nil, d, nil)

	ctx, cancel := context.WithCancel(context.Background())

	// Start in background
	done := make(chan error)
	go func() {
		done <- p.Start(ctx)
	}()

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	// Stop
	cancel()

	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("Start did not return after cancel")
	}
}

func TestPrimaryStartWhenClosed(t *testing.T) {
	v := newTestValidator(t)
	validators := []*testValidator{v}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   v.priv,
	}

	p := New(config, committee, nil, d, nil)

	// Close first
	p.closed.Store(true)

	err := p.Start(context.Background())
	require.ErrorIs(t, err, ErrPrimaryClosed)
}
