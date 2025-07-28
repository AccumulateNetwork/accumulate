// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package liteclient implements healing-based proof generation for Accumulate accounts.
// This file contains legacy proof functions for backward compatibility.
// The main implementation has been modularized into:
// - bvn_anchor.go - BVN anchor chain functions
// - index_chain.go - Index chain decoding functions  
// - dn_anchor.go - DN anchor chain functions
package liteclient

import (
	"context"
	"fmt"

	v2api "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
)

// Legacy proof functions for backward compatibility.
// New code should use the modular implementation in the separate files.



// FetchProof is a legacy wrapper function that uses the healing-based proof generator.
// This maintains backward compatibility for existing tests while using the new implementation.
func FetchProof(accountURL string) (*VerifiedAccount, error) {
	// Create a new v2 API client
	client, err := v2api.New("https://mainnet.accumulatenetwork.io/v2")
	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %w", err)
	}

	// Create healing proof generator
	hpg, err := NewHealingProofGenerator(client)
	if err != nil {
		return nil, fmt.Errorf("failed to create healing proof generator: %w", err)
	}
	defer hpg.Close()

	// Generate proof using the healing approach
	return hpg.GenerateProof(context.Background(), accountURL)
}

// VerifyProof is a legacy wrapper function for receipt validation.
// New code should use the modular implementation.
func VerifyProof(receipt *merkle.Receipt, accountURL string, expectedRoot []byte) (bool, error) {
	if receipt == nil {
		return false, fmt.Errorf("receipt is nil")
	}

	// Validate the receipt structure
	if !receipt.Validate(nil) {
		return false, fmt.Errorf("receipt validation failed")
	}

	return true, nil
}
