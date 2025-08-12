// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"fmt"
	"os"
	
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// DebugGetDnHeight is a debug version to trace what's happening
func (s *NetworkService) DebugGetDnHeight(batch *database.Batch) (uint64, error) {
	debug := os.Getenv("DEBUG_DN_HEIGHT") != ""
	
	if debug {
		fmt.Printf("[DEBUG] getDnHeight called with partition: %s\n", s.partition)
	}
	
	anchorPoolUrl := protocol.PartitionUrl(s.partition).JoinPath(protocol.AnchorPool)
	if debug {
		fmt.Printf("[DEBUG] Looking for anchors at: %s\n", anchorPoolUrl)
	}
	
	c := batch.Account(anchorPoolUrl).MainChain()
	head, err := c.Head().Get()
	if err != nil {
		if debug {
			fmt.Printf("[DEBUG] Error getting chain head: %v\n", err)
		}
		return 0, errors.UnknownError.WithFormat("load anchor ledger main chain head: %w", err)
	}
	
	if debug {
		fmt.Printf("[DEBUG] Main chain has %d entries\n", head.Count)
	}
	
	foundCount := 0
	for i := head.Count - 1; i >= 0 && i >= head.Count-100; i-- { // Check last 100 entries
		entry, err := c.Entry(i)
		if err != nil {
			if debug && i == head.Count-1 {
				fmt.Printf("[DEBUG] Error getting entry %d: %v\n", i, err)
			}
			continue
		}

		var msg *messaging.TransactionMessage
		err = batch.Message2(entry).Main().GetAs(&msg)
		if err != nil {
			if debug && i == head.Count-1 {
				fmt.Printf("[DEBUG] Error getting message for entry %d: %v\n", i, err)
			}
			continue
		}

		if msg.Transaction == nil || msg.Transaction.Body == nil {
			continue
		}
		
		// Check what type of transaction this is
		bodyType := msg.Transaction.Body.Type()
		if debug && foundCount < 5 {
			fmt.Printf("[DEBUG] Entry %d: Transaction type = %v\n", i, bodyType)
			foundCount++
		}
		
		body, ok := msg.Transaction.Body.(*protocol.DirectoryAnchor)
		if ok {
			if debug {
				fmt.Printf("[DEBUG] ✅ Found DirectoryAnchor at entry %d with MinorBlockIndex: %d\n", i, body.MinorBlockIndex)
			}
			return body.MinorBlockIndex, nil
		}
	}
	
	if debug {
		fmt.Printf("[DEBUG] ❌ No DirectoryAnchor found in last 100 entries\n")
		fmt.Printf("[DEBUG] This means:\n")
		fmt.Printf("[DEBUG]   - Either we're looking in wrong partition (%s)\n", s.partition)
		fmt.Printf("[DEBUG]   - Or DirectoryAnchors are stored differently\n")
		fmt.Printf("[DEBUG]   - Or the chain is not being updated\n")
	}
	
	return 0, nil
}