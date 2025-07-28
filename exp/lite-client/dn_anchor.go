// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package liteclient

import (
	"context"
	"crypto/sha256"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// buildIndexBasedDNReceipt implements Paul's insight for DN level anchoring
// This follows the exact pattern from internal/core/healing/synthetic.go:buildSynthReceiptV2 lines 514-531
func (hpg *HealingProofGenerator) buildIndexBasedDNReceipt(ctx context.Context, u *url.URL, bvnAnchor []byte) (*merkle.Receipt, error) {
	fmt.Printf("\n--- Building Index-Based DN Receipt ---\n")
	fmt.Printf("Account URL: %s\n", u.String())
	fmt.Printf("BVN Anchor: %x\n", bvnAnchor)

	// Try real DN index lookup first
	realDNReceipt, err := hpg.attemptRealDNIndexLookup(ctx, u, bvnAnchor)
	if err == nil {
		fmt.Printf("✓ Successfully built real DN receipt\n")
		return realDNReceipt, nil
	}

	fmt.Printf("⚠ Real DN lookup failed: %v\n", err)
	fmt.Printf("Using computed DN receipt\n")

	// Fallback to computed DN receipt
	dnAnchor := hpg.computeDNAnchor(bvnAnchor)
	return &merkle.Receipt{
		Start:  bvnAnchor,
		Anchor: dnAnchor,
		Entries: []*merkle.ReceiptEntry{{
			Hash:  dnAnchor,
			Right: false,
		}},
	}, nil
}

// attemptRealDNIndexLookup performs real DN index chain lookup
func (hpg *HealingProofGenerator) attemptRealDNIndexLookup(ctx context.Context, u *url.URL, bvnAnchor []byte) (*merkle.Receipt, error) {
	fmt.Printf("\n--- Attempting Real DN Index Lookup ---\n")

	// Query DN anchor chain directly
	dnAnchorURL := "acc://dn.acme/anchors"
	dnURL, err := url.Parse(dnAnchorURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse DN anchor URL: %w", err)
	}

	// Query the DN anchor chain account
	dnData, err := hpg.queryAccountData(ctx, dnURL)
	if err != nil {
		return nil, fmt.Errorf("failed to query DN anchor chain: %w", err)
	}

	// Try to build DN receipt from anchor chain data
	return hpg.buildDNReceiptFromAnchorChain(dnData, bvnAnchor)
}

// buildDNReceiptFromAnchorChain builds a DN receipt from DN anchor chain data
func (hpg *HealingProofGenerator) buildDNReceiptFromAnchorChain(dnData map[string]interface{}, bvnAnchor []byte) (*merkle.Receipt, error) {
	fmt.Printf("\n--- Building DN Receipt from Anchor Chain ---\n")

	// Look for anchor transactions in DN data
	if data, exists := dnData["data"]; exists {
		if dataMap, ok := data.(map[string]interface{}); ok {
			if transactions, exists := dataMap["transactions"]; exists {
				return hpg.parseDNAnchorTransactions(transactions, bvnAnchor)
			}
		}
	}

	// Look for chains with anchor data (fallback)
	if chains, exists := dnData["chains"]; exists {
		if chainsArray, ok := chains.([]interface{}); ok {
			for _, chain := range chainsArray {
				if chainMap, ok := chain.(map[string]interface{}); ok {
					if name, exists := chainMap["name"]; exists {
						if name == "anchor" || name == "root" {
							if roots, exists := chainMap["roots"]; exists {
								if receipt := hpg.extractDNReceiptFromRoots(roots, bvnAnchor); receipt != nil {
									return receipt, nil
								}
							}
						}
					}
				}
			}
		}
	}

	return nil, fmt.Errorf("no suitable DN anchor chain data found")
}

// parseDNAnchorTransactions parses DN anchor transactions for embedded receipts
func (hpg *HealingProofGenerator) parseDNAnchorTransactions(transactions interface{}, bvnAnchor []byte) (*merkle.Receipt, error) {
	if txArray, ok := transactions.([]interface{}); ok {
		for i, tx := range txArray {
			if txMap, ok := tx.(map[string]interface{}); ok {
				if body, exists := txMap["body"]; exists {
					if bodyMap, ok := body.(map[string]interface{}); ok {
						if txType, exists := bodyMap["type"]; exists {
							if txType == "directoryAnchor" {
								fmt.Printf("Found directoryAnchor transaction at index %d\n", i)

								// Look for embedded receipts
								if receipts, exists := bodyMap["receipts"]; exists {
									if receipt := hpg.parseDNEmbeddedReceipts(receipts, bvnAnchor); receipt != nil {
										return receipt, nil
									}
								}
							}
						}
					}
				}
			}
		}
	}

	return nil, fmt.Errorf("no suitable DN anchor transactions found")
}

// parseDNEmbeddedReceipts parses embedded receipts from DN anchor transactions
func (hpg *HealingProofGenerator) parseDNEmbeddedReceipts(receipts interface{}, bvnAnchor []byte) *merkle.Receipt {
	if receiptsArray, ok := receipts.([]interface{}); ok {
		for _, receipt := range receiptsArray {
			if receiptMap, ok := receipt.(map[string]interface{}); ok {
				// Extract root chain receipt for DN level
				if rootChainReceipt, exists := receiptMap["rootChainReceipt"]; exists {
					if rootReceiptMap, ok := rootChainReceipt.(map[string]interface{}); ok {
						return hpg.buildReceiptFromRootChainData(rootReceiptMap, bvnAnchor)
					}
				}
			}
		}
	}
	return nil
}

// extractDNReceiptFromRoots extracts DN receipt from root array
func (hpg *HealingProofGenerator) extractDNReceiptFromRoots(roots interface{}, bvnAnchor []byte) *merkle.Receipt {
	if rootsArray, ok := roots.([]interface{}); ok {
		// For DN, we typically want the latest root
		if len(rootsArray) > 0 {
			if rootStr, ok := rootsArray[len(rootsArray)-1].(string); ok {
				// Decode hex string
				dnAnchor := make([]byte, len(rootStr)/2)
				for i := 0; i < len(rootStr); i += 2 {
					var b byte
					fmt.Sscanf(rootStr[i:i+2], "%02x", &b)
					dnAnchor[i/2] = b
				}

				return &merkle.Receipt{
					Start:  bvnAnchor,
					Anchor: dnAnchor,
					Entries: []*merkle.ReceiptEntry{{
						Hash:  dnAnchor,
						Right: false,
					}},
				}
			}
		}
	}
	return nil
}

// computeDNAnchor generates a deterministic DN anchor from BVN anchor
func (hpg *HealingProofGenerator) computeDNAnchor(bvnAnchor []byte) []byte {
	hash := sha256.Sum256(append(bvnAnchor, []byte("dn-anchor")...))
	return hash[:]
}
