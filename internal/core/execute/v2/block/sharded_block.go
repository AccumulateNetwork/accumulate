// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"sort"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const defaultShardCount = 64

// ShardedBlock wraps a Block and dispatches transaction messages across shards
// for parallel execution. Non-transaction messages (signatures, anchors, etc.)
// are processed sequentially after shard execution.
//
// Each shard creates a child database batch from the block's main batch.
// Because accounts are deterministically assigned to shards by ADI hash,
// shard batches operate on disjoint key sets and can safely execute in parallel.
// After parallel execution, child batches are committed to the parent in
// shard-ID order for deterministic state.
type ShardedBlock struct {
	inner      *Block
	dispatcher *execute.TransactionDispatcher
	shardCount int
}

// NewShardedBlock wraps an existing Block with sharded parallel execution.
// The shard count must be a power of two between 1 and 256.
func NewShardedBlock(inner *Block, shardCount int) (*ShardedBlock, error) {
	if shardCount < 1 || shardCount > 256 {
		return nil, errors.BadRequest.WithFormat("shard count must be between 1 and 256, got %d", shardCount)
	}
	if shardCount&(shardCount-1) != 0 {
		return nil, errors.BadRequest.WithFormat("shard count must be a power of two, got %d", shardCount)
	}

	shardDepth := 0
	for n := shardCount; n > 1; n >>= 1 {
		shardDepth++
	}

	return &ShardedBlock{
		inner:      inner,
		dispatcher: execute.NewTransactionDispatcher(shardDepth),
		shardCount: shardCount,
	}, nil
}

// Params returns the block parameters.
func (sb *ShardedBlock) Params() execute.BlockParams {
	return sb.inner.Params()
}

// Close delegates to the inner block.
func (sb *ShardedBlock) Close() (execute.BlockState, error) {
	return sb.inner.Close()
}

// shardWork holds the messages assigned to one shard and collects results.
type shardWork struct {
	shardID  int
	messages []messaging.Message
}

// shardResult holds the output of processing one shard.
type shardResult struct {
	shardID  int
	statuses []*protocol.TransactionStatus
	err      error
}

// Process dispatches messages across shards for parallel execution.
//
// Algorithm:
//  1. Classify messages: transaction messages with a principal are routed to a
//     shard; everything else (signatures, anchors) goes to a sequential bucket.
//  2. If messages span multiple shards, create a child batch per shard from
//     the block's main batch. Execute each shard's messages on its child batch
//     in a separate goroutine.
//  3. Commit child batches to the parent in shard-ID order (deterministic).
//  4. Process non-transaction messages sequentially on the main batch.
//
// If all messages route to one shard (common case) or there's only one message,
// we skip the parallel path and delegate directly to the inner block.
func (sb *ShardedBlock) Process(envelope *messaging.Envelope) ([]*protocol.TransactionStatus, error) {
	messages, err := envelope.Normalize()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Fast path: single shard or trivial envelope
	if sb.shardCount <= 1 || len(messages) <= 1 {
		return sb.inner.Process(envelope)
	}

	// Classify messages by shard. Transaction messages are routed by their
	// principal account. Synthetic messages (SyntheticMessage, BadSyntheticMessage)
	// are routed by the principal of the inner transaction so they execute on the
	// correct shard and see consistent state. All other messages (signatures,
	// anchors, etc.) are processed sequentially after shard execution.
	var nonTxnMsgs []messaging.Message
	shardBuckets := make(map[int]*shardWork, sb.shardCount)

	for _, msg := range messages {
		principal := extractShardablePrincipal(msg)
		if principal == nil {
			nonTxnMsgs = append(nonTxnMsgs, msg)
			continue
		}

		sid := sb.dispatcher.RouteToShard(principal)
		sw, exists := shardBuckets[sid]
		if !exists {
			sw = &shardWork{shardID: sid}
			shardBuckets[sid] = sw
		}
		sw.messages = append(sw.messages, msg)
	}

	// If all transaction messages land in one shard, use sequential path
	if len(shardBuckets) <= 1 {
		return sb.inner.Process(envelope)
	}

	// Sort shard IDs for deterministic commit order
	shardIDs := make([]int, 0, len(shardBuckets))
	for sid := range shardBuckets {
		shardIDs = append(shardIDs, sid)
	}
	sort.Ints(shardIDs)

	// Create a child batch per shard and execute in parallel.
	// Each child batch is nested under the block's main batch so its writes
	// are isolated until committed.
	type childEntry struct {
		shardID int
		batch   *database.Batch
		block   *Block
	}

	children := make([]childEntry, len(shardIDs))
	for i, sid := range shardIDs {
		childBatch := sb.inner.Batch.Begin(true)
		childBlock := &Block{
			BlockParams: sb.inner.BlockParams,
			Executor:    sb.inner.Executor,
			Batch:       childBatch,
		}
		children[i] = childEntry{shardID: sid, batch: childBatch, block: childBlock}
	}

	// Execute shards in parallel
	results := make([]shardResult, len(children))
	var wg sync.WaitGroup
	for i, child := range children {
		wg.Add(1)
		go func(idx int, b *Block, msgs []messaging.Message) {
			defer wg.Done()
			env := &messaging.Envelope{Messages: msgs}
			statuses, err := b.Process(env)
			results[idx] = shardResult{
				shardID:  children[idx].shardID,
				statuses: statuses,
				err:      err,
			}
		}(i, child.block, shardBuckets[shardIDs[i]].messages)
	}
	wg.Wait()

	// Check for errors before committing any child batch
	for _, r := range results {
		if r.err != nil {
			// Discard all child batches
			for _, c := range children {
				c.batch.Discard()
			}
			return nil, errors.UnknownError.WithFormat("shard %d: %w", r.shardID, r.err)
		}
	}

	// Commit child batches in shard-ID order (deterministic)
	var allStatuses []*protocol.TransactionStatus
	for i, child := range children {
		if err := child.batch.Commit(); err != nil {
			// Discard remaining
			for j := i; j < len(children); j++ {
				children[j].batch.Discard()
			}
			return nil, errors.UnknownError.WithFormat("commit shard %d: %w", child.shardID, err)
		}
		// Merge child block state into the parent block state
		sb.inner.State.Delivered += children[i].block.State.Delivered
		sb.inner.State.Signed += children[i].block.State.Signed
		sb.inner.State.Produced += children[i].block.State.Produced
		sb.inner.State.ChainUpdates.Merge(&children[i].block.State.ChainUpdates)
		sb.inner.State.ReceivedAnchors = append(sb.inner.State.ReceivedAnchors, children[i].block.State.ReceivedAnchors...)
		sb.inner.State.AcmeBurnt.Add(&sb.inner.State.AcmeBurnt, &children[i].block.State.AcmeBurnt)
		sb.inner.State.NetworkUpdate = append(sb.inner.State.NetworkUpdate, children[i].block.State.NetworkUpdate...)
		if children[i].block.State.MakeMajorBlock > 0 {
			sb.inner.State.MakeMajorBlock = children[i].block.State.MakeMajorBlock
			sb.inner.State.MakeMajorBlockTime = children[i].block.State.MakeMajorBlockTime
		}

		allStatuses = append(allStatuses, results[i].statuses...)
	}

	// Process non-transaction messages sequentially on the main batch
	if len(nonTxnMsgs) > 0 {
		env := &messaging.Envelope{Messages: nonTxnMsgs}
		statuses, err := sb.inner.Process(env)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
		allStatuses = append(allStatuses, statuses...)
	}

	return allStatuses, nil
}

// extractShardablePrincipal returns the principal URL for shard routing, or nil
// if the message cannot be routed to a shard. It handles:
//   - TransactionMessage: returns the transaction principal
//   - SyntheticMessage / BadSyntheticMessage: unwraps through SequencedMessage
//     to find the inner TransactionMessage principal
func extractShardablePrincipal(msg messaging.Message) *url.URL {
	switch m := msg.(type) {
	case *messaging.TransactionMessage:
		if m.Transaction != nil && m.Transaction.Header.Principal != nil {
			return m.Transaction.Header.Principal
		}

	case *messaging.SyntheticMessage:
		return extractPrincipalFromSynthFields(m.Message)

	case *messaging.BadSyntheticMessage:
		return extractPrincipalFromSynthFields(m.Message)
	}
	return nil
}

// extractPrincipalFromSynthFields unwraps a synthetic message's inner message
// (SequencedMessage → TransactionMessage) to find the principal URL.
func extractPrincipalFromSynthFields(inner messaging.Message) *url.URL {
	if inner == nil {
		return nil
	}
	// Unwrap SequencedMessage if present
	if seq, ok := inner.(*messaging.SequencedMessage); ok {
		inner = seq.Message
	}
	if txn, ok := inner.(*messaging.TransactionMessage); ok {
		if txn.Transaction != nil && txn.Transaction.Header.Principal != nil {
			return txn.Transaction.Header.Principal
		}
	}
	return nil
}

// ShardedBlockMetrics provides metrics about sharded block execution.
type ShardedBlockMetrics struct {
	// ShardCount is the number of shards used.
	ShardCount int

	// MessagesPerShard maps shard ID to message count.
	MessagesPerShard map[int]int

	// ShardExecutionTimes maps shard ID to execution duration.
	ShardExecutionTimes map[int]time.Duration

	// TotalWallTime is the total wall-clock time for parallel execution.
	TotalWallTime time.Duration
}
