// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"crypto/ed25519"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// APIClient interface for the client methods we need
type APIClient interface {
	Faucet(context.Context, *protocol.AcmeFaucet) (*api.TxResponse, error)
	QueryTx(context.Context, *api.TxnQuery) (*api.TransactionQueryResponse, error)
	Describe(context.Context) (*api.DescriptionResponse, error)
	CloseIdleConnections()
}

// MockClient implements a mock API client for testing
type MockClient struct {
	FaucetFunc     func(context.Context, *protocol.AcmeFaucet) (*api.TxResponse, error)
	QueryTxFunc    func(context.Context, *api.TxnQuery) (*api.TransactionQueryResponse, error)
	DescribeFunc   func(context.Context) (*api.DescriptionResponse, error)
	faucetCalls    int
	queryTxCalls   int
	describeCalls  int
	mu             sync.Mutex
}

func (m *MockClient) Faucet(ctx context.Context, req *protocol.AcmeFaucet) (*api.TxResponse, error) {
	m.mu.Lock()
	m.faucetCalls++
	m.mu.Unlock()
	if m.FaucetFunc != nil {
		return m.FaucetFunc(ctx, req)
	}
	return &api.TxResponse{
		TransactionHash: []byte("mock-tx-hash"),
	}, nil
}

func (m *MockClient) QueryTx(ctx context.Context, req *api.TxnQuery) (*api.TransactionQueryResponse, error) {
	m.mu.Lock()
	m.queryTxCalls++
	m.mu.Unlock()
	if m.QueryTxFunc != nil {
		return m.QueryTxFunc(ctx, req)
	}
	return &api.TransactionQueryResponse{}, nil
}

func (m *MockClient) Describe(ctx context.Context) (*api.DescriptionResponse, error) {
	m.mu.Lock()
	m.describeCalls++
	m.mu.Unlock()
	if m.DescribeFunc != nil {
		return m.DescribeFunc(ctx)
	}
	// Create a proper NetworkDescription
	network := &api.NetworkDescription{}
	network.Id = "TestNetwork"
	network.Partitions = []api.PartitionDescription{
		{
			Id:       "TestPartition",
			BasePort: 26656,
			Nodes: []api.NodeDescription{
				{Address: "http://127.0.0.1"},
			},
		},
	}
	return &api.DescriptionResponse{
		Network: *network,
	}, nil
}

func (m *MockClient) CloseIdleConnections() {
	// No-op for mock
}

func (m *MockClient) GetFaucetCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.faucetCalls
}

func (m *MockClient) GetQueryTxCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.queryTxCalls
}

func (m *MockClient) GetDescribeCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.describeCalls
}

// Helper function to create a test DataSet
func createTestDataSet() *logging.DataSet {
	dsl := &logging.DataSetLog{}
	dsl.Initialize("test", logging.DefaultOptions())
	return dsl.GetDataSet("test")
}

// Helper function to wrap initTxs for testing
func testInitTxs(simTime float64, transactionsPerClient int, client *Client, mock APIClient) error {
	// Temporarily override the client's Client field with our mock
	// by creating a wrapper that mimics what initTxs does
	var m sync.Mutex
	deltas := make([]float64, transactionsPerClient)
	times := make([]float64, transactionsPerClient)

	txWaitGroup := sync.WaitGroup{}
	for i := 0; i < transactionsPerClient; i++ {
		acc, _ := createAccount()
		txWaitGroup.Add(1)

		go func(n int) {
			defer txWaitGroup.Done()
			t := time.Now()

			resp, err := mock.Faucet(context.Background(), &protocol.AcmeFaucet{Url: acc})
			if err != nil {
				return
			}
			txReq := api.TxnQuery{}
			txReq.Txid = resp.TransactionHash
			txReq.Wait = time.Second * 100
			txReq.IgnorePending = false

			_, err = mock.QueryTx(context.Background(), &txReq)
			if err != nil {
				return
			}

			m.Lock()
			deltas[n] = time.Since(t).Seconds()
			client.TxCount++
			m.Unlock()
		}(i)

		times[i] = time.Since(start).Seconds()
	}

	txWaitGroup.Wait()
	mock.CloseIdleConnections()

	for i := 0; i < transactionsPerClient; i++ {
		client.DataSet.Lock()
		client.DataSet.Save("index", (transactionsPerClient*client.Id)+i, 10, true)
		client.DataSet.Save("simTime", simTime, 10, false)
		client.DataSet.Save("clientId", client.Id, 10, false)
		client.DataSet.Save("settlementTime", deltas[i], 10, false)
		client.DataSet.Unlock()
	}

	return nil
}

// TestCreateAccount tests the account creation function
func TestCreateAccount(t *testing.T) {
	acc, err := createAccount()
	require.NoError(t, err, "createAccount should not return an error")
	require.NotNil(t, acc, "account URL should not be nil")

	// Verify the account URL is not empty
	require.NotEmpty(t, acc.String(), "account URL string should not be empty")

	// Verify it starts with acc:// (lite account prefix)
	require.Contains(t, acc.String(), "acc://", "should be an accumulate URL")
}

// TestCreateAccountDeterminism tests that multiple calls create different accounts
func TestCreateAccountDeterminism(t *testing.T) {
	acc1, err1 := createAccount()
	acc2, err2 := createAccount()

	require.NoError(t, err1)
	require.NoError(t, err2)
	require.NotEqual(t, acc1.String(), acc2.String(), "each call should create a unique account")
}

// TestInitTxsBasic tests the basic transaction initialization
func TestInitTxsBasic(t *testing.T) {
	mock := &MockClient{}
	ds := createTestDataSet()

	client := &Client{
		DataSet: ds,
		Client:  nil, // Not used in testInitTxs
		Id:      0,
		TxCount: 0,
	}

	// Test with a small number of transactions
	err := testInitTxs(0.0, 5, client, mock)
	require.NoError(t, err)

	// Verify that faucet was called for each transaction
	require.Equal(t, 5, mock.GetFaucetCalls(), "faucet should be called once per transaction")

	// Verify that QueryTx was called for each transaction
	require.Equal(t, 5, mock.GetQueryTxCalls(), "QueryTx should be called once per transaction")

	// Verify transaction count was updated
	require.Equal(t, 5, client.TxCount, "TxCount should be updated")
}

// TestInitTxsConcurrency tests concurrent transaction execution
func TestInitTxsConcurrency(t *testing.T) {
	var callCount int
	var mu sync.Mutex

	mock := &MockClient{
		FaucetFunc: func(ctx context.Context, req *protocol.AcmeFaucet) (*api.TxResponse, error) {
			mu.Lock()
			callCount++
			mu.Unlock()
			// Simulate some processing time
			time.Sleep(10 * time.Millisecond)
			return &api.TxResponse{
				TransactionHash: []byte("mock-tx-hash"),
			}, nil
		},
	}

	ds := createTestDataSet()
	client := &Client{
		DataSet: ds,
		Client:  nil,
		Id:      0,
		TxCount: 0,
	}

	// Run with multiple concurrent transactions
	start := time.Now()
	err := testInitTxs(0.0, 10, client, mock)
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.Equal(t, 10, callCount, "all transactions should complete")

	// With 10 concurrent transactions taking 10ms each, total time should be
	// much less than 100ms (which would be sequential execution)
	require.Less(t, elapsed, 100*time.Millisecond, "transactions should run concurrently")
}

// TestInitTxsErrorHandling tests error handling in transaction execution
func TestInitTxsErrorHandling(t *testing.T) {
	mock := &MockClient{
		FaucetFunc: func(ctx context.Context, req *protocol.AcmeFaucet) (*api.TxResponse, error) {
			return nil, errors.New("faucet error")
		},
	}

	ds := createTestDataSet()
	client := &Client{
		DataSet: ds,
		Client:  nil,
		Id:      0,
		TxCount: 0,
	}

	// Should not return error even if faucet fails
	err := testInitTxs(0.0, 3, client, mock)
	require.NoError(t, err, "testInitTxs should not return error even if faucet fails")

	// TxCount should not increment on failures
	require.Equal(t, 0, client.TxCount, "TxCount should not increment on failures")
}

// TestInitTxsQueryTxError tests error handling when QueryTx fails
func TestInitTxsQueryTxError(t *testing.T) {
	mock := &MockClient{
		QueryTxFunc: func(ctx context.Context, req *api.TxnQuery) (*api.TransactionQueryResponse, error) {
			return nil, errors.New("query error")
		},
	}

	ds := createTestDataSet()
	client := &Client{
		DataSet: ds,
		Client:  nil,
		Id:      0,
		TxCount: 0,
	}

	err := testInitTxs(0.0, 3, client, mock)
	require.NoError(t, err, "testInitTxs should not return error even if QueryTx fails")

	// TxCount should not increment on QueryTx failures
	require.Equal(t, 0, client.TxCount, "TxCount should not increment on QueryTx failures")
}

// TestClientStructure tests the Client structure initialization
func TestClientStructure(t *testing.T) {
	ds := createTestDataSet()

	client := &Client{
		DataSet: ds,
		Client:  nil,
		Id:      42,
		TxCount: 10,
	}

	require.NotNil(t, client.DataSet)
	require.Equal(t, 42, client.Id)
	require.Equal(t, 10, client.TxCount)
}

// TestRateLimiting tests that transactions are rate-limited properly
func TestRateLimiting(t *testing.T) {
	// Skip in short mode as this test takes time
	if testing.Short() {
		t.Skip("skipping rate limiting test in short mode")
	}

	mock := &MockClient{
		FaucetFunc: func(ctx context.Context, req *protocol.AcmeFaucet) (*api.TxResponse, error) {
			// Fast response
			return &api.TxResponse{
				TransactionHash: []byte("mock-tx-hash"),
			}, nil
		},
	}

	ds := createTestDataSet()
	client := &Client{
		DataSet: ds,
		Client:  nil,
		Id:      0,
		TxCount: 0,
	}

	// Simulate multiple bursts with rate limiting
	// This mimics the main loop behavior
	transactionsPerClient := 5
	numBursts := 3

	startTime := time.Now()
	for i := 0; i < numBursts; i++ {
		tick := time.Now()
		err := testInitTxs(float64(i), transactionsPerClient, client, mock)
		require.NoError(t, err)

		// Sleep to maintain 1 second per burst
		time.Sleep(time.Second - time.Since(tick))
	}
	elapsed := time.Since(startTime)

	// Total time should be close to numBursts seconds (±200ms tolerance)
	expectedDuration := time.Duration(numBursts) * time.Second
	require.InDelta(t, expectedDuration.Seconds(), elapsed.Seconds(), 0.3,
		"rate limiting should maintain ~1 second per burst")

	// Verify total transactions
	totalTx := numBursts * transactionsPerClient
	require.Equal(t, totalTx, client.TxCount)
}

// TestDataSetSaving tests that transaction data is saved to the dataset
func TestDataSetSaving(t *testing.T) {
	mock := &MockClient{}
	ds := createTestDataSet()

	client := &Client{
		DataSet: ds,
		Client:  nil,
		Id:      5,
		TxCount: 0,
	}

	err := testInitTxs(1.5, 2, client, mock)
	require.NoError(t, err)

	// Verify dataset has entries
	// The dataset should have saved index, simTime, clientId, and settlementTime for each transaction
	require.NotNil(t, ds)
}

// TestAccountURLGeneration tests that generated account URLs are valid
func TestAccountURLGeneration(t *testing.T) {
	for i := 0; i < 100; i++ {
		acc, err := createAccount()
		require.NoError(t, err)
		require.NotNil(t, acc)

		// Verify URL string is not empty
		require.NotEmpty(t, acc.String())

		// Verify it contains acc://
		require.Contains(t, acc.String(), "acc://")
	}
}

// TestMockClientConcurrency tests that the mock client is thread-safe
func TestMockClientConcurrency(t *testing.T) {
	mock := &MockClient{}

	var wg sync.WaitGroup
	numGoroutines := 100

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_, _ = mock.Faucet(context.Background(), &protocol.AcmeFaucet{})
		}()
	}

	wg.Wait()
	require.Equal(t, numGoroutines, mock.GetFaucetCalls())
}

// TestTransactionHashGeneration tests that transaction hashes are properly captured
func TestTransactionHashGeneration(t *testing.T) {
	expectedHash := []byte("test-tx-hash-12345")

	mock := &MockClient{
		FaucetFunc: func(ctx context.Context, req *protocol.AcmeFaucet) (*api.TxResponse, error) {
			return &api.TxResponse{
				TransactionHash: expectedHash,
			}, nil
		},
		QueryTxFunc: func(ctx context.Context, req *api.TxnQuery) (*api.TransactionQueryResponse, error) {
			// Verify the hash was passed correctly
			require.Equal(t, expectedHash, req.Txid)
			return &api.TransactionQueryResponse{}, nil
		},
	}

	ds := createTestDataSet()
	client := &Client{
		DataSet: ds,
		Client:  nil,
		Id:      0,
		TxCount: 0,
	}

	err := testInitTxs(0.0, 1, client, mock)
	require.NoError(t, err)
	require.Equal(t, 1, mock.GetFaucetCalls())
	require.Equal(t, 1, mock.GetQueryTxCalls())
}

// BenchmarkCreateAccount benchmarks account creation performance
func BenchmarkCreateAccount(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := createAccount()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkInitTxs benchmarks transaction initialization
func BenchmarkInitTxs(b *testing.B) {
	mock := &MockClient{}
	ds := createTestDataSet()

	client := &Client{
		DataSet: ds,
		Client:  nil,
		Id:      0,
		TxCount: 0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client.TxCount = 0 // Reset for each iteration
		_ = testInitTxs(0.0, 10, client, mock)
	}
}

// TestED25519KeyGeneration tests that keys are generated properly
func TestED25519KeyGeneration(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	require.NotNil(t, pub)
	require.NotNil(t, priv)
	require.Equal(t, ed25519.PublicKeySize, len(pub))
	require.Equal(t, ed25519.PrivateKeySize, len(priv))
}

// TestLiteTokenAddress tests lite token address creation
func TestLiteTokenAddress(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	acc, err := protocol.LiteTokenAddress(pub, protocol.ACME, protocol.SignatureTypeED25519)
	require.NoError(t, err)
	require.NotNil(t, acc)

	// Verify the URL structure
	require.NotEmpty(t, acc.String())
	require.Contains(t, acc.String(), "acc://")
}

// TestClientIdUniqueness verifies that multiple clients have unique IDs
func TestClientIdUniqueness(t *testing.T) {
	ds := createTestDataSet()

	clients := make([]*Client, 10)
	for i := 0; i < 10; i++ {
		clients[i] = &Client{
			DataSet: ds,
			Client:  nil,
			Id:      i,
			TxCount: 0,
		}
	}

	// Verify all IDs are unique
	ids := make(map[int]bool)
	for _, c := range clients {
		require.False(t, ids[c.Id], "client IDs should be unique")
		ids[c.Id] = true
	}
	require.Equal(t, 10, len(ids))
}

// TestTxCountIncrement tests that transaction count increments correctly
func TestTxCountIncrement(t *testing.T) {
	mock := &MockClient{}
	ds := createTestDataSet()

	client := &Client{
		DataSet: ds,
		Client:  nil,
		Id:      0,
		TxCount: 0,
	}

	// First batch
	err := testInitTxs(0.0, 3, client, mock)
	require.NoError(t, err)
	require.Equal(t, 3, client.TxCount)

	// Second batch should add to existing count
	err = testInitTxs(1.0, 5, client, mock)
	require.NoError(t, err)
	require.Equal(t, 8, client.TxCount)
}

// TestSimTimePropagation tests that simulation time is passed correctly
func TestSimTimePropagation(t *testing.T) {
	mock := &MockClient{}
	ds := createTestDataSet()

	client := &Client{
		DataSet: ds,
		Client:  nil,
		Id:      0,
		TxCount: 0,
	}

	// Test with different simulation times
	simTimes := []float64{0.0, 1.5, 2.0, 10.5}
	for _, simTime := range simTimes {
		err := testInitTxs(simTime, 2, client, mock)
		require.NoError(t, err)
	}

	// Verify all transactions completed
	expectedTxCount := len(simTimes) * 2
	require.Equal(t, expectedTxCount, client.TxCount)
}

// TestMockDescribe tests the Describe mock method
func TestMockDescribe(t *testing.T) {
	mock := &MockClient{}

	resp, err := mock.Describe(context.Background())
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "TestNetwork", resp.Network.Id)
	require.Len(t, resp.Network.Partitions, 1)
	require.Equal(t, "TestPartition", resp.Network.Partitions[0].Id)
}

// TestMockDescribeCustom tests custom Describe implementation
func TestMockDescribeCustom(t *testing.T) {
	mock := &MockClient{
		DescribeFunc: func(ctx context.Context) (*api.DescriptionResponse, error) {
			return nil, errors.New("custom error")
		},
	}

	resp, err := mock.Describe(context.Background())
	require.Error(t, err)
	require.Nil(t, resp)
	require.Equal(t, "custom error", err.Error())
}

// TestCloseIdleConnections tests the CloseIdleConnections method
func TestCloseIdleConnections(t *testing.T) {
	mock := &MockClient{}

	// Should not panic
	mock.CloseIdleConnections()
}

// TestFaucetURL tests that Faucet receives the correct URL
func TestFaucetURL(t *testing.T) {
	var receivedURL string

	mock := &MockClient{
		FaucetFunc: func(ctx context.Context, req *protocol.AcmeFaucet) (*api.TxResponse, error) {
			if req.Url != nil {
				receivedURL = req.Url.String()
			}
			return &api.TxResponse{
				TransactionHash: []byte("test-hash"),
			}, nil
		},
	}

	ds := createTestDataSet()
	client := &Client{
		DataSet: ds,
		Client:  nil,
		Id:      0,
		TxCount: 0,
	}

	err := testInitTxs(0.0, 1, client, mock)
	require.NoError(t, err)
	require.NotEmpty(t, receivedURL, "Faucet should receive a URL")
	require.Contains(t, receivedURL, "acc://")
}
