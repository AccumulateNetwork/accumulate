// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

//==============================================================================
// CASCADE AI DIRECTIVE: DO NOT DELETE THESE INSTRUCTIONS
//==============================================================================
// 1. This file contains critical anchor healing code that must be preserved
// 2. All development plan notes and comments must be preserved
// 3. See ANCHOR_HEIGHT_TEST_PLAN.md for complete directives
// 4. CASCADE AI MUST NOT delete or modify any development plans
//==============================================================================

//==============================================================================
// CASCADE AI DIRECTIVE: PRESERVE THIS ENTIRE DEVELOPMENT PLAN
//==============================================================================
// Development Plan:
// 1. Implement caching system for anchor healing to reduce redundant network requests
// 2. Store query results in a map indexed by URL and query type
// 3. Track problematic nodes to avoid querying them for certain types of requests
//
// Development Plan Notes:
// Note 1: URL Construction Differences - There is a fundamental difference in how URLs are constructed between sequence.go and heal_anchor.go:
// - sequence.go uses raw partition URLs for tracking (e.g., acc://bvn-Apollo.acme)
// - heal_anchor.go appends the partition ID to the anchor pool URL (e.g., acc://dn.acme/anchors/Apollo)
// This discrepancy can cause anchor healing to fail because:
// - The code might be looking for anchors at different URL paths
// - Queries might return "element does not exist" errors when checking the wrong URL format
// - Anchor relationships might not be properly maintained between partitions
//==============================================================================
// CASCADE AI DIRECTIVE: DO NOT DELETE ANY PART OF THIS DEVELOPMENT PLAN
//==============================================================================

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/healing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Maximum number of anchors to check per source-destination pair in a single cycle
const maxAnchorsPerCycle = 5

// PendingAnchorsQuery is a query for pending anchors
type PendingAnchorsQuery struct {
	SourceNetwork      *url.URL `json:"sourceNetwork"`
	DestinationNetwork *url.URL `json:"destinationNetwork"`
	IncludePending     bool     `json:"includePending"`
	Partition          string   `json:"partition"`
	Start              uint64   `json:"start"`
	Count              uint64   `json:"count"`
}

// CopyAsInterface implements the api.Query interface
func (q *PendingAnchorsQuery) CopyAsInterface() interface{} {
	if q == nil {
		return nil
	}
	cpy := *q
	return &cpy
}

// IsValid implements the api.Query interface
func (q *PendingAnchorsQuery) IsValid() error {
	return nil
}

// MarshalBinary implements the api.Query interface
func (q *PendingAnchorsQuery) MarshalBinary() ([]byte, error) {
	return json.Marshal(q)
}

// UnmarshalBinary implements the api.Query interface
func (q *PendingAnchorsQuery) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, q)
}

// UnmarshalBinaryFrom implements the api.Query interface
func (q *PendingAnchorsQuery) UnmarshalBinaryFrom(r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	return q.UnmarshalBinary(data)
}

// UnmarshalFieldsFrom implements the api.Query interface
func (q *PendingAnchorsQuery) UnmarshalFieldsFrom(reader *encoding.Reader) error {
	// Read the source network URL
	var readSourceNetwork = func(r io.Reader) error {
		var data []byte
		var err error
		data, err = io.ReadAll(r)
		if err != nil {
			return err
		}
		q.SourceNetwork, err = url.Parse(string(data))
		return err
	}
	reader.ReadValue(1, readSourceNetwork)

	// Read the destination network URL
	var readDestinationNetwork = func(r io.Reader) error {
		var data []byte
		var err error
		data, err = io.ReadAll(r)
		if err != nil {
			return err
		}
		q.DestinationNetwork, err = url.Parse(string(data))
		return err
	}
	reader.ReadValue(2, readDestinationNetwork)

	// Read the include pending flag
	var readIncludePending = func(r io.Reader) error {
		var data []byte
		var err error
		data, err = io.ReadAll(r)
		if err != nil {
			return err
		}
		q.IncludePending = len(data) > 0 && data[0] != 0
		return nil
	}
	reader.ReadValue(3, readIncludePending)

	// Read the partition
	var readPartition = func(r io.Reader) error {
		var data []byte
		var err error
		data, err = io.ReadAll(r)
		if err != nil {
			return err
		}
		q.Partition = string(data)
		return nil
	}
	reader.ReadValue(4, readPartition)

	// Read the start
	var readStart = func(r io.Reader) error {
		var data []byte
		var err error
		data, err = io.ReadAll(r)
		if err != nil {
			return err
		}
		q.Start = binary.BigEndian.Uint64(data)
		return nil
	}
	reader.ReadValue(5, readStart)

	// Read the count
	var readCount = func(r io.Reader) error {
		var data []byte
		var err error
		data, err = io.ReadAll(r)
		if err != nil {
			return err
		}
		q.Count = binary.BigEndian.Uint64(data)
		return nil
	}
	reader.ReadValue(6, readCount)

	return nil
}

// QueryType implements the api.Query interface
func (q *PendingAnchorsQuery) QueryType() api.QueryType {
	return api.QueryTypePending // Using the existing Pending query type
}

// PendingAnchorsResponse is a response for pending anchors
type PendingAnchorsResponse struct {
	Pending []*PendingAnchor `json:"pending"`
}

// PendingAnchor is a pending anchor
type PendingAnchor struct {
	SourceNetwork      *url.URL `json:"sourceNetwork"`
	DestinationNetwork *url.URL `json:"destinationNetwork"`
	SourceIndex        uint64   `json:"sourceIndex"`
	Root               []byte   `json:"root"`
	RootIndexExists    bool     `json:"rootIndexExists"`
	State              string   `json:"state"`
}

// CopyAsInterface implements the api.Record interface
func (r *PendingAnchorsResponse) CopyAsInterface() interface{} {
	return &PendingAnchorsResponse{
		Pending: r.Pending,
	}
}

// MarshalBinary implements the api.Record interface
func (r *PendingAnchorsResponse) MarshalBinary() ([]byte, error) {
	return json.Marshal(r)
}

// RecordType implements the api.Record interface
func (r *PendingAnchorsResponse) RecordType() api.RecordType {
	return api.RecordType(1) // Using a placeholder value
}

// UnmarshalBinary implements the api.Record interface
func (r *PendingAnchorsResponse) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, r)
}

// UnmarshalBinaryFrom implements the api.Record interface
func (r *PendingAnchorsResponse) UnmarshalBinaryFrom(rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	return r.UnmarshalBinary(data)
}

// UnmarshalFieldsFrom implements the api.Record interface
func (r *PendingAnchorsResponse) UnmarshalFieldsFrom(reader *encoding.Reader) error {
	// Read the anchors
	var readAnchors = func(rd io.Reader) error {
		var data []byte
		var err error
		data, err = io.ReadAll(rd)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, &r.Pending)
	}
	reader.ReadValue(1, readAnchors)
	return nil
}

var cmdHealAnchor = &cobra.Command{
	Use:   "anchor [network]",
	Short: "Heal anchors",
	Long:  `Heal anchors between partitions`,
	Args:  cobra.ExactArgs(1),
	Run:   healAnchor,
}

func init() {
	cmdHeal.AddCommand(cmdHealAnchor)
	healerFlags(cmdHealAnchor)
}

// Global healer instance for anchor healing
var currentHealer *healer

// getCurrentHealer returns the current healer instance
func getCurrentHealer() *healer {
	return currentHealer
}

func healAnchor(_ *cobra.Command, args []string) {
	h := new(healer)
	h.Reset() // Initialize the cache and other fields
	
	// Store the healer instance globally
	currentHealer = h
	
	// Set up the healing functions
	h.healSingle = healSingleAnchor
	h.healSequence = healAnchors
	h.heal(args)
}

// Cascade assures that this struct/function complies and considers all the notes given in the development plan.  No code will disregard discoveries documented in the notes
// healAnchors heals anchors between two partitions
func healAnchors(src, dst *protocol.PartitionInfo) {
	// Get the healer instance
	h := getCurrentHealer()
	
	// Create URLs for the source and destination partitions
	srcUrl := protocol.PartitionUrl(src.ID)
	dstUrl := protocol.PartitionUrl(dst.ID)
	
	slog.InfoContext(h.ctx, "Healing anchors between partitions",
		"node", h.network,
		"source", src.ID,
		"destination", dst.ID,
		"source_url", srcUrl.String(),
		"destination_url", dstUrl.String())

	// Get the anchor ledger from the destination partition
	// Use the approach from sequence.go: get the anchor pool URL
	anchorLedgerUrl := dstUrl.JoinPath(protocol.AnchorPool)
	anchorAccount, err := h.tryEach().QueryAccount(h.ctx, anchorLedgerUrl, nil)
	if err != nil {
		slog.ErrorContext(h.ctx, "Failed to get anchor ledger directly",
			"node", h.network,
			"partition", dst.ID,
			"error", err)
		
		// Track this skipped partition pair
		skippedPartitionsMu.Lock()
		if skippedPartitions[src.ID] == nil {
			skippedPartitions[src.ID] = make(map[string]string)
		}
		skippedPartitions[src.ID][dst.ID] = fmt.Sprintf("failed-to-get-anchor-ledger: %v", err)
		skippedPartitionsMu.Unlock()
		
		return
	}

	// Try to cast to AnchorLedger, but handle gracefully if it's not
	var anchorLedger *protocol.AnchorLedger
	if al, ok := anchorAccount.Account.(*protocol.AnchorLedger); ok {
		anchorLedger = al
	} else {
		// This is a fallthrough case where we don't handle other types
		// and will return an error below
		slog.WarnContext(h.ctx, "Account is not an AnchorLedger",
			"node", h.network,
			"partition", dst.ID,
			"type", fmt.Sprintf("%T", anchorAccount.Account))
		
		// Track this skipped partition pair
		skippedPartitionsMu.Lock()
		if skippedPartitions[src.ID] == nil {
			skippedPartitions[src.ID] = make(map[string]string)
		}
		skippedPartitions[src.ID][dst.ID] = fmt.Sprintf("not-anchor-ledger: %T", anchorAccount.Account)
		skippedPartitionsMu.Unlock()
		
		return
	}
	
	// Find the anchor chain for the source partition
	// Use the approach from sequence.go: use Anchor(srcUrl) to get the tracking info
	// Cascade assures that this struct/function complies and considers all the notes given in the development plan.  No code will disregard discoveries documented in the notes
	anchorChain := anchorLedger.Anchor(srcUrl)

	if anchorChain == nil {
		slog.WarnContext(h.ctx, "Source partition not found in anchor ledger",
			"node", h.network,
			"source", src.ID,
			"destination", dst.ID)
		
		// Track this skipped partition pair
		skippedPartitionsMu.Lock()
		if skippedPartitions[src.ID] == nil {
			skippedPartitions[src.ID] = make(map[string]string)
		}
		skippedPartitions[src.ID][dst.ID] = "source-partition-not-found-in-anchor-ledger"
		skippedPartitionsMu.Unlock()
		
		return
	}

	// Get the anchor chain URL
	// Cascade assures that this struct/function complies and considers all the notes given in the development plan.  No code will disregard discoveries documented in the notes
	anchorChainUrl := srcUrl.JoinPath(protocol.AnchorPool)
	
	// Query the chain count using our new method
	chainCount, err := h.queryChainCountFromAnchorChain(h.ctx, anchorChainUrl)
	if err != nil {
		slog.ErrorContext(h.ctx, "Failed to query chain count",
			"node", h.network,
			"source", src.ID,
			"destination", dst.ID,
			"error", err)
		return
	}

	// Calculate missing anchors
	var firstMissingHeight uint64
	var totalMissingAnchors uint64
	var unprocessedAnchors uint64
	var unprocessedStart uint64
	var unprocessedEnd uint64
	var lastProcessedHeight uint64

	// Use the same approach as sequence.go for calculating missing anchors
	// In sequence.go, it compares Produced with Received
	slog.InfoContext(h.ctx, "Anchor chain details",
		"source", src.ID,
		"destination", dst.ID,
		"chain_count", chainCount,
		"delivered", anchorChain.Delivered,
		"received", anchorChain.Received,
		"produced", anchorChain.Produced)

	// Check if we have any anchors in the destination
	if anchorChain.Delivered == 0 {
		// No anchors in the destination, start from the beginning
		firstMissingHeight = 1
		// Use the same logic as sequence.go: compare produced with received
		totalMissingAnchors = anchorChain.Produced - anchorChain.Received
		if totalMissingAnchors == 0 && chainCount > 0 {
			// Fallback to chain count if produced is 0
			totalMissingAnchors = chainCount
		}
	} else {
		// We have some anchors, check if we're missing any
		firstMissingHeight = anchorChain.Delivered + 1
		// Use the same logic as sequence.go: compare produced with received
		if anchorChain.Produced > anchorChain.Received {
			totalMissingAnchors = anchorChain.Produced - anchorChain.Received
		} else if chainCount > anchorChain.Delivered {
			// Fallback to chain count if produced is less than delivered
			totalMissingAnchors = chainCount - anchorChain.Delivered
		}
	}

	// Check if we have unprocessed anchors
	if anchorChain.Received > anchorChain.Delivered {
		unprocessedAnchors = anchorChain.Received - anchorChain.Delivered
		unprocessedStart = anchorChain.Delivered + 1
		unprocessedEnd = anchorChain.Received
	}

	// Record the last processed height
	lastProcessedHeight = anchorChain.Delivered

	// Update the missing anchors map
	missingAnchorsMu.Lock()
	if missingAnchors == nil {
		missingAnchors = make(map[string]map[string]MissingAnchorsInfo)
	}
	if missingAnchors[src.ID] == nil {
		missingAnchors[src.ID] = make(map[string]MissingAnchorsInfo)
	}
	missingAnchors[src.ID][dst.ID] = MissingAnchorsInfo{
		Count:              int(totalMissingAnchors),
		FirstMissingHeight: firstMissingHeight,
		Unprocessed:        int(unprocessedAnchors),
		UnprocessedStart:   unprocessedStart,
		UnprocessedEnd:     unprocessedEnd,
		LastProcessedHeight: lastProcessedHeight,
	}
	missingAnchorsMu.Unlock()

	// Process anchors if needed
	if totalMissingAnchors > 0 {
		// Check if we've already processed these anchors
		checkedAnchorsMu.RLock()
		alreadyChecked := false
		if checkedAnchors != nil && checkedAnchors[src.ID] != nil && checkedAnchors[src.ID][dst.ID] != nil {
			if checkedAnchors[src.ID][dst.ID][firstMissingHeight] {
				alreadyChecked = true
			}
		}
		checkedAnchorsMu.RUnlock()

		if alreadyChecked {
			slog.InfoContext(h.ctx, "Already checked this anchor height in this session, skipping",
				"node", h.network,
				"source", src.ID,
				"destination", dst.ID,
				"height", firstMissingHeight)
			return
		}

		// Mark this anchor height as checked
		checkedAnchorsMu.Lock()
		if checkedAnchors == nil {
			checkedAnchors = make(map[string]map[string]map[uint64]bool)
		}
		if checkedAnchors[src.ID] == nil {
			checkedAnchors[src.ID] = make(map[string]map[uint64]bool)
		}
		if checkedAnchors[src.ID][dst.ID] == nil {
			checkedAnchors[src.ID][dst.ID] = make(map[uint64]bool)
		}
		checkedAnchors[src.ID][dst.ID][firstMissingHeight] = true
		checkedAnchorsMu.Unlock()

		// Limit the number of anchors to process
		processLimit := totalMissingAnchors
		if processLimit > maxAnchorsPerCycle {
			processLimit = maxAnchorsPerCycle
		}

		slog.InfoContext(h.ctx, "Processing missing anchors",
			"node", h.network,
			"source", src.ID,
			"destination", dst.ID,
			"first_missing_height", firstMissingHeight,
			"total_missing", totalMissingAnchors,
			"processing", processLimit)

		// Process the anchors one by one
		for i := firstMissingHeight; i < firstMissingHeight+processLimit; i++ {
			healSingleAnchor(src, dst, i, nil)
		}
	} else {
		slog.InfoContext(h.ctx, "No missing anchors detected",
			"node", h.network,
			"source", src.ID,
			"destination", dst.ID,
			"chain_count", chainCount,
			"delivered", anchorChain.Delivered)
	}
}

// queryChainEntriesFromAnchorChain queries entries from an anchor chain using the approach from sequence.go
func (h *healer) queryChainEntriesFromAnchorChain(ctx context.Context, chainUrl *url.URL, startIndex, count uint64) ([]*api.MessageRecord[messaging.Message], error) {
	slog.InfoContext(ctx, "Querying anchor entries", 
		"chain", chainUrl,
		"start", startIndex,
		"count", count)
	
	// Following sequence.go approach, collect entries one by one
	var messages []*api.MessageRecord[messaging.Message]
	
	// Extract partition IDs from the chain URL
	srcId, ok := protocol.ParsePartitionUrl(chainUrl)
	if !ok {
		return nil, fmt.Errorf("failed to parse source partition URL: %v", chainUrl)
	}
	
	// Create source URL
	srcUrl := protocol.PartitionUrl(srcId)
	
	// For each partition, try to get the sequence
	for _, dst := range h.net.Status.Network.Partitions {
		// Skip if it's the same partition
		if dst.ID == srcId {
			continue
		}
		
		dstUrl := protocol.PartitionUrl(dst.ID)
		
		// Process only up to the limit
		processedCount := uint64(0)
		for i := startIndex; i < startIndex+count && processedCount < count; i++ {
			var msg *api.MessageRecord[messaging.Message]
			var err error
			
			// Try with network peers if available, just like sequence.go
			if h.net != nil && len(h.net.Peers) > 0 {
				// Try each peer for this partition
				for _, peer := range h.net.Peers[strings.ToLower(srcId)] {
					peerCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
					defer cancel()
					
					slog.DebugContext(ctx, "Checking anchor", 
						"source", srcId, 
						"destination", dst.ID, 
						"number", i, 
						"peer", peer.ID)
					
					msg, err = h.C2.ForPeer(peer.ID).Private().Sequence(peerCtx, srcUrl.JoinPath(protocol.AnchorPool), dstUrl, i, private.SequenceOptions{})
					if err == nil {
						break
					}
					
					slog.WarnContext(ctx, "Failed to check anchor", 
						"source", srcId, 
						"destination", dst.ID, 
						"number", i, 
						"peer", peer.ID, 
						"error", err)
				}
				
				if msg == nil {
					slog.WarnContext(ctx, "Unable to query anchor due to peer unavailability", 
						"source", srcId, 
						"destination", dst.ID, 
						"number", i)
					continue
				}
			} else {
				// Use the default client if no network peers
				slog.DebugContext(ctx, "Checking anchor", 
					"source", srcId, 
					"destination", dst.ID, 
					"number", i)
				
				msg, err = h.C2.Private().Sequence(ctx, srcUrl.JoinPath(protocol.AnchorPool), dstUrl, i, private.SequenceOptions{})
				if err != nil {
					// Check if it's a peer unavailability error
					if strings.Contains(err.Error(), "peer unavailable") {
						slog.WarnContext(ctx, "Unable to query sequence due to peer unavailability", 
							"source", srcId, 
							"destination", dst.ID, 
							"number", i)
						continue
					}
					
					slog.WarnContext(ctx, "Failed to query sequence",
						"source", srcId,
						"destination", dst.ID,
						"number", i,
						"error", err)
					continue
				}
			}
			
			// Add the message to our list
			messages = append(messages, msg)
			processedCount++
			
			slog.DebugContext(ctx, "Collected anchor", 
				"source", srcId, 
				"destination", dst.ID, 
				"anchor_height", i, 
				"txid", msg.ID)
		}
	}
	
	return messages, nil
}

// queryChainCountFromAnchorChain queries the count of an anchor chain using the approach from sequence.go
func (h *healer) queryChainCountFromAnchorChain(ctx context.Context, chainUrl *url.URL) (uint64, error) {
	slog.InfoContext(ctx, "Querying chain count", "chain", chainUrl)
	
	// Following sequence.go approach, use tryEach().QueryChain
	chainInfo, err := h.tryEach().QueryChain(ctx, chainUrl, &api.ChainQuery{Name: "anchor-sequence"})
	if err != nil {
		// Check if it's a peer unavailability error
		if strings.Contains(err.Error(), "peer unavailable") {
			slog.WarnContext(ctx, "Unable to query anchor sequence chain due to peer unavailability", 
				"chain", chainUrl)
			return 0, fmt.Errorf("peer unavailable: %w", err)
		}
		
		// Log the error details for debugging
		slog.ErrorContext(ctx, "Failed to query chain count with detailed error",
			"chain", chainUrl,
			"error", err,
			"error_type", fmt.Sprintf("%T", err))
		
		return 0, fmt.Errorf("failed to query chain count: %w", err)
	}
	
	// Log the chain count for debugging
	slog.InfoContext(ctx, "Successfully queried chain count",
		"chain", chainUrl,
		"count", chainInfo.Count)
	
	return chainInfo.Count, nil
}

// healSingleAnchor heals a single anchor between two partitions
func healSingleAnchor(src, dst *protocol.PartitionInfo, num uint64, txid *url.TxID) {
	// Get the healer instance
	h := getCurrentHealer()
	
	// Log the healing attempt
	slog.InfoContext(h.ctx, "Attempting to heal anchor",
		"node", h.network,
		"source", src.ID,
		"destination", dst.ID,
		"height", num,
		"txid", func() string {
			if txid == nil {
				return "unknown"
			}
			return txid.String()
		}())

	// Create a sequenced info struct for the healing process
	si := healing.SequencedInfo{
		Source:      src.ID,
		Destination: dst.ID,
		Number:      num,
		ID:          txid,
	}

	// Create a function to submit messages
	submitFunc := func(msgs ...messaging.Message) error {
		select {
		case h.submit <- msgs:
			return nil
		case <-h.ctx.Done():
			return h.ctx.Err()
		}
	}

	// Create arguments for the healing process
	args := healing.HealAnchorArgs{
		Client:       message.AddressedClient{Client: h.C2},
		Querier:      h.tryEach(),
		Submit:       submitFunc,
		NetInfo:      h.net,
		Known:        make(map[[32]byte]*protocol.Transaction),
		Pretend:      pretend,
		Wait:         waitForTxn,
	}

	// Call the healing function
	err := healing.HealAnchor(h.ctx, args, si)
	if err != nil {
		slog.ErrorContext(h.ctx, "Failed to heal anchor",
			"node", h.network,
			"source", src.ID,
			"destination", dst.ID,
			"height", num,
			"error", err)
		return
	}

	slog.InfoContext(h.ctx, "Successfully healed anchor",
		"node", h.network,
		"source", src.ID,
		"destination", dst.ID,
		"height", num)
}

func (h *healer) decodeHash(hash []byte) string {
	if hash == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%X", hash)
}

func (h *healer) decodeUint64(data []byte) uint64 {
	if len(data) < 8 {
		return 0
	}
	return binary.BigEndian.Uint64(data)
}
