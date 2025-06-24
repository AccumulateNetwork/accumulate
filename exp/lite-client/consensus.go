package liteclient

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// UpdateWithBlock applies authority changes from a new block to the tracker.
func (t *AuthorityTracker) UpdateWithBlock(block interface{}) error {
	// TODO: Detect authority/key page changes in the block (e.g., by scanning block transactions for KeyPage/KeyBook updates)
	// This is a placeholder for actual authority change detection logic, which depends on block structure and protocol.
	// If an authority change is detected, call t.RecordChange(blockIndex, newAuth)

	return nil
}

// GetEffectiveAuthority returns the authority set (KeyPage) active at a given block height.
func (t *AuthorityTracker) GetEffectiveAuthority(height int64) (*protocol.KeyPage, error) {
	// Find the closest AuthoritySet at or before the given height
	var (
		closestIndex uint64 = 0
		found        bool   = false
	)
	for idx := range t.history {
		if int64(idx) <= height && (!found || idx > closestIndex) {
			closestIndex = idx
			found = true
		}
	}
	if !found {
		return nil, fmt.Errorf("no authority set found for block height %d", height)
	}
	auth := t.history[closestIndex]
	// Convert AuthoritySet to KeyPage
	page := &protocol.KeyPage{
		AcceptThreshold: auth.Threshold,
		Keys:           make([]*protocol.KeySpec, len(auth.Keys)),
	}
	for i, key := range auth.Keys {
		page.Keys[i] = &protocol.KeySpec{PublicKeyHash: key}
	}
	return page, nil
}

func ValidateMajorBlockChain(blocks []*messaging.MakeMajorBlock, genesisAuthority *AuthoritySet) error {
	if len(blocks) == 0 {
		return nil // Nothing to validate
	}

	// Initialize authority tracker
	tracker := NewAuthorityTracker(genesisAuthority, blocks[0].MajorBlockIndex)
	validator := &SignerValidator{}

	for i, block := range blocks {
		blockIndex := block.MajorBlockIndex
		blockTime := block.MajorBlockTime

		// 1. Validate block sequence and timestamp
		if i > 0 {
			prev := blocks[i-1]
			if blockIndex != prev.MajorBlockIndex+1 {
				return fmt.Errorf("block sequence error: block %d follows %d", blockIndex, prev.MajorBlockIndex)
			}
			if blockTime.Before(prev.MajorBlockTime) {
				return fmt.Errorf("block time error: block %d time %v is before previous block %v", blockIndex, blockTime, prev.MajorBlockTime)
			}
		}

		// 2. Get current authority set
		authSet, err := tracker.GetAuthorityAt(blockIndex)
		if err != nil {
			return fmt.Errorf("authority set not found for block %d: %w", blockIndex, err)
		}

		// 3. Extract signatures from envelope (placeholder: MakeMajorBlock does not contain signatures directly)
		// TODO: Replace 'block' with the correct envelope containing signatures for this major block
		sigs, err := validator.ExtractSignaturesFromEnvelope(block)
		if err != nil {
			return fmt.Errorf("failed to extract signatures from block %d: %w", blockIndex, err)
		}
		if len(sigs) == 0 {
			return fmt.Errorf("no signatures found in block %d", blockIndex)
		}

		// 4. Extract root hash (placeholder, actual extraction depends on block structure)
		// TODO: Replace with actual root hash extraction
		var rootHash []byte
		// rootHash = block.StateTreeHash or similar

		// 5. Validate signatures against authority set
		validSigCount := 0
		for _, sig := range sigs {
			keySig, ok := interface{}(sig).(protocol.KeySignature)
			if !ok {
				continue // skip if not a KeySignature
			}
			okSig, err := VerifySignatureAgainstAuthoritySet(keySig, rootHash, authSet)
			if okSig && err == nil {
				validSigCount++
			}
		}
		if uint64(validSigCount) < authSet.Threshold {
			return fmt.Errorf("insufficient valid signatures for block %d: got %d, need %d", blockIndex, validSigCount, authSet.Threshold)
		}

		// 6. Apply authority changes if present
		if err := tracker.UpdateWithBlock(block); err != nil {
			return fmt.Errorf("failed to update authority tracker at block %d: %w", blockIndex, err)
		}
	}

	return nil
}
