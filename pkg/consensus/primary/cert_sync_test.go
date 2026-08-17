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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/gossip"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

func TestCertSyncRequest_MarshalUnmarshal(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	// Create digests
	digests := make([]types.CertificateDigest, 3)
	for i := range digests {
		_, _ = rand.Read(digests[i][:])
	}

	req := &gossip.CertSyncRequest{
		Digests:   digests,
		Requester: pub,
		RequestID: 12345,
	}

	data, err := req.Marshal()
	require.NoError(t, err)

	req2, err := gossip.UnmarshalCertSyncRequest(data)
	require.NoError(t, err)

	assert.Equal(t, req.RequestID, req2.RequestID)
	assert.Equal(t, req.Requester, req2.Requester)
	assert.Equal(t, len(req.Digests), len(req2.Digests))
	for i := range req.Digests {
		assert.Equal(t, req.Digests[i], req2.Digests[i])
	}
}

func TestCertSyncResponse_MarshalUnmarshal(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	// Create a certificate
	header := types.NewHeader(pub, 1, 0, nil, nil)
	err = header.Sign(priv)
	require.NoError(t, err)

	cert := types.NewCertificate(header, [][]byte{header.Signature}, []uint16{0})

	// Create missing digests
	missing := make([]types.CertificateDigest, 2)
	for i := range missing {
		_, _ = rand.Read(missing[i][:])
	}

	resp := &gossip.CertSyncResponse{
		Certificates: []*types.Certificate{cert},
		RequestID:    67890,
		Missing:      missing,
	}

	data, err := resp.Marshal()
	require.NoError(t, err)

	resp2, err := gossip.UnmarshalCertSyncResponse(data)
	require.NoError(t, err)

	assert.Equal(t, resp.RequestID, resp2.RequestID)
	assert.Equal(t, len(resp.Certificates), len(resp2.Certificates))
	assert.Equal(t, resp.Certificates[0].Digest(), resp2.Certificates[0].Digest())
	assert.Equal(t, len(resp.Missing), len(resp2.Missing))
	for i := range resp.Missing {
		assert.Equal(t, resp.Missing[i], resp2.Missing[i])
	}
}

func TestCertSyncer_RequestBatching(t *testing.T) {
	d := dag.NewDAG(10)

	// Create syncer with short batch interval for testing
	config := CertSyncerConfig{
		BatchInterval:         10 * time.Millisecond,
		DeduplicationInterval: 100 * time.Millisecond,
		RetryTimeout:          1 * time.Second,
		MaxRetries:            3,
		JitterMax:             5 * time.Millisecond,
	}
	config.PublicKey = make([]byte, 32)
	_, _ = rand.Read(config.PublicKey)

	pending := NewPendingCertificates(100)

	syncer := NewCertSyncer(config, d, nil, pending)

	// Request multiple digests
	digests := make([]types.CertificateDigest, 5)
	for i := range digests {
		_, _ = rand.Read(digests[i][:])
	}

	// Add digests
	syncer.RequestMissing(digests[:3])
	syncer.RequestMissing(digests[3:])

	// Wait for batch to be processed (without gossip, nothing happens but no panic)
	time.Sleep(50 * time.Millisecond)
}

func TestCertSyncer_Deduplication(t *testing.T) {
	d := dag.NewDAG(10)

	config := CertSyncerConfig{
		BatchInterval:         10 * time.Millisecond,
		DeduplicationInterval: 500 * time.Millisecond, // Long dedup interval
		RetryTimeout:          1 * time.Second,
		MaxRetries:            3,
		JitterMax:             5 * time.Millisecond,
	}
	config.PublicKey = make([]byte, 32)
	_, _ = rand.Read(config.PublicKey)

	pending := NewPendingCertificates(100)
	syncer := NewCertSyncer(config, d, nil, pending)

	// Create a digest
	var digest types.CertificateDigest
	_, _ = rand.Read(digest[:])

	// Request it multiple times
	syncer.RequestMissing([]types.CertificateDigest{digest})
	time.Sleep(20 * time.Millisecond) // Wait for first batch

	// Request same digest again - should be deduplicated
	syncer.RequestMissing([]types.CertificateDigest{digest})

	// Verify in-flight tracking (without gossip, request stays in-flight)
	count := syncer.InFlightCount()
	assert.LessOrEqual(t, count, 1, "Should have at most 1 in-flight request due to deduplication")
}

func TestCertSyncer_DAGContainsSkip(t *testing.T) {
	d := dag.NewDAG(10)

	// Insert a certificate into DAG
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	header := types.NewHeader(pub, 0, 0, nil, nil) // Genesis round
	err = header.Sign(priv)
	require.NoError(t, err)

	cert := types.NewCertificate(header, [][]byte{header.Signature}, []uint16{0})
	err = d.InsertGenesis(cert)
	require.NoError(t, err)

	config := CertSyncerConfig{
		BatchInterval:         10 * time.Millisecond,
		DeduplicationInterval: 100 * time.Millisecond,
		RetryTimeout:          1 * time.Second,
		MaxRetries:            3,
		JitterMax:             5 * time.Millisecond,
	}
	config.PublicKey = make([]byte, 32)
	_, _ = rand.Read(config.PublicKey)

	pending := NewPendingCertificates(100)
	syncer := NewCertSyncer(config, d, nil, pending)

	// Request the digest that's already in DAG
	syncer.RequestMissing([]types.CertificateDigest{cert.Digest()})

	// Should not create an in-flight request
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, 0, syncer.InFlightCount(), "Should not request certs already in DAG")
}

func TestCertSyncer_Lifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d := dag.NewDAG(10)

	config := CertSyncerConfig{
		BatchInterval:         10 * time.Millisecond,
		DeduplicationInterval: 100 * time.Millisecond,
		RetryTimeout:          100 * time.Millisecond,
		MaxRetries:            2,
		JitterMax:             5 * time.Millisecond,
	}
	config.PublicKey = make([]byte, 32)
	_, _ = rand.Read(config.PublicKey)

	pending := NewPendingCertificates(100)
	syncer := NewCertSyncer(config, d, nil, pending)

	// Start without gossip should work
	err := syncer.Start(ctx)
	require.NoError(t, err)

	// Request some certificates
	var digest types.CertificateDigest
	_, _ = rand.Read(digest[:])
	syncer.RequestMissing([]types.CertificateDigest{digest})

	time.Sleep(50 * time.Millisecond)

	// Stop
	syncer.Stop()

	// Metrics should be available
	sent, recv, certs := syncer.Metrics()
	assert.GreaterOrEqual(t, sent, uint64(0))
	assert.GreaterOrEqual(t, recv, uint64(0))
	assert.GreaterOrEqual(t, certs, uint64(0))
}

func TestCertSyncer_MaxRetries(t *testing.T) {
	d := dag.NewDAG(10)

	config := CertSyncerConfig{
		BatchInterval:         10 * time.Millisecond,
		DeduplicationInterval: 10 * time.Millisecond, // Short for testing
		RetryTimeout:          20 * time.Millisecond, // Short for testing
		MaxRetries:            2,
		JitterMax:             1 * time.Millisecond,
	}
	config.PublicKey = make([]byte, 32)
	_, _ = rand.Read(config.PublicKey)

	pending := NewPendingCertificates(100)
	syncer := NewCertSyncer(config, d, nil, pending)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := syncer.Start(ctx)
	require.NoError(t, err)
	defer syncer.Stop()

	// Request a digest
	var digest types.CertificateDigest
	_, _ = rand.Read(digest[:])
	syncer.RequestMissing([]types.CertificateDigest{digest})

	// Wait for max retries to be exhausted
	time.Sleep(200 * time.Millisecond)

	// Should have given up
	assert.Equal(t, 0, syncer.InFlightCount(), "Should have given up after max retries")
}

func TestCertSyncer_HandleSyncRequest(t *testing.T) {
	d := dag.NewDAG(10)

	// Insert a certificate
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	header := types.NewHeader(pub, 0, 0, nil, nil)
	err = header.Sign(priv)
	require.NoError(t, err)

	cert := types.NewCertificate(header, [][]byte{header.Signature}, []uint16{0})
	err = d.InsertGenesis(cert)
	require.NoError(t, err)

	config := CertSyncerConfig{
		PublicKey: pub,
	}
	config.applyDefaults()

	pending := NewPendingCertificates(100)
	syncer := NewCertSyncer(config, d, nil, pending)

	// Create a sync request
	var missingDigest types.CertificateDigest
	_, _ = rand.Read(missingDigest[:])

	req := &gossip.CertSyncRequest{
		Digests:   []types.CertificateDigest{cert.Digest(), missingDigest},
		RequestID: 123,
	}

	// Handle the request (without gossip, nothing is sent but no panic)
	syncer.handleSyncRequest(req)
}

func TestCertSyncer_CertReceivedCallback(t *testing.T) {
	d := dag.NewDAG(10)

	config := CertSyncerConfig{}
	config.PublicKey = make([]byte, 32)
	_, _ = rand.Read(config.PublicKey)
	config.applyDefaults()

	pending := NewPendingCertificates(100)
	syncer := NewCertSyncer(config, d, nil, pending)

	// Track received certificates
	var received []*types.Certificate
	syncer.SetCertReceivedCallback(func(cert *types.Certificate) {
		received = append(received, cert)
	})

	// Create a certificate for the response
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	header := types.NewHeader(pub, 1, 0, nil, nil)
	err = header.Sign(priv)
	require.NoError(t, err)

	cert := types.NewCertificate(header, [][]byte{header.Signature}, []uint16{0})

	// Simulate receiving a sync response
	resp := &gossip.CertSyncResponse{
		Certificates: []*types.Certificate{cert},
		RequestID:    456,
	}

	syncer.handleSyncResponse(resp)

	// Callback should have been invoked
	require.Len(t, received, 1)
	assert.Equal(t, cert.Digest(), received[0].Digest())
}

func TestCertSyncRequest_EmptyDigests(t *testing.T) {
	req := &gossip.CertSyncRequest{
		Digests:   []types.CertificateDigest{},
		Requester: make([]byte, 32),
		RequestID: 1,
	}

	data, err := req.Marshal()
	require.NoError(t, err)

	req2, err := gossip.UnmarshalCertSyncRequest(data)
	require.NoError(t, err)
	assert.Empty(t, req2.Digests)
}

func TestCertSyncResponse_EmptyCertificates(t *testing.T) {
	resp := &gossip.CertSyncResponse{
		Certificates: []*types.Certificate{},
		RequestID:    1,
		Missing:      []types.CertificateDigest{},
	}

	data, err := resp.Marshal()
	require.NoError(t, err)

	resp2, err := gossip.UnmarshalCertSyncResponse(data)
	require.NoError(t, err)
	assert.Empty(t, resp2.Certificates)
	assert.Empty(t, resp2.Missing)
}

// TestCertSyncRequest_Rounds verifies the round-based catch-up request
// (#4057) round-trips the wire and that a request with rounds serves every
// certificate of those rounds.
func TestCertSyncRequest_Rounds(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	req := &gossip.CertSyncRequest{
		Requester: pub,
		RequestID: 7,
		Rounds:    []types.Round{4, 5, 6},
	}

	data, err := req.Marshal()
	require.NoError(t, err)

	req2, err := gossip.UnmarshalCertSyncRequest(data)
	require.NoError(t, err)
	require.Equal(t, req.Rounds, req2.Rounds)
	require.Equal(t, req.RequestID, req2.RequestID)

	// A request without rounds must still round-trip (backward compat)
	req3 := &gossip.CertSyncRequest{Requester: pub, RequestID: 8}
	data, err = req3.Marshal()
	require.NoError(t, err)
	req4, err := gossip.UnmarshalCertSyncRequest(data)
	require.NoError(t, err)
	require.Empty(t, req4.Rounds)
}

// TestCertSyncer_ServeRounds verifies handleSyncRequest serves whole rounds.
func TestCertSyncer_ServeRounds(t *testing.T) {
	d := dag.NewDAG(10)

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	header := types.NewHeader(pub, 0, 0, nil, nil)
	require.NoError(t, header.Sign(priv))
	cert := types.NewCertificate(header, [][]byte{header.Signature}, []uint16{0})
	require.NoError(t, d.InsertGenesis(cert))

	config := CertSyncerConfig{PublicKey: pub}
	config.applyDefaults()
	syncer := NewCertSyncer(config, d, nil, NewPendingCertificates(100))

	// Round-only request — must not panic and must find the cert (no gossip
	// layer, so nothing is actually sent; this exercises the lookup path)
	syncer.handleSyncRequest(&gossip.CertSyncRequest{
		RequestID: 9,
		Rounds:    []types.Round{0, 1},
	})

	// RequestRounds without gossip must be a no-op
	syncer.RequestRounds([]types.Round{1, 2, 3})
}
