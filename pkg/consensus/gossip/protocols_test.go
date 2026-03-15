// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package gossip_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	bhost "github.com/libp2p/go-libp2p/p2p/host/blank"
	swarmt "github.com/libp2p/go-libp2p/p2p/net/swarm/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/gossip"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// mockBatchStore is a simple in-memory batch store for testing.
type mockBatchStore struct {
	mu      sync.RWMutex
	batches map[types.BatchDigest]*types.Batch
}

func newMockBatchStore() *mockBatchStore {
	return &mockBatchStore{
		batches: make(map[types.BatchDigest]*types.Batch),
	}
}

func (s *mockBatchStore) GetBatch(digest types.BatchDigest) (*types.Batch, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.batches[digest], nil
}

func (s *mockBatchStore) StoreBatch(batch *types.Batch) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.batches[batch.Digest()] = batch
	return nil
}

// mockDAGStore is a simple in-memory DAG store for testing.
type mockDAGStore struct {
	mu    sync.RWMutex
	certs map[types.Round]map[string]*types.Certificate // round -> author hex -> cert
}

func newMockDAGStore() *mockDAGStore {
	return &mockDAGStore{
		certs: make(map[types.Round]map[string]*types.Certificate),
	}
}

func (s *mockDAGStore) GetCertificate(round types.Round, author []byte) (*types.Certificate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	roundCerts, ok := s.certs[round]
	if !ok {
		return nil, nil
	}
	return roundCerts[string(author)], nil
}

func (s *mockDAGStore) GetRound(round types.Round) ([]*types.Certificate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	roundCerts, ok := s.certs[round]
	if !ok {
		return nil, nil
	}

	result := make([]*types.Certificate, 0, len(roundCerts))
	for _, cert := range roundCerts {
		result = append(result, cert)
	}
	return result, nil
}

func (s *mockDAGStore) StoreCertificate(cert *types.Certificate) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	round := cert.Round()
	if _, ok := s.certs[round]; !ok {
		s.certs[round] = make(map[string]*types.Certificate)
	}
	s.certs[round][string(cert.Author())] = cert
	return nil
}

// protocolTestNetwork holds hosts for protocol testing.
type protocolTestNetwork struct {
	t     *testing.T
	ctx   context.Context
	hosts []host.Host
}

func newProtocolTestNetwork(t *testing.T, ctx context.Context, n int) *protocolTestNetwork {
	t.Helper()

	ptn := &protocolTestNetwork{
		t:     t,
		ctx:   ctx,
		hosts: make([]host.Host, n),
	}

	for i := 0; i < n; i++ {
		netw := swarmt.GenSwarm(t)
		h := bhost.NewBlankHost(netw)
		ptn.hosts[i] = h
		t.Cleanup(func() { h.Close() })
	}

	// Divulge addresses between all hosts to allow dialing
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			swarmt.DivulgeAddresses(ptn.hosts[i].Network(), ptn.hosts[j].Network())
			// Connect hosts so they have peer info
			err := ptn.hosts[i].Connect(ctx, ptn.hosts[j].Peerstore().PeerInfo(ptn.hosts[j].ID()))
			require.NoError(t, err)
		}
	}

	return ptn
}

func TestProtocolHandler_NewProtocolHandler(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ptn := newProtocolTestNetwork(t, ctx, 1)

	t.Run("success", func(t *testing.T) {
		ph, err := gossip.NewProtocolHandler(ptn.hosts[0], nil, nil)
		require.NoError(t, err)
		assert.NotNil(t, ph)
	})

	t.Run("nil host", func(t *testing.T) {
		_, err := gossip.NewProtocolHandler(nil, nil, nil)
		assert.Error(t, err)
	})
}

func TestProtocolHandler_BatchFetch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ptn := newProtocolTestNetwork(t, ctx, 2)

	// Set up server with batch store
	batchStore := newMockBatchStore()
	server, err := gossip.NewProtocolHandler(ptn.hosts[0], batchStore, nil)
	require.NoError(t, err)
	err = server.RegisterHandlers()
	require.NoError(t, err)

	// Set up client
	client, err := gossip.NewProtocolHandler(ptn.hosts[1], nil, nil)
	require.NoError(t, err)

	// Store a batch on the server
	batch := types.NewBatch([][]byte{
		[]byte("transaction 1"),
		[]byte("transaction 2"),
	})
	err = batchStore.StoreBatch(batch)
	require.NoError(t, err)

	t.Run("fetch existing batch", func(t *testing.T) {
		fetched, err := client.FetchBatch(ctx, ptn.hosts[0].ID(), batch.Digest())
		require.NoError(t, err)
		require.NotNil(t, fetched)
		assert.Equal(t, batch.Digest(), fetched.Digest())
		assert.Equal(t, len(batch.Transactions), len(fetched.Transactions))
	})

	t.Run("fetch non-existent batch", func(t *testing.T) {
		var nonExistent types.BatchDigest
		rand.Read(nonExistent[:])

		fetched, err := client.FetchBatch(ctx, ptn.hosts[0].ID(), nonExistent)
		require.NoError(t, err)
		assert.Nil(t, fetched)
	})

	server.UnregisterHandlers()
}

func TestProtocolHandler_CertFetch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ptn := newProtocolTestNetwork(t, ctx, 2)

	// Set up server with DAG store
	dagStore := newMockDAGStore()
	server, err := gossip.NewProtocolHandler(ptn.hosts[0], nil, dagStore)
	require.NoError(t, err)
	err = server.RegisterHandlers()
	require.NoError(t, err)

	// Set up client
	client, err := gossip.NewProtocolHandler(ptn.hosts[1], nil, nil)
	require.NoError(t, err)

	// Create and store a certificate
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	header := types.NewHeader(pub, 5, 1, nil, nil)
	err = header.Sign(priv)
	require.NoError(t, err)

	cert := types.NewCertificate(*header, [][]byte{header.Signature}, []uint16{0})
	err = dagStore.StoreCertificate(cert)
	require.NoError(t, err)

	t.Run("fetch existing certificate", func(t *testing.T) {
		fetched, err := client.FetchCertificate(ctx, ptn.hosts[0].ID(), cert.Round(), pub)
		require.NoError(t, err)
		require.NotNil(t, fetched)
		assert.Equal(t, cert.Digest(), fetched.Digest())
		assert.Equal(t, cert.Round(), fetched.Round())
	})

	t.Run("fetch non-existent certificate", func(t *testing.T) {
		randomPub := make([]byte, 32)
		rand.Read(randomPub)

		fetched, err := client.FetchCertificate(ctx, ptn.hosts[0].ID(), 999, randomPub)
		require.NoError(t, err)
		assert.Nil(t, fetched)
	})

	server.UnregisterHandlers()
}

func TestProtocolHandler_DAGSync(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ptn := newProtocolTestNetwork(t, ctx, 2)

	// Set up server with DAG store
	dagStore := newMockDAGStore()
	server, err := gossip.NewProtocolHandler(ptn.hosts[0], nil, dagStore)
	require.NoError(t, err)
	err = server.RegisterHandlers()
	require.NoError(t, err)

	// Set up client
	client, err := gossip.NewProtocolHandler(ptn.hosts[1], nil, nil)
	require.NoError(t, err)

	// Create and store multiple certificates across rounds
	var expectedCerts []*types.Certificate
	for round := types.Round(1); round <= 5; round++ {
		for i := 0; i < 3; i++ {
			pub, priv, err := ed25519.GenerateKey(rand.Reader)
			require.NoError(t, err)

			header := types.NewHeader(pub, round, 1, nil, nil)
			err = header.Sign(priv)
			require.NoError(t, err)

			cert := types.NewCertificate(*header, [][]byte{header.Signature}, []uint16{0})
			err = dagStore.StoreCertificate(cert)
			require.NoError(t, err)
			expectedCerts = append(expectedCerts, cert)
		}
	}

	t.Run("sync all rounds", func(t *testing.T) {
		certs, err := client.SyncDAG(ctx, ptn.hosts[0].ID(), 1, 5)
		require.NoError(t, err)
		require.NotNil(t, certs)
		assert.Equal(t, 15, len(certs)) // 5 rounds * 3 certs each
	})

	t.Run("sync subset of rounds", func(t *testing.T) {
		certs, err := client.SyncDAG(ctx, ptn.hosts[0].ID(), 2, 4)
		require.NoError(t, err)
		require.NotNil(t, certs)
		assert.Equal(t, 9, len(certs)) // 3 rounds * 3 certs each
	})

	t.Run("sync non-existent rounds", func(t *testing.T) {
		certs, err := client.SyncDAG(ctx, ptn.hosts[0].ID(), 100, 105)
		require.NoError(t, err)
		assert.Empty(t, certs) // May be nil or empty slice
	})

	server.UnregisterHandlers()
}

func TestBatchRequest_Marshal(t *testing.T) {
	var digest types.BatchDigest
	rand.Read(digest[:])

	req := &gossip.BatchRequest{Digest: digest}
	data := req.Marshal()
	assert.Equal(t, 32, len(data))

	// Unmarshal
	req2, err := gossip.UnmarshalBatchRequest(data)
	require.NoError(t, err)
	assert.Equal(t, digest, req2.Digest)
}

func TestBatchRequest_UnmarshalErrors(t *testing.T) {
	t.Run("too short", func(t *testing.T) {
		_, err := gossip.UnmarshalBatchRequest([]byte{1, 2, 3})
		assert.Error(t, err)
	})

	t.Run("too long", func(t *testing.T) {
		data := make([]byte, 33)
		_, err := gossip.UnmarshalBatchRequest(data)
		assert.Error(t, err)
	})
}

func TestCertRequest_Marshal(t *testing.T) {
	author := make([]byte, 32)
	rand.Read(author)

	req := &gossip.CertRequest{
		Round:  42,
		Author: author,
	}
	data := req.Marshal()
	assert.Equal(t, 40, len(data)) // 8 + 32

	// Unmarshal
	req2, err := gossip.UnmarshalCertRequest(data)
	require.NoError(t, err)
	assert.Equal(t, types.Round(42), req2.Round)
	assert.Equal(t, author, req2.Author)
}

func TestCertRequest_UnmarshalErrors(t *testing.T) {
	t.Run("too short", func(t *testing.T) {
		_, err := gossip.UnmarshalCertRequest([]byte{1, 2, 3})
		assert.Error(t, err)
	})
}

func TestDAGSyncRequest_Marshal(t *testing.T) {
	req := &gossip.DAGSyncRequest{
		FromRound: 10,
		ToRound:   20,
	}
	data := req.Marshal()
	assert.Equal(t, 16, len(data))

	// Unmarshal
	req2, err := gossip.UnmarshalDAGSyncRequest(data)
	require.NoError(t, err)
	assert.Equal(t, types.Round(10), req2.FromRound)
	assert.Equal(t, types.Round(20), req2.ToRound)
}

func TestDAGSyncRequest_UnmarshalErrors(t *testing.T) {
	t.Run("too short", func(t *testing.T) {
		_, err := gossip.UnmarshalDAGSyncRequest([]byte{1, 2, 3})
		assert.Error(t, err)
	})

	t.Run("too long", func(t *testing.T) {
		data := make([]byte, 17)
		_, err := gossip.UnmarshalDAGSyncRequest(data)
		assert.Error(t, err)
	})
}

func TestProtocolHandler_ConcurrentRequests(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	ptn := newProtocolTestNetwork(t, ctx, 2)

	// Set up server
	batchStore := newMockBatchStore()
	server, err := gossip.NewProtocolHandler(ptn.hosts[0], batchStore, nil)
	require.NoError(t, err)
	err = server.RegisterHandlers()
	require.NoError(t, err)
	defer server.UnregisterHandlers()

	// Store multiple batches
	batches := make([]*types.Batch, 10)
	for i := 0; i < 10; i++ {
		batches[i] = types.NewBatch([][]byte{[]byte("batch " + string(rune('0'+i)))})
		err = batchStore.StoreBatch(batches[i])
		require.NoError(t, err)
	}

	// Set up client
	client, err := gossip.NewProtocolHandler(ptn.hosts[1], nil, nil)
	require.NoError(t, err)

	// Fetch all batches concurrently
	var wg sync.WaitGroup
	results := make([]*types.Batch, 10)
	errors := make([]error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errors[idx] = client.FetchBatch(ctx, ptn.hosts[0].ID(), batches[idx].Digest())
		}(i)
	}

	wg.Wait()

	// Verify all fetches succeeded
	for i := 0; i < 10; i++ {
		require.NoError(t, errors[i], "fetch %d failed", i)
		require.NotNil(t, results[i], "fetch %d returned nil", i)
		assert.Equal(t, batches[i].Digest(), results[i].Digest())
	}
}
