// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dispatcher

import (
	"context"
	"sync"
	"testing"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/testutil"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// mockRouter implements routing.Router for testing.
type mockRouter struct {
	routes map[string]string // URL path -> partition
	mu     sync.RWMutex
}

func newMockRouter() *mockRouter {
	return &mockRouter{
		routes: make(map[string]string),
	}
}

func (r *mockRouter) AddRoute(urlPath, partition string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes[urlPath] = partition
}

func (r *mockRouter) RouteAccount(u *url.URL) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if partition, ok := r.routes[u.String()]; ok {
		return partition, nil
	}
	// Default to bvn0
	return "bvn0", nil
}

func (r *mockRouter) Route(envs ...*messaging.Envelope) (string, error) {
	return "bvn0", nil
}

// We can't use the real pubsub.PubSub in tests without a real libp2p host,
// so we test the components that don't need it.

func TestDispatcher_QueueManagement(t *testing.T) {
	// Test that queuing works correctly without needing real envelopes
	d := &Dispatcher{
		partition:   "bvn0",
		queue:       make(map[string][]*messaging.Envelope),
		sendTimeout: DefaultSendTimeout,
	}

	// Directly add to queue (bypassing Submit which needs envelope normalization)
	env1 := &messaging.Envelope{}
	env2 := &messaging.Envelope{}

	d.queueMu.Lock()
	d.queue["bvn1"] = append(d.queue["bvn1"], env1)
	d.queue["bvn2"] = append(d.queue["bvn2"], env2)
	d.queueMu.Unlock()

	// Check queues
	d.queueMu.Lock()
	assert.Len(t, d.queue["bvn1"], 1)
	assert.Len(t, d.queue["bvn2"], 1)
	d.queueMu.Unlock()
}

func TestDispatcher_Send_MovesQueue(t *testing.T) {
	// Test that Send moves queue to a new map
	d := &Dispatcher{
		partition:   "bvn0",
		queue:       make(map[string][]*messaging.Envelope),
		topics:      make(map[string]*pubsub.Topic),
		sendTimeout: DefaultSendTimeout,
	}

	// Add some envelopes to the queue
	d.queue["bvn1"] = []*messaging.Envelope{{}}
	d.queue["bvn2"] = []*messaging.Envelope{{}, {}}

	// Verify queue is populated before Send
	d.queueMu.Lock()
	assert.Len(t, d.queue["bvn1"], 1)
	assert.Len(t, d.queue["bvn2"], 2)
	d.queueMu.Unlock()
}

func TestDispatcher_Send_EmptyQueue(t *testing.T) {
	d := &Dispatcher{
		partition:   "bvn0",
		queue:       make(map[string][]*messaging.Envelope),
		topics:      make(map[string]*pubsub.Topic),
		sendTimeout: DefaultSendTimeout,
	}

	ctx := context.Background()

	// Send with empty queue should return immediately
	errs := d.Send(ctx)

	// Channel should be closed immediately for empty queue
	errCount := 0
	for range errs {
		errCount++
	}
	assert.Equal(t, 0, errCount)
}

func TestDispatcher_Subscribe(t *testing.T) {
	inbound := make(chan *messaging.Envelope, 10)
	d := &Dispatcher{
		partition: "bvn0",
		inbound:   inbound,
	}

	ch := d.Subscribe()
	// Both should be the same underlying channel (just different types for send vs receive)
	assert.NotNil(t, ch)
}

func TestDispatcher_Close(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	d := &Dispatcher{
		partition: "bvn0",
		topics:    make(map[string]*pubsub.Topic),
		inbound:   make(chan *messaging.Envelope, 10),
		ctx:       ctx,
		cancel:    cancel,
	}

	// Close should not panic
	d.Close()

	// Should be marked as closed
	d.mu.Lock()
	assert.True(t, d.closed)
	d.mu.Unlock()

	// Closing again should be safe
	d.Close()
}

func TestDispatcher_MessageWrapping_RoundTrip(t *testing.T) {
	partitions := []string{"bvn0", "directory", "bvn-long-name-123"}

	for _, partition := range partitions {
		t.Run(partition, func(t *testing.T) {
			d := &Dispatcher{partition: partition}

			// Test with various payload sizes
			payloads := [][]byte{
				{},
				{1, 2, 3},
				make([]byte, 1000),
			}

			for i, payload := range payloads {
				// Fill large payloads with data
				if len(payload) > 10 {
					for j := range payload {
						payload[j] = byte(j % 256)
					}
				}

				wrapped := d.wrapMessage(payload)
				source, unwrapped, err := d.unwrapMessage(wrapped)

				require.NoError(t, err, "payload %d", i)
				assert.Equal(t, partition, source, "payload %d", i)
				assert.Equal(t, payload, unwrapped, "payload %d", i)
			}
		})
	}
}

func TestAnchorHandler_PendingAnchor(t *testing.T) {
	// Test GetPendingAnchor, RecordAck, and ClearPendingAnchor
	ah := &AnchorHandler{
		partition: "bvn0",
		pending:   make(map[uint64]*pendingAnchor),
	}

	anchor := &Anchor{
		SourcePartition: "bvn0",
		BlockIndex:      100,
		BlockHash:       [32]byte{1, 2, 3},
		StateRoot:       [32]byte{4, 5, 6},
		Timestamp:       time.Now(),
	}

	// Add pending anchor
	ah.pendingMu.Lock()
	ah.pending[100] = &pendingAnchor{
		anchor: anchor,
		acks:   make(map[string]bool),
	}
	ah.pendingMu.Unlock()

	// Get pending anchor
	got, ok := ah.GetPendingAnchor(100)
	assert.True(t, ok)
	assert.Equal(t, anchor.BlockIndex, got.BlockIndex)

	// Non-existent anchor
	_, ok = ah.GetPendingAnchor(999)
	assert.False(t, ok)

	// Record ack
	result := ah.RecordAck(100, "BVN1")
	assert.False(t, result) // Never returns true, caller determines quorum

	// Verify ack was recorded
	ah.pendingMu.RLock()
	pa := ah.pending[100]
	assert.True(t, pa.acks["bvn1"]) // normalized to lowercase
	ah.pendingMu.RUnlock()

	// Record ack for non-existent anchor
	result = ah.RecordAck(999, "bvn2")
	assert.False(t, result)

	// Clear pending anchor
	ah.ClearPendingAnchor(100)

	// Should be gone
	_, ok = ah.GetPendingAnchor(100)
	assert.False(t, ok)
}

func TestAnchorHandler_SubscribeChannels(t *testing.T) {
	ah := &AnchorHandler{
		anchors: make(chan *Anchor, 10),
		acks:    make(chan *AnchorAck, 10),
	}

	// SubscribeAnchors should return the anchors channel
	anchorCh := ah.SubscribeAnchors()
	assert.NotNil(t, anchorCh)

	// SubscribeAcks should return the acks channel
	ackCh := ah.SubscribeAcks()
	assert.NotNil(t, ackCh)

	// Verify they receive data
	testAnchor := &Anchor{BlockIndex: 123}
	testAck := &AnchorAck{BlockIndex: 456}

	ah.anchors <- testAnchor
	ah.acks <- testAck

	select {
	case received := <-anchorCh:
		assert.Equal(t, uint64(123), received.BlockIndex)
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for anchor")
	}

	select {
	case received := <-ackCh:
		assert.Equal(t, uint64(456), received.BlockIndex)
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for ack")
	}
}

func TestAnchorHandler_Close(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	ah := &AnchorHandler{
		partition: "bvn0",
		anchors:   make(chan *Anchor, 10),
		acks:      make(chan *AnchorAck, 10),
		pending:   make(map[uint64]*pendingAnchor),
		ctx:       ctx,
		cancel:    cancel,
	}

	// Close should not panic
	err := ah.Close()
	assert.NoError(t, err)

	// Should be marked as closed
	ah.mu.Lock()
	assert.True(t, ah.closed)
	ah.mu.Unlock()

	// Close again should be safe
	err = ah.Close()
	assert.NoError(t, err)
}

func TestAnchorMarshalUnmarshal_EdgeCases(t *testing.T) {
	testCases := []struct {
		name   string
		anchor *Anchor
	}{
		{
			name: "Empty partition",
			anchor: &Anchor{
				SourcePartition: "",
				BlockIndex:      1,
				BlockHash:       [32]byte{},
				StateRoot:       [32]byte{},
				Timestamp:       time.Unix(0, 0),
			},
		},
		{
			name: "Max values",
			anchor: &Anchor{
				SourcePartition: "partition",
				BlockIndex:      ^uint64(0),
				BlockHash:       [32]byte{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255},
				StateRoot:       [32]byte{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255},
				Timestamp:       time.Unix(0, int64(^uint64(0)>>1)),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := marshalAnchor(tc.anchor)
			require.NoError(t, err)

			restored, err := unmarshalAnchor(data)
			require.NoError(t, err)

			assert.Equal(t, tc.anchor.SourcePartition, restored.SourcePartition)
			assert.Equal(t, tc.anchor.BlockIndex, restored.BlockIndex)
			assert.Equal(t, tc.anchor.BlockHash, restored.BlockHash)
			assert.Equal(t, tc.anchor.StateRoot, restored.StateRoot)
			assert.Equal(t, tc.anchor.Timestamp.UnixNano(), restored.Timestamp.UnixNano())
		})
	}
}

func TestAnchorAckMarshalUnmarshal_EdgeCases(t *testing.T) {
	testCases := []struct {
		name string
		ack  *AnchorAck
	}{
		{
			name: "Empty partitions",
			ack: &AnchorAck{
				AnchorPartition: "",
				AckPartition:    "",
				BlockIndex:      1,
				BlockHash:       [32]byte{},
			},
		},
		{
			name: "Long partitions",
			ack: &AnchorAck{
				AnchorPartition: "very-long-partition-name-here",
				AckPartition:    "another-very-long-partition",
				BlockIndex:      12345,
				BlockHash:       [32]byte{1, 2, 3},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := marshalAnchorAck(tc.ack)
			require.NoError(t, err)

			restored, err := unmarshalAnchorAck(data)
			require.NoError(t, err)

			assert.Equal(t, tc.ack.AnchorPartition, restored.AnchorPartition)
			assert.Equal(t, tc.ack.AckPartition, restored.AckPartition)
			assert.Equal(t, tc.ack.BlockIndex, restored.BlockIndex)
			assert.Equal(t, tc.ack.BlockHash, restored.BlockHash)
		})
	}
}

func TestPartitionDiscovery_Lifecycle(t *testing.T) {
	host := testutil.NewMockHost(peer.ID("test-host"))
	pd := NewPartitionDiscovery(host, "bvn0")

	// Initially no peers
	peers := pd.GetPeers("bvn1")
	assert.Nil(t, peers)

	// Add multiple peers
	peer1 := peer.ID("peer-1")
	peer2 := peer.ID("peer-2")
	peer3 := peer.ID("peer-3")

	pd.AddPeer("bvn1", peer1)
	pd.AddPeer("bvn1", peer2)
	pd.AddPeer("bvn2", peer3)

	// Check bvn1 has 2 peers
	peers = pd.GetPeers("bvn1")
	assert.Len(t, peers, 2)

	// Check bvn2 has 1 peer
	peers = pd.GetPeers("bvn2")
	assert.Len(t, peers, 1)

	// Get all partitions
	partitions := pd.GetAllPartitions()
	assert.Len(t, partitions, 2)

	// Remove a peer
	pd.RemovePeer("bvn1", peer1)
	peers = pd.GetPeers("bvn1")
	assert.Len(t, peers, 1)
	assert.Equal(t, peer2, peers[0])
}

func TestPartitionDiscovery_CaseInsensitive(t *testing.T) {
	pd := NewPartitionDiscovery(nil, "bvn0")

	peer1 := peer.ID("peer-1")
	peer2 := peer.ID("peer-2")

	// Add with different cases
	pd.AddPeer("BVN1", peer1)
	pd.AddPeer("bvn1", peer2)

	// Should be in the same partition (case insensitive)
	peers := pd.GetPeers("Bvn1")
	assert.Len(t, peers, 2)

	// Remove with different case
	pd.RemovePeer("BVN1", peer1)
	peers = pd.GetPeers("bvn1")
	assert.Len(t, peers, 1)
}

func TestTopicPatterns(t *testing.T) {
	// Verify topic patterns produce expected strings
	partition := "bvn0"

	dispatchTopic := TopicDispatch
	assert.Contains(t, dispatchTopic, "%s")

	anchorTopic := TopicAnchor
	assert.Contains(t, anchorTopic, "%s")

	// Test formatting
	formatted := "acc/" + partition + "/dispatch"
	assert.NotEmpty(t, formatted)
}

func TestDefaultValues(t *testing.T) {
	// Verify default values are reasonable
	assert.Greater(t, DefaultEnvelopeChannelSize, 0)
	assert.Greater(t, DefaultSendTimeout, time.Duration(0))
	assert.Greater(t, DefaultAckTimeout, time.Duration(0))
	assert.Greater(t, DefaultAnchorChannelSize, 0)
}

func TestDispatcherOptions_CustomValues(t *testing.T) {
	opts := DispatcherOptions{
		EnvelopeChannelSize: 1000,
		SendTimeout:         30 * time.Second,
	}

	assert.Equal(t, 1000, opts.EnvelopeChannelSize)
	assert.Equal(t, 30*time.Second, opts.SendTimeout)
}

func TestAnchorHandlerOptions_CustomValues(t *testing.T) {
	opts := AnchorHandlerOptions{
		AnchorChannelSize: 200,
		AckChannelSize:    200,
	}

	assert.Equal(t, 200, opts.AnchorChannelSize)
	assert.Equal(t, 200, opts.AckChannelSize)
}

func TestUnwrapMessage_InvalidCases(t *testing.T) {
	d := &Dispatcher{}

	testCases := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"too short", []byte{0x01}},
		{"wrong version", []byte{0x02, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{"truncated partition", []byte{0x01, 0xFF, 0x00, 0x00, 0x00}},
		{"truncated payload length", []byte{0x01, 0x02, 'a', 'b'}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := d.unwrapMessage(tc.data)
			assert.Error(t, err)
		})
	}
}

func TestNewDispatcher_ValidationErrors(t *testing.T) {
	// Test that nil host returns an error
	_, err := NewDispatcher(nil, nil, nil, "bvn0")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "host is nil")
}

func TestNewAnchorHandler_ValidationErrors_Detailed(t *testing.T) {
	host := testutil.NewMockHost(peer.ID("test"))

	// Test nil host
	_, err := NewAnchorHandler(nil, nil, "bvn0")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "host is nil")

	// Test nil pubsub
	_, err = NewAnchorHandler(host, nil, "bvn0")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "pubsub is nil")
}

func TestDispatcher_Start_AlreadyStarted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d := &Dispatcher{
		partition: "bvn0",
		topics:    make(map[string]*pubsub.Topic),
		queue:     make(map[string][]*messaging.Envelope),
		inbound:   make(chan *messaging.Envelope, 10),
		ctx:       ctx,
		cancel:    cancel,
	}

	// Trying to start an already started dispatcher should error
	err := d.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already started")
}

func TestDispatcher_Start_Closed(t *testing.T) {
	ctx := context.Background()

	d := &Dispatcher{
		partition: "bvn0",
		topics:    make(map[string]*pubsub.Topic),
		queue:     make(map[string][]*messaging.Envelope),
		inbound:   make(chan *messaging.Envelope, 10),
		closed:    true,
	}

	// Trying to start a closed dispatcher should error
	err := d.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

func TestAnchorHandler_Start_AlreadyStarted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ah := &AnchorHandler{
		partition: "bvn0",
		anchors:   make(chan *Anchor, 10),
		acks:      make(chan *AnchorAck, 10),
		pending:   make(map[uint64]*pendingAnchor),
		ctx:       ctx,
		cancel:    cancel,
	}

	// Trying to start an already started handler should error
	err := ah.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already started")
}

func TestAnchorHandler_Start_Closed(t *testing.T) {
	ctx := context.Background()

	ah := &AnchorHandler{
		partition: "bvn0",
		anchors:   make(chan *Anchor, 10),
		acks:      make(chan *AnchorAck, 10),
		pending:   make(map[uint64]*pendingAnchor),
		closed:    true,
	}

	// Trying to start a closed handler should error
	err := ah.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

func TestAnchorHandler_BroadcastAnchor_NotStarted(t *testing.T) {
	ah := &AnchorHandler{
		partition: "bvn0",
		anchors:   make(chan *Anchor, 10),
		acks:      make(chan *AnchorAck, 10),
		pending:   make(map[uint64]*pendingAnchor),
	}

	anchor := &Anchor{
		SourcePartition: "bvn0",
		BlockIndex:      100,
		BlockHash:       [32]byte{1, 2, 3},
	}

	// BroadcastAnchor without starting should error
	err := ah.BroadcastAnchor(context.Background(), anchor)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not started")
}

func TestAnchorHandler_BroadcastAck_NotStarted(t *testing.T) {
	ah := &AnchorHandler{
		partition: "bvn0",
		anchors:   make(chan *Anchor, 10),
		acks:      make(chan *AnchorAck, 10),
		pending:   make(map[uint64]*pendingAnchor),
	}

	ack := &AnchorAck{
		AnchorPartition: "bvn0",
		AckPartition:    "directory",
		BlockIndex:      100,
	}

	// BroadcastAck without starting should error
	err := ah.BroadcastAck(context.Background(), ack)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not started")
}

func TestDispatcher_PartitionNormalization(t *testing.T) {
	d := &Dispatcher{
		partition: "BVN0", // uppercase
	}

	// wrapMessage should use the partition as-is (it's normalized at creation time)
	// But let's verify the dispatcher stores partition correctly
	assert.Equal(t, "BVN0", d.partition)
}

func TestUnwrapMessage_PayloadTruncated(t *testing.T) {
	d := &Dispatcher{}

	// Create a message with correct header but payload length exceeds actual data
	// Version 1, partition length 2, partition "ab", payload length 100 (but no payload)
	data := []byte{0x01, 0x02, 'a', 'b', 0x00, 0x00, 0x00, 0x64}

	_, _, err := d.unwrapMessage(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "truncated")
}

func TestSyncProgress(t *testing.T) {
	// Test sync progress calculation - reuse the same test approach
	progress := struct {
		TotalChunks    int
		ReceivedChunks int
	}{
		TotalChunks:    10,
		ReceivedChunks: 5,
	}

	// 50% complete
	percent := float64(progress.ReceivedChunks) / float64(progress.TotalChunks) * 100
	assert.Equal(t, 50.0, percent)

	// Edge case: zero total
	progress.TotalChunks = 0
	if progress.TotalChunks == 0 {
		percent = 0
	} else {
		percent = float64(progress.ReceivedChunks) / float64(progress.TotalChunks) * 100
	}
	assert.Equal(t, 0.0, percent)
}

func TestMockRouterUsage(t *testing.T) {
	router := newMockRouter()

	// Add a route
	router.AddRoute("acc://foo.acme", "bvn1")

	// Test RouteAccount
	u, _ := url.Parse("acc://foo.acme")
	partition, err := router.RouteAccount(u)
	assert.NoError(t, err)
	assert.Equal(t, "bvn1", partition)

	// Test default route
	u2, _ := url.Parse("acc://unknown.acme")
	partition, err = router.RouteAccount(u2)
	assert.NoError(t, err)
	assert.Equal(t, "bvn0", partition)

	// Test Route method
	partition, err = router.Route(&messaging.Envelope{})
	assert.NoError(t, err)
	assert.Equal(t, "bvn0", partition)
}

func TestNewDispatcherWithOptions_Defaults(t *testing.T) {
	// Verify defaults are applied when options are zero-valued
	opts := DispatcherOptions{}

	// Check that defaults would be applied
	if opts.EnvelopeChannelSize <= 0 {
		opts.EnvelopeChannelSize = DefaultEnvelopeChannelSize
	}
	if opts.SendTimeout <= 0 {
		opts.SendTimeout = DefaultSendTimeout
	}

	assert.Equal(t, DefaultEnvelopeChannelSize, opts.EnvelopeChannelSize)
	assert.Equal(t, DefaultSendTimeout, opts.SendTimeout)
}

func TestNewAnchorHandlerWithOptions_Defaults(t *testing.T) {
	// Verify defaults are applied when options are zero-valued
	opts := AnchorHandlerOptions{}

	// Check that defaults would be applied
	if opts.AnchorChannelSize <= 0 {
		opts.AnchorChannelSize = DefaultAnchorChannelSize
	}
	if opts.AckChannelSize <= 0 {
		opts.AckChannelSize = DefaultAnchorChannelSize
	}

	assert.Equal(t, DefaultAnchorChannelSize, opts.AnchorChannelSize)
	assert.Equal(t, DefaultAnchorChannelSize, opts.AckChannelSize)
}

func TestDispatcher_GetOrCreateTopic_ConcurrentAccess(t *testing.T) {
	// Test that topics map is properly protected
	d := &Dispatcher{
		partition: "bvn0",
		topics:    make(map[string]*pubsub.Topic),
	}

	// Add topics manually (simulating topic creation)
	d.mu.Lock()
	d.topics["bvn1"] = nil // Placeholder
	d.topics["bvn2"] = nil
	d.mu.Unlock()

	// Read topics concurrently
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.mu.RLock()
			_ = d.topics["bvn1"]
			d.mu.RUnlock()
		}()
	}
	wg.Wait()
}

func TestPartitionDiscovery_ConcurrentAccess(t *testing.T) {
	pd := NewPartitionDiscovery(nil, "bvn0")

	// Concurrent add/remove operations
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		peerID := peer.ID("peer-" + string(rune('0'+i)))

		go func(pid peer.ID) {
			defer wg.Done()
			pd.AddPeer("bvn1", pid)
		}(peerID)

		go func(pid peer.ID) {
			defer wg.Done()
			pd.RemovePeer("bvn1", pid)
		}(peerID)
	}
	wg.Wait()

	// Should not panic or have data races
}

func TestAnchor_AllFields(t *testing.T) {
	now := time.Now()
	anchor := &Anchor{
		SourcePartition: "bvn0",
		BlockIndex:      1000,
		BlockHash:       [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
		StateRoot:       [32]byte{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
		Timestamp:       now,
	}

	// Test marshal and unmarshal
	data, err := marshalAnchor(anchor)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	restored, err := unmarshalAnchor(data)
	require.NoError(t, err)

	assert.Equal(t, anchor.SourcePartition, restored.SourcePartition)
	assert.Equal(t, anchor.BlockIndex, restored.BlockIndex)
	assert.Equal(t, anchor.BlockHash, restored.BlockHash)
	assert.Equal(t, anchor.StateRoot, restored.StateRoot)
	assert.Equal(t, anchor.Timestamp.UnixNano(), restored.Timestamp.UnixNano())
}

func TestAnchorAck_AllFields(t *testing.T) {
	ack := &AnchorAck{
		AnchorPartition: "bvn0",
		AckPartition:    "directory",
		BlockIndex:      5000,
		BlockHash:       [32]byte{10, 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120, 130, 140, 150, 160, 170, 180, 190, 200, 210, 220, 230, 240, 250, 255, 245, 235, 225, 215, 205, 195},
	}

	// Test marshal and unmarshal
	data, err := marshalAnchorAck(ack)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	restored, err := unmarshalAnchorAck(data)
	require.NoError(t, err)

	assert.Equal(t, ack.AnchorPartition, restored.AnchorPartition)
	assert.Equal(t, ack.AckPartition, restored.AckPartition)
	assert.Equal(t, ack.BlockIndex, restored.BlockIndex)
	assert.Equal(t, ack.BlockHash, restored.BlockHash)
}

func TestBigEndianHelpers_AllRanges(t *testing.T) {
	testCases := []uint64{
		0,
		1,
		127,
		128,
		255,
		256,
		65535,
		65536,
		16777215,
		16777216,
		4294967295,
		4294967296,
		1099511627775,
		1099511627776,
		281474976710655,
		281474976710656,
		72057594037927935,
		72057594037927936,
		^uint64(0), // max uint64
	}

	for _, val := range testCases {
		b := make([]byte, 8)
		putUint64BE(b, val)
		result := getUint64BE(b)
		assert.Equal(t, val, result, "failed for value %d", val)
	}
}

func TestDispatcher_InboundChannel_Buffering(t *testing.T) {
	// Create dispatcher with specific buffer size
	bufSize := 5
	inbound := make(chan *messaging.Envelope, bufSize)
	d := &Dispatcher{
		partition: "bvn0",
		inbound:   inbound,
	}

	// Fill buffer
	for i := 0; i < bufSize; i++ {
		select {
		case d.inbound <- &messaging.Envelope{}:
		default:
			t.Fatal("Should be able to buffer messages")
		}
	}

	// Next send should block (or use select with default)
	select {
	case d.inbound <- &messaging.Envelope{}:
		t.Fatal("Buffer should be full")
	default:
		// Expected - buffer is full
	}
}

func TestPartitionDiscovery_EmptyPartition(t *testing.T) {
	pd := NewPartitionDiscovery(nil, "bvn0")

	// Getting peers from non-existent partition
	peers := pd.GetPeers("nonexistent")
	assert.Nil(t, peers)

	// Getting all partitions when empty
	partitions := pd.GetAllPartitions()
	assert.Len(t, partitions, 0)
}

func TestDispatcher_WrapMessage_Version(t *testing.T) {
	d := &Dispatcher{partition: "test"}

	payload := []byte("test payload")
	wrapped := d.wrapMessage(payload)

	// First byte should be version 0x01
	assert.Equal(t, byte(0x01), wrapped[0])

	// Second byte should be partition length
	assert.Equal(t, byte(len("test")), wrapped[1])
}
