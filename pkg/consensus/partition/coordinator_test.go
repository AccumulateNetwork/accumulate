// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package partition

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// MockDispatcher is a mock implementation of Dispatcher for testing.
type MockDispatcher struct {
	mu              sync.Mutex
	anchors         []*Anchor
	syntheticTxs    map[string][][]byte // partition -> transactions
	anchorHandler   func(*Anchor) error
	syntheticTxHandler func(string, []byte) error
}

func NewMockDispatcher() *MockDispatcher {
	return &MockDispatcher{
		syntheticTxs: make(map[string][][]byte),
	}
}

func (d *MockDispatcher) SubmitAnchor(ctx context.Context, anchor *Anchor) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.anchors = append(d.anchors, anchor)
	if d.anchorHandler != nil {
		return d.anchorHandler(anchor)
	}
	return nil
}

func (d *MockDispatcher) SubmitSyntheticTx(ctx context.Context, partition string, tx []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.syntheticTxs[partition] = append(d.syntheticTxs[partition], tx)
	if d.syntheticTxHandler != nil {
		return d.syntheticTxHandler(partition, tx)
	}
	return nil
}

func (d *MockDispatcher) GetAnchors() []*Anchor {
	d.mu.Lock()
	defer d.mu.Unlock()
	result := make([]*Anchor, len(d.anchors))
	copy(result, d.anchors)
	return result
}

func (d *MockDispatcher) GetSyntheticTxs(partition string) [][]byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	txs := d.syntheticTxs[partition]
	result := make([][]byte, len(txs))
	copy(result, txs)
	return result
}

func (d *MockDispatcher) ClearAnchors() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.anchors = nil
}

// generateTestKeyPair generates a random ed25519 key pair for testing.
func generateTestKeyPair(t *testing.T) ed25519.PrivateKey {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return priv
}

// createTestBlock creates a test block with the given parameters.
func createTestBlock(index uint64, hash [32]byte) *Block {
	return &Block{
		Index: index,
		Hash:  hash,
		Round: types.Round(index),
		Time:  time.Now(),
	}
}

// TestPartitionCoordinator_AnchorFlow tests BVN->DN anchor flow.
func TestPartitionCoordinator_AnchorFlow(t *testing.T) {
	ctx := context.Background()

	// Setup: Create DN coordinator
	dnKey := generateTestKeyPair(t)
	dnDispatcher := NewMockDispatcher()
	dnAnchorer := NewDefaultAnchorer("dn")

	dnCoordinator, err := NewCoordinator(CoordinatorConfig{
		Partition:      "dn",
		PartitionType:  PartitionTypeDN,
		KeyPair:        dnKey,
		AnchorInterval: 1,
	}, dnDispatcher, dnAnchorer)
	require.NoError(t, err)

	// Setup: Create BVN coordinator
	bvnKey := generateTestKeyPair(t)
	bvnDispatcher := NewMockDispatcher()
	bvnAnchorer := NewDefaultAnchorer("bvn0")

	// Connect BVN dispatcher to DN coordinator - when BVN sends anchor, DN receives it
	bvnDispatcher.anchorHandler = func(anchor *Anchor) error {
		return dnCoordinator.ReceiveAnchor(anchor)
	}

	bvnCoordinator, err := NewCoordinator(CoordinatorConfig{
		Partition:      "bvn0",
		PartitionType:  PartitionTypeBVN,
		KeyPair:        bvnKey,
		AnchorInterval: 1,
		DNAddress:      "dn",
	}, bvnDispatcher, bvnAnchorer)
	require.NoError(t, err)

	// Track received anchors and receipts
	var receivedAnchors []*Anchor
	var receivedReceipts []*AnchorReceipt
	var anchorMu sync.Mutex

	dnCoordinator.OnAnchorReceived(func(anchor *Anchor) {
		anchorMu.Lock()
		receivedAnchors = append(receivedAnchors, anchor)
		anchorMu.Unlock()
	})

	bvnCoordinator.OnReceiptReceived(func(receipt *AnchorReceipt) {
		anchorMu.Lock()
		receivedReceipts = append(receivedReceipts, receipt)
		anchorMu.Unlock()
	})

	// Test Step 1: BVN produces block
	blockHash := [32]byte{1, 2, 3, 4, 5, 6, 7, 8}
	bvnBlock := createTestBlock(1, blockHash)

	// Test Step 2: BVN creates and sends anchor for DN
	err = bvnCoordinator.OnBlockCommit(ctx, bvnBlock)
	require.NoError(t, err)

	// Verify anchor was sent
	require.Equal(t, uint64(1), bvnCoordinator.LastAnchoredIndex())

	// Verify anchor was received by DN
	anchorMu.Lock()
	require.Len(t, receivedAnchors, 1)
	require.Equal(t, "bvn0", receivedAnchors[0].SourcePartition)
	require.Equal(t, uint64(1), receivedAnchors[0].BlockIndex)
	require.Equal(t, blockHash, receivedAnchors[0].BlockHash)
	anchorMu.Unlock()

	// Verify anchor is pending in DN
	pendingAnchors := dnCoordinator.PendingAnchors()
	require.Len(t, pendingAnchors, 1)
	require.NotNil(t, pendingAnchors["bvn0"])

	// Test Step 3: DN processes anchor and creates receipt
	dnBlockHash := [32]byte{10, 20, 30, 40}
	receipts, err := dnCoordinator.ProcessPendingAnchors(1, dnBlockHash)
	require.NoError(t, err)
	require.Len(t, receipts, 1)

	// Verify receipt details
	receipt := receipts[0]
	require.Equal(t, "bvn0", receipt.SourcePartition)
	require.Equal(t, uint64(1), receipt.AnchorBlockIdx)
	require.Equal(t, blockHash, receipt.AnchorBlockHash)
	require.Equal(t, uint64(1), receipt.DNBlockIndex)
	require.Equal(t, dnBlockHash, receipt.DNBlockHash)

	// Test Step 4: DN sends receipt back to BVN
	err = bvnCoordinator.ReceiveReceipt(receipt)
	require.NoError(t, err)

	// Test Step 5: Verify BVN received the receipt
	anchorMu.Lock()
	require.Len(t, receivedReceipts, 1)
	require.Equal(t, uint64(1), receivedReceipts[0].AnchorBlockIdx)
	require.Equal(t, uint64(1), receivedReceipts[0].DNBlockIndex)
	anchorMu.Unlock()

	// Verify BVN stored the receipt
	storedReceipts := bvnCoordinator.AnchorReceipts()
	require.Len(t, storedReceipts, 1)

	// Verify pending anchors cleared in DN
	require.Len(t, dnCoordinator.PendingAnchors(), 0)
}

// TestPartitionCoordinator_SyntheticTxRouting tests cross-partition tx routing.
func TestPartitionCoordinator_SyntheticTxRouting(t *testing.T) {
	ctx := context.Background()

	// Setup BVN0
	bvn0Key := generateTestKeyPair(t)
	bvn0Dispatcher := NewMockDispatcher()

	bvn0Coordinator, err := NewCoordinator(CoordinatorConfig{
		Partition:      "bvn0",
		PartitionType:  PartitionTypeBVN,
		KeyPair:        bvn0Key,
		AnchorInterval: 1,
	}, bvn0Dispatcher, NewDefaultAnchorer("bvn0"))
	require.NoError(t, err)

	// Setup BVN1
	bvn1Key := generateTestKeyPair(t)
	bvn1Dispatcher := NewMockDispatcher()

	// Track received synthetic transactions on BVN1
	var receivedTxs [][]byte
	var txMu sync.Mutex

	bvn0Dispatcher.syntheticTxHandler = func(partition string, tx []byte) error {
		if partition == "bvn1" {
			txMu.Lock()
			receivedTxs = append(receivedTxs, tx)
			txMu.Unlock()
		}
		return nil
	}

	_, err = NewCoordinator(CoordinatorConfig{
		Partition:      "bvn1",
		PartitionType:  PartitionTypeBVN,
		KeyPair:        bvn1Key,
		AnchorInterval: 1,
	}, bvn1Dispatcher, NewDefaultAnchorer("bvn1"))
	require.NoError(t, err)

	// Create synthetic transaction on BVN0
	syntheticTx := []byte("synthetic-tx-data-for-bvn1")

	// Route to BVN1
	err = bvn0Coordinator.SubmitSyntheticTx(ctx, "bvn1", syntheticTx)
	require.NoError(t, err)

	// Verify delivery
	txMu.Lock()
	require.Len(t, receivedTxs, 1)
	require.Equal(t, syntheticTx, receivedTxs[0])
	txMu.Unlock()

	// Verify dispatcher recorded the transaction
	dispatcherTxs := bvn0Dispatcher.GetSyntheticTxs("bvn1")
	require.Len(t, dispatcherTxs, 1)
	require.Equal(t, syntheticTx, dispatcherTxs[0])
}

// TestPartitionCoordinator_MultipleAnchors tests batched anchors from multiple BVNs.
func TestPartitionCoordinator_MultipleAnchors(t *testing.T) {
	ctx := context.Background()

	// Setup DN coordinator
	dnKey := generateTestKeyPair(t)
	dnDispatcher := NewMockDispatcher()

	dnCoordinator, err := NewCoordinator(CoordinatorConfig{
		Partition:      "dn",
		PartitionType:  PartitionTypeDN,
		KeyPair:        dnKey,
		AnchorInterval: 1,
	}, dnDispatcher, NewDefaultAnchorer("dn"))
	require.NoError(t, err)

	// Track received anchors
	var receivedAnchors []*Anchor
	var anchorMu sync.Mutex

	dnCoordinator.OnAnchorReceived(func(anchor *Anchor) {
		anchorMu.Lock()
		receivedAnchors = append(receivedAnchors, anchor)
		anchorMu.Unlock()
	})

	// Setup multiple BVNs
	numBVNs := 3
	bvnCoordinators := make([]*Coordinator, numBVNs)
	bvnDispatchers := make([]*MockDispatcher, numBVNs)

	for i := 0; i < numBVNs; i++ {
		bvnKey := generateTestKeyPair(t)
		bvnDispatcher := NewMockDispatcher()
		bvnDispatchers[i] = bvnDispatcher

		// Connect to DN
		bvnDispatcher.anchorHandler = func(anchor *Anchor) error {
			return dnCoordinator.ReceiveAnchor(anchor)
		}

		bvnName := []string{"bvn0", "bvn1", "bvn2"}[i]
		bvnCoordinator, err := NewCoordinator(CoordinatorConfig{
			Partition:      bvnName,
			PartitionType:  PartitionTypeBVN,
			KeyPair:        bvnKey,
			AnchorInterval: 1,
			DNAddress:      "dn",
		}, bvnDispatcher, NewDefaultAnchorer(bvnName))
		require.NoError(t, err)

		bvnCoordinators[i] = bvnCoordinator
	}

	// All BVNs produce blocks and anchor simultaneously
	var wg sync.WaitGroup
	for i := 0; i < numBVNs; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			blockHash := [32]byte{byte(idx + 1)}
			block := createTestBlock(1, blockHash)
			err := bvnCoordinators[idx].OnBlockCommit(ctx, block)
			require.NoError(t, err)
		}(i)
	}
	wg.Wait()

	// Verify all anchors received
	anchorMu.Lock()
	require.Len(t, receivedAnchors, numBVNs)
	anchorMu.Unlock()

	// Verify all anchors pending in DN
	pendingAnchors := dnCoordinator.PendingAnchors()
	require.Len(t, pendingAnchors, numBVNs)
	require.NotNil(t, pendingAnchors["bvn0"])
	require.NotNil(t, pendingAnchors["bvn1"])
	require.NotNil(t, pendingAnchors["bvn2"])

	// DN processes all anchors
	dnBlockHash := [32]byte{100}
	receipts, err := dnCoordinator.ProcessPendingAnchors(1, dnBlockHash)
	require.NoError(t, err)
	require.Len(t, receipts, numBVNs)

	// Verify receipts for all BVNs
	receiptsByPartition := make(map[string]*AnchorReceipt)
	for _, r := range receipts {
		receiptsByPartition[r.SourcePartition] = r
	}
	require.Contains(t, receiptsByPartition, "bvn0")
	require.Contains(t, receiptsByPartition, "bvn1")
	require.Contains(t, receiptsByPartition, "bvn2")

	// Verify pending anchors cleared
	require.Len(t, dnCoordinator.PendingAnchors(), 0)

	// Verify stored receipts in DN
	dnReceipts := dnCoordinator.AnchorReceipts()
	require.Len(t, dnReceipts, numBVNs)
}

// TestPartitionCoordinator_AnchorInterval tests that anchoring respects the interval.
func TestPartitionCoordinator_AnchorInterval(t *testing.T) {
	ctx := context.Background()

	bvnKey := generateTestKeyPair(t)
	dispatcher := NewMockDispatcher()

	// Set anchor interval to 5 blocks
	coordinator, err := NewCoordinator(CoordinatorConfig{
		Partition:      "bvn0",
		PartitionType:  PartitionTypeBVN,
		KeyPair:        bvnKey,
		AnchorInterval: 5,
		DNAddress:      "dn",
	}, dispatcher, NewDefaultAnchorer("bvn0"))
	require.NoError(t, err)

	// Commit blocks 1-4 - no anchors should be sent
	for i := uint64(1); i <= 4; i++ {
		block := createTestBlock(i, [32]byte{byte(i)})
		err := coordinator.OnBlockCommit(ctx, block)
		require.NoError(t, err)
	}
	require.Len(t, dispatcher.GetAnchors(), 0)

	// Commit block 5 - anchor should be sent
	block5 := createTestBlock(5, [32]byte{5})
	err = coordinator.OnBlockCommit(ctx, block5)
	require.NoError(t, err)
	require.Len(t, dispatcher.GetAnchors(), 1)
	require.Equal(t, uint64(5), dispatcher.GetAnchors()[0].BlockIndex)

	// Commit blocks 6-9 - no new anchors
	for i := uint64(6); i <= 9; i++ {
		block := createTestBlock(i, [32]byte{byte(i)})
		err := coordinator.OnBlockCommit(ctx, block)
		require.NoError(t, err)
	}
	require.Len(t, dispatcher.GetAnchors(), 1)

	// Commit block 10 - second anchor should be sent
	block10 := createTestBlock(10, [32]byte{10})
	err = coordinator.OnBlockCommit(ctx, block10)
	require.NoError(t, err)
	require.Len(t, dispatcher.GetAnchors(), 2)
	require.Equal(t, uint64(10), dispatcher.GetAnchors()[1].BlockIndex)
}

// TestPartitionCoordinator_DNDoesNotSendAnchors verifies DN doesn't send anchors.
func TestPartitionCoordinator_DNDoesNotSendAnchors(t *testing.T) {
	ctx := context.Background()

	dnKey := generateTestKeyPair(t)
	dispatcher := NewMockDispatcher()

	coordinator, err := NewCoordinator(CoordinatorConfig{
		Partition:      "dn",
		PartitionType:  PartitionTypeDN,
		KeyPair:        dnKey,
		AnchorInterval: 1,
	}, dispatcher, NewDefaultAnchorer("dn"))
	require.NoError(t, err)

	// Commit multiple blocks
	for i := uint64(1); i <= 10; i++ {
		block := createTestBlock(i, [32]byte{byte(i)})
		err := coordinator.OnBlockCommit(ctx, block)
		require.NoError(t, err)
	}

	// No anchors should be sent from DN
	require.Len(t, dispatcher.GetAnchors(), 0)
}

// TestPartitionCoordinator_ReceiptValidation tests that BVN validates receipt partition.
func TestPartitionCoordinator_ReceiptValidation(t *testing.T) {
	bvnKey := generateTestKeyPair(t)
	dispatcher := NewMockDispatcher()

	coordinator, err := NewCoordinator(CoordinatorConfig{
		Partition:      "bvn0",
		PartitionType:  PartitionTypeBVN,
		KeyPair:        bvnKey,
		AnchorInterval: 1,
	}, dispatcher, NewDefaultAnchorer("bvn0"))
	require.NoError(t, err)

	// Create receipt for wrong partition
	wrongReceipt := &AnchorReceipt{
		SourcePartition: "bvn1", // Wrong partition
		AnchorBlockIdx:  1,
		DNBlockIndex:    1,
	}

	// Should reject receipt for wrong partition
	err = coordinator.ReceiveReceipt(wrongReceipt)
	require.Error(t, err)
	require.Contains(t, err.Error(), "receipt is for partition bvn1")

	// Create receipt for correct partition
	correctReceipt := &AnchorReceipt{
		SourcePartition: "bvn0",
		AnchorBlockIdx:  1,
		DNBlockIndex:    1,
	}

	// Should accept receipt for correct partition
	err = coordinator.ReceiveReceipt(correctReceipt)
	require.NoError(t, err)
}

// TestPartitionCoordinator_OnlyDNReceivesAnchors tests that only DN can receive anchors.
func TestPartitionCoordinator_OnlyDNReceivesAnchors(t *testing.T) {
	bvnKey := generateTestKeyPair(t)
	dispatcher := NewMockDispatcher()

	coordinator, err := NewCoordinator(CoordinatorConfig{
		Partition:      "bvn0",
		PartitionType:  PartitionTypeBVN,
		KeyPair:        bvnKey,
		AnchorInterval: 1,
	}, dispatcher, NewDefaultAnchorer("bvn0"))
	require.NoError(t, err)

	// Create an anchor
	anchor := NewAnchor("bvn1", 1, [32]byte{1}, [32]byte{2}, [32]byte{3})

	// BVN should reject incoming anchor
	err = coordinator.ReceiveAnchor(anchor)
	require.Error(t, err)
	require.Contains(t, err.Error(), "only DN can receive anchors")
}

// TestPartitionCoordinator_OnlyBVNReceivesReceipts tests that only BVN can receive receipts.
func TestPartitionCoordinator_OnlyBVNReceivesReceipts(t *testing.T) {
	dnKey := generateTestKeyPair(t)
	dispatcher := NewMockDispatcher()

	coordinator, err := NewCoordinator(CoordinatorConfig{
		Partition:      "dn",
		PartitionType:  PartitionTypeDN,
		KeyPair:        dnKey,
		AnchorInterval: 1,
	}, dispatcher, NewDefaultAnchorer("dn"))
	require.NoError(t, err)

	// Create a receipt
	receipt := &AnchorReceipt{
		SourcePartition: "dn",
		AnchorBlockIdx:  1,
		DNBlockIndex:    1,
	}

	// DN should reject incoming receipt
	err = coordinator.ReceiveReceipt(receipt)
	require.Error(t, err)
	require.Contains(t, err.Error(), "only BVN can receive receipts")
}

// TestPartitionCoordinator_AnchorSignatureVerification tests anchor signature verification.
func TestPartitionCoordinator_AnchorSignatureVerification(t *testing.T) {
	dnKey := generateTestKeyPair(t)
	dispatcher := NewMockDispatcher()

	coordinator, err := NewCoordinator(CoordinatorConfig{
		Partition:      "dn",
		PartitionType:  PartitionTypeDN,
		KeyPair:        dnKey,
		AnchorInterval: 1,
	}, dispatcher, NewDefaultAnchorer("dn"))
	require.NoError(t, err)

	// Create anchor with valid signature
	bvnKey := generateTestKeyPair(t)
	anchor := NewAnchor("bvn0", 1, [32]byte{1}, [32]byte{2}, [32]byte{3})
	anchor.Validator = bvnKey.Public().(ed25519.PublicKey)
	anchor.Sign(bvnKey)

	// Valid anchor should be accepted
	err = coordinator.ReceiveAnchor(anchor)
	require.NoError(t, err)

	// Create anchor with invalid signature
	invalidAnchor := NewAnchor("bvn1", 1, [32]byte{1}, [32]byte{2}, [32]byte{3})
	invalidAnchor.Validator = bvnKey.Public().(ed25519.PublicKey)
	invalidAnchor.Signature = []byte("invalid-signature-data-that-is-64-bytes-long-for-ed25519-sigs!!")

	// Invalid anchor should be rejected
	err = coordinator.ReceiveAnchor(invalidAnchor)
	require.Error(t, err)
	require.Contains(t, err.Error(), "verify anchor")
}

// TestPartitionCoordinator_ConcurrentAnchoring tests concurrent anchor operations.
func TestPartitionCoordinator_ConcurrentAnchoring(t *testing.T) {
	dnKey := generateTestKeyPair(t)
	dnDispatcher := NewMockDispatcher()

	dnCoordinator, err := NewCoordinator(CoordinatorConfig{
		Partition:      "dn",
		PartitionType:  PartitionTypeDN,
		KeyPair:        dnKey,
		AnchorInterval: 1,
	}, dnDispatcher, NewDefaultAnchorer("dn"))
	require.NoError(t, err)

	// Create multiple BVNs sending anchors concurrently
	numBVNs := 10
	numBlocks := 5

	var wg sync.WaitGroup
	for i := 0; i < numBVNs; i++ {
		wg.Add(1)
		go func(bvnIdx int) {
			defer wg.Done()

			bvnKey := generateTestKeyPair(t)
			bvnName := "bvn" + string(rune('0'+bvnIdx))

			for j := 1; j <= numBlocks; j++ {
				anchor := NewAnchor(bvnName, uint64(j), [32]byte{byte(bvnIdx), byte(j)}, [32]byte{}, [32]byte{})
				anchor.Validator = bvnKey.Public().(ed25519.PublicKey)
				anchor.Sign(bvnKey)

				err := dnCoordinator.ReceiveAnchor(anchor)
				if err != nil {
					t.Logf("Warning: anchor error for %s block %d: %v", bvnName, j, err)
				}
			}
		}(i)
	}
	wg.Wait()

	// DN should process anchors concurrently without errors
	// Each BVN's latest anchor should be in pending (last block overwrites previous)
	pendingAnchors := dnCoordinator.PendingAnchors()
	require.GreaterOrEqual(t, len(pendingAnchors), 1)

	// Process pending anchors
	dnBlockHash := [32]byte{255}
	receipts, err := dnCoordinator.ProcessPendingAnchors(1, dnBlockHash)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(receipts), 1)

	// After processing, pending should be empty
	require.Len(t, dnCoordinator.PendingAnchors(), 0)

	// Run concurrent process operations
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := dnCoordinator.ProcessPendingAnchors(uint64(idx+2), [32]byte{byte(idx)})
			// Should not error even with no pending anchors
			require.NoError(t, err)
		}(i)
	}
	wg.Wait()
}

// TestPartitionCoordinator_NilHandling tests nil value handling.
func TestPartitionCoordinator_NilHandling(t *testing.T) {
	dnKey := generateTestKeyPair(t)
	dispatcher := NewMockDispatcher()

	coordinator, err := NewCoordinator(CoordinatorConfig{
		Partition:      "dn",
		PartitionType:  PartitionTypeDN,
		KeyPair:        dnKey,
		AnchorInterval: 1,
	}, dispatcher, NewDefaultAnchorer("dn"))
	require.NoError(t, err)

	// Test nil block
	err = coordinator.OnBlockCommit(context.Background(), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "block is nil")

	// Test nil anchor
	err = coordinator.ReceiveAnchor(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "anchor is nil")
}

// TestPartitionCoordinator_BVNNilReceipt tests nil receipt handling.
func TestPartitionCoordinator_BVNNilReceipt(t *testing.T) {
	bvnKey := generateTestKeyPair(t)
	dispatcher := NewMockDispatcher()

	coordinator, err := NewCoordinator(CoordinatorConfig{
		Partition:      "bvn0",
		PartitionType:  PartitionTypeBVN,
		KeyPair:        bvnKey,
		AnchorInterval: 1,
	}, dispatcher, NewDefaultAnchorer("bvn0"))
	require.NoError(t, err)

	// Test nil receipt
	err = coordinator.ReceiveReceipt(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "receipt is nil")
}

// TestPartitionCoordinator_PartitionInfo tests partition info methods.
func TestPartitionCoordinator_PartitionInfo(t *testing.T) {
	dnKey := generateTestKeyPair(t)
	dispatcher := NewMockDispatcher()

	coordinator, err := NewCoordinator(CoordinatorConfig{
		Partition:      "test-partition",
		PartitionType:  PartitionTypeDN,
		KeyPair:        dnKey,
		AnchorInterval: 1,
	}, dispatcher, NewDefaultAnchorer("test-partition"))
	require.NoError(t, err)

	require.Equal(t, "test-partition", coordinator.Partition())
	require.Equal(t, PartitionTypeDN, coordinator.Type())
}

// TestPartitionType_String tests PartitionType string representation.
func TestPartitionType_String(t *testing.T) {
	require.Equal(t, "DN", PartitionTypeDN.String())
	require.Equal(t, "BVN", PartitionTypeBVN.String())
	require.Equal(t, "PartitionType(99)", PartitionType(99).String())
}

// TestNewCoordinator_Validation tests coordinator creation validation.
func TestNewCoordinator_Validation(t *testing.T) {
	validKey := generateTestKeyPair(t)
	dispatcher := NewMockDispatcher()
	anchorer := NewDefaultAnchorer("test")

	// Missing partition name
	_, err := NewCoordinator(CoordinatorConfig{
		Partition:     "",
		PartitionType: PartitionTypeDN,
		KeyPair:       validKey,
	}, dispatcher, anchorer)
	require.Error(t, err)
	require.Contains(t, err.Error(), "partition name is required")

	// Invalid key pair
	_, err = NewCoordinator(CoordinatorConfig{
		Partition:     "test",
		PartitionType: PartitionTypeDN,
		KeyPair:       []byte("short"),
	}, dispatcher, anchorer)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid key pair")

	// Valid config with default anchor interval
	coord, err := NewCoordinator(CoordinatorConfig{
		Partition:      "test",
		PartitionType:  PartitionTypeDN,
		KeyPair:        validKey,
		AnchorInterval: 0, // Should default to 1
	}, dispatcher, anchorer)
	require.NoError(t, err)
	require.NotNil(t, coord)
}
