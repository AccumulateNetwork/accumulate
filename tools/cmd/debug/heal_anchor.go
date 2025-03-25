// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	"github.com/spf13/cobra"
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
	if reader.ReadValue(1, func(r io.Reader) error {
		var data []byte
		var err error
		data, err = io.ReadAll(r)
		if err != nil {
			return err
		}
		q.SourceNetwork, err = url.Parse(string(data))
		return err
	}) {
		// Field was read
	}

	// Read the destination network URL
	if reader.ReadValue(2, func(r io.Reader) error {
		var data []byte
		var err error
		data, err = io.ReadAll(r)
		if err != nil {
			return err
		}
		q.DestinationNetwork, err = url.Parse(string(data))
		return err
	}) {
		// Field was read
	}

	// Read the include pending flag
	if reader.ReadValue(3, func(r io.Reader) error {
		var data []byte
		var err error
		data, err = io.ReadAll(r)
		if err != nil {
			return err
		}
		q.IncludePending = len(data) > 0 && data[0] != 0
		return nil
	}) {
		// Field was read
	}

	// Read the partition
	if reader.ReadValue(4, func(r io.Reader) error {
		var data []byte
		var err error
		data, err = io.ReadAll(r)
		if err != nil {
			return err
		}
		q.Partition = string(data)
		return nil
	}) {
		// Field was read
	}

	// Read the start
	if reader.ReadValue(5, func(r io.Reader) error {
		var data []byte
		var err error
		data, err = io.ReadAll(r)
		if err != nil {
			return err
		}
		q.Start = binary.BigEndian.Uint64(data)
		return nil
	}) {
		// Field was read
	}

	// Read the count
	if reader.ReadValue(6, func(r io.Reader) error {
		var data []byte
		var err error
		data, err = io.ReadAll(r)
		if err != nil {
			return err
		}
		q.Count = binary.BigEndian.Uint64(data)
		return nil
	}) {
		// Field was read
	}

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
	if reader.ReadValue(1, func(rd io.Reader) error {
		var data []byte
		var err error
		data, err = io.ReadAll(rd)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, &r.Pending)
	}) {
		// Field was read successfully, no additional action needed
	}
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

// healAnchors heals anchors between two partitions
func healAnchors(src, dst *protocol.PartitionInfo) {
	// Get the healer instance
	h := getCurrentHealer()
	
	// Create URLs for the source and destination partitions
	srcUrl := protocol.PartitionUrl(src.ID)
	
	slog.InfoContext(h.ctx, "Healing anchors between partitions",
		"node", h.network,
		"source", src.ID,
		"destination", dst.ID)

	// Try to cast to AnchorLedger, but handle gracefully if it's not
	var src2dst *protocol.AnchorLedger
	
	// First try to get the anchor ledger directly from the partition
	anchorLedgerUrl := protocol.PartitionUrl(dst.ID).JoinPath(protocol.AnchorPool)
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
	
	// Try to cast to AnchorLedger
	if al, ok := anchorAccount.Account.(*protocol.AnchorLedger); ok {
		src2dst = al
	} else {
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
	var anchorChain *protocol.PartitionSyntheticLedger
	for _, p := range src2dst.Sequence {
		if p.Url.String() == srcUrl.String() {
			anchorChain = p
			break
		}
	}

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

	// Check if we have any anchors in the destination
	if anchorChain.Delivered == 0 {
		// No anchors in the destination, start from the beginning
		firstMissingHeight = 1
		totalMissingAnchors = chainCount
	} else {
		// We have some anchors, check if we're missing any
		firstMissingHeight = anchorChain.Delivered + 1
		if chainCount >= firstMissingHeight {
			totalMissingAnchors = chainCount - anchorChain.Delivered
		}
	}

	// If we have missing anchors, try to heal them
	if totalMissingAnchors > 0 {
		slog.InfoContext(h.ctx, "Missing anchors detected",
			"node", h.network,
			"source", src.ID,
			"destination", dst.ID,
			"first_missing", firstMissingHeight,
			"total_missing", totalMissingAnchors,
			"chain_count", chainCount,
			"delivered", anchorChain.Delivered)

		// Update the global missing anchors map
		missingAnchorsMu.Lock()
		if missingAnchors[src.ID] == nil {
			missingAnchors[src.ID] = make(map[string]struct {
				Count             int
				FirstMissingHeight uint64
			})
		}
		missingAnchors[src.ID][dst.ID] = struct {
			Count             int
			FirstMissingHeight uint64
		}{
			Count:             int(totalMissingAnchors),
			FirstMissingHeight: firstMissingHeight,
		}
		missingAnchorsMu.Unlock()

		// If we're not in dry run mode, try to heal the anchors
		if !h.dryRun {
			// Limit the number of anchors to process in one cycle
			var batchSize uint64 = maxAnchorsPerCycle
			if totalMissingAnchors < batchSize {
				batchSize = totalMissingAnchors
			}
			
			// Query the missing anchor entries using our new method
			entries, err := h.queryChainEntriesFromAnchorChain(h.ctx, anchorChainUrl, firstMissingHeight, batchSize)
			if err != nil {
				slog.ErrorContext(h.ctx, "Failed to query anchor entries",
					"node", h.network,
					"source", src.ID,
					"destination", dst.ID,
					"error", err)
				return
			}
			
			if len(entries.Records) == 0 {
				slog.WarnContext(h.ctx, "No entries found despite missing anchors",
					"node", h.network,
					"source", src.ID,
					"destination", dst.ID,
					"missing_count", totalMissingAnchors)
				return
			}
			
			slog.InfoContext(h.ctx, "Found anchor entries to heal",
				"node", h.network,
				"source", src.ID,
				"destination", dst.ID,
				"count", len(entries.Records))
			
			// Process each entry and heal it
			for i, record := range entries.Records {
				// Try to extract the entry index and transaction ID
				if entry, ok := record.(*api.ChainEntryRecord[api.Record]); ok {
					entryIndex := entry.Index
					
					// Create a transaction ID from the entry hash
					var hash [32]byte
					copy(hash[:], entry.Entry[:])
					txid := srcUrl.WithTxID(hash)
					
					// Heal this specific anchor
					slog.InfoContext(h.ctx, "Healing anchor entry",
						"node", h.network,
						"source", src.ID,
						"destination", dst.ID,
						"index", entryIndex,
						"entry", fmt.Sprintf("%x", entry.Entry))
					
					// Call healSingleAnchor for this entry
					healSingleAnchor(src, dst, entryIndex, txid)
				} else {
					slog.WarnContext(h.ctx, "Unexpected entry type",
						"node", h.network,
						"source", src.ID,
						"destination", dst.ID,
						"index", firstMissingHeight+uint64(i),
						"type", fmt.Sprintf("%T", record))
				}
			}
		} else {
			slog.InfoContext(h.ctx, "Dry run: would heal anchors",
				"node", h.network,
				"source", src.ID,
				"destination", dst.ID,
				"count", totalMissingAnchors)
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

// queryChainEntriesFromAnchorChain queries entries from an anchor chain using the technique
// demonstrated in the test file.
func (h *healer) queryChainEntriesFromAnchorChain(ctx context.Context, chainUrl *url.URL, startIndex, count uint64) (*api.RecordRange[api.Record], error) {
	// Create a query for the chain entries with pagination
	entryQuery := &api.ChainQuery{
		Name: "anchor-sequence",
		Range: &api.RangeOptions{
			Start: startIndex,
			Count: &count,
		},
	}
	
	// Generate a cache key for this query
	cacheKey := fmt.Sprintf("%s-%d-%d", chainUrl.String(), startIndex, count)
	
	// Check if we have this query cached
	if h.cache != nil {
		if cached, ok := h.cache[cacheKey]; ok {
			slog.InfoContext(ctx, "Using cached chain entries",
				"chain", chainUrl,
				"start", startIndex,
				"count", count)
			return cached.(*api.RecordRange[api.Record]), nil
		}
	}
	
	slog.InfoContext(ctx, "Querying chain entries",
		"chain", chainUrl,
		"start", startIndex,
		"count", count)
	
	// Execute the query directly via JSON-RPC
	entryResult, err := h.C1.Query(ctx, chainUrl, entryQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query chain entries: %w", err)
	}
	
	// Try to cast the result to different possible types
	if entries, ok := entryResult.(*api.RecordRange[*api.ChainEntryRecord[api.Record]]); ok {
		// Convert to the generic RecordRange type
		genericEntries := &api.RecordRange[api.Record]{
			Total:   entries.Total,
			Records: make([]api.Record, len(entries.Records)),
		}
		
		for i, entry := range entries.Records {
			genericEntries.Records[i] = entry
		}
		
		// Cache the result
		if h.cache == nil {
			h.cache = make(map[string]interface{})
		}
		h.cache[cacheKey] = genericEntries
		
		return genericEntries, nil
	}
	
	// Try the generic RecordRange type
	if entries, ok := entryResult.(*api.RecordRange[api.Record]); ok {
		// Cache the result
		if h.cache == nil {
			h.cache = make(map[string]interface{})
		}
		h.cache[cacheKey] = entries
		
		return entries, nil
	}
	
	// If we reach here, we don't know how to handle this type
	return nil, fmt.Errorf("unexpected result type: %T", entryResult)
}

// queryChainCountFromAnchorChain queries the number of entries in an anchor chain
func (h *healer) queryChainCountFromAnchorChain(ctx context.Context, chainUrl *url.URL) (uint64, error) {
	// Create a cache key for this query
	cacheKey := fmt.Sprintf("%s-count", chainUrl.String())
	
	// Check if we have this query cached
	if h.cache != nil {
		if cached, ok := h.cache[cacheKey]; ok {
			slog.InfoContext(ctx, "Using cached chain count", "chain", chainUrl)
			return cached.(uint64), nil
		}
	}
	
	// Create a query for the chain
	chainQuery := &api.ChainQuery{
		Name: "anchor-sequence",
	}
	
	slog.InfoContext(ctx, "Querying chain count", "chain", chainUrl)
	
	// Execute the query directly via JSON-RPC
	chainInfo, err := h.C1.Query(ctx, chainUrl, chainQuery)
	if err != nil {
		return 0, fmt.Errorf("failed to query chain count: %w", err)
	}
	
	// Try to cast the result to a ChainRecord
	chainRecord, ok := chainInfo.(*api.ChainRecord)
	if !ok {
		return 0, fmt.Errorf("expected ChainRecord, got %T", chainInfo)
	}
	
	// Cache the result
	if h.cache == nil {
		h.cache = make(map[string]interface{})
	}
	h.cache[cacheKey] = chainRecord.Count
	
	return chainRecord.Count, nil
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
