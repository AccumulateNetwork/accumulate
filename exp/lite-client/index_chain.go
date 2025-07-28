// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package liteclient

import (
	"context"
	"encoding/binary"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// IndexEntry represents a decoded main-index entry that maps main chain positions to BVN anchor positions
type IndexEntry struct {
	Source uint64 // Position in the main chain
	Anchor uint64 // Position in the BVN anchor chain
}

// attemptRealBVNIndexLookup performs real BVN index chain lookup using the main-index chain.
// Following the healing pattern: use main-index root to find BVN anchor position.
// Reference: internal/core/healing/synthetic.go:buildSynthReceiptV2
func (hpg *HealingProofGenerator) attemptRealBVNIndexLookup(ctx context.Context, u *url.URL, mainAnchor []byte) (*merkle.Receipt, error) {
	// Quietly attempt real BVN index lookup

	// Step 1: Query account data to extract main-index root
	accountData, err := hpg.queryAccountData(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("failed to query account data: %w", err)
	}

	// Step 2: Extract main-index root from account data
	mainIndexRoot, err := hpg.extractMainIndexRoot(accountData)
	if err != nil {
		return nil, fmt.Errorf("failed to extract main-index root: %w", err)
	}

	fmt.Printf("Main-index root: %x\n", mainIndexRoot)

	// Step 3: Use main-index root to generate BVN receipt
	return hpg.generateBVNReceiptFromMainIndex(mainAnchor, mainIndexRoot)
}

// extractMainIndexRoot extracts the main-index root from account data.
// The main-index root contains the mapping information needed to find BVN anchor positions.
func (hpg *HealingProofGenerator) extractMainIndexRoot(accountData map[string]interface{}) ([]byte, error) {
	// Extract main-index root quietly

	// Look for main-index chain in the account chains
	if chains, exists := accountData["chains"]; exists {
		if chainsArray, ok := chains.([]interface{}); ok {
			for _, chain := range chainsArray {
				if chainMap, ok := chain.(map[string]interface{}); ok {
					if name, exists := chainMap["name"]; exists {
						if name == "main-index" {
							// Found main-index chain
							
							// Extract the latest root from the main-index chain
							if roots, exists := chainMap["roots"]; exists {
								if rootsArray, ok := roots.([]interface{}); ok && len(rootsArray) > 0 {
									// Get the latest root (last entry)
									if rootStr, ok := rootsArray[len(rootsArray)-1].(string); ok {
										// Decode hex string to bytes
										rootBytes := make([]byte, len(rootStr)/2)
										for i := 0; i < len(rootStr); i += 2 {
											var b byte
											fmt.Sscanf(rootStr[i:i+2], "%02x", &b)
											rootBytes[i/2] = b
										}
										return rootBytes, nil
									}
								}
							}
						}
					}
				}
			}
		}
	}

	return nil, fmt.Errorf("main-index root not found in account data")
}

// generateBVNReceiptFromMainIndex generates a BVN receipt using the main-index root.
// Following the healing pattern: mainIndex.Anchor tells us the position, then we query the actual BVN.
func (hpg *HealingProofGenerator) generateBVNReceiptFromMainIndex(mainAnchor []byte, mainIndexRoot []byte) (*merkle.Receipt, error) {
	fmt.Printf("\n--- Generating BVN Receipt from Main-Index ---\n")
	fmt.Printf("Main-index root length: %d bytes\n", len(mainIndexRoot))

	// Step 1: Decode the index entry from the main-index root
	indexEntry, err := hpg.decodeIndexEntry(mainIndexRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to decode index entry: %w", err)
	}

	fmt.Printf("Decoded index entry - Source: %d, Anchor: %d\n", indexEntry.Source, indexEntry.Anchor)

	// Step 2: Query the real BVN anchor chain at the specified position
	bvnReceipt, err := hpg.queryRealBVNAnchorChain(context.Background(), indexEntry.Anchor, mainAnchor)
	if err != nil {
		return nil, fmt.Errorf("failed to query BVN anchor chain: %w", err)
	}

	return bvnReceipt, nil
}

// decodeIndexEntry decodes a binary index entry into source and anchor positions.
// The index entry format follows the protocol.IndexEntry structure.
func (hpg *HealingProofGenerator) decodeIndexEntry(data []byte) (*IndexEntry, error) {
	fmt.Printf("\n--- Decoding Index Entry ---\n")
	fmt.Printf("Raw data: %x\n", data)

	if len(data) < 16 {
		return nil, fmt.Errorf("index entry too short: got %d bytes, need at least 16", len(data))
	}

	// Decode as little-endian uint64 values
	// First 8 bytes: source position
	// Next 8 bytes: anchor position
	source := binary.LittleEndian.Uint64(data[0:8])
	anchor := binary.LittleEndian.Uint64(data[8:16])

	fmt.Printf("Decoded - Source: %d, Anchor: %d\n", source, anchor)

	return &IndexEntry{
		Source: source,
		Anchor: anchor,
	}, nil
}
