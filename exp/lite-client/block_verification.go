package liteclient

import (
	"context"
	"fmt"

	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
)

// VerifyAllMajorBlockSignatures validates all major blocks from genesis to the latest, sequentially.
// It checks for sequential indices, validates signatures, and returns error on first failure.
func VerifyAllMajorBlockSignatures(
	ctx context.Context,
	cl *client.Client,
	authorityManager AuthorityManager,
) error {
	// Retrieve all major blocks from genesis (index 0)
	blocks, err := QueryMajorBlocks(ctx, cl, 0, ^uint64(0)) // ^uint64(0) is max uint64
	if err != nil {
		return fmt.Errorf("failed to query all major blocks: %w", err)
	}
	var prevIndex uint64 = 0
	for i, block := range blocks {
		record := convertToMajorBlockRecord(block)
		index := record.Index
		if i == 0 {
			prevIndex = index
		} else {
			if index != prevIndex+1 {
				return fmt.Errorf("major block indices not sequential: got %d after %d", index, prevIndex)
			}
			prevIndex = index
		}
		authoritySet, err := authorityManager.GetAuthorityAt(index)
		if err != nil {
			return fmt.Errorf("failed to get authority set for block %d: %w", index, err)
		}
		rootHash, ok := block["rootHash"].([]byte)
		if !ok {
			return fmt.Errorf("failed to extract rootHash for block %d", index)
		}
		signatures, ok := block["signatures"].([]interface{})
		if !ok {
			return fmt.Errorf("failed to extract signatures for block %d", index)
		}
		okSig, err := ValidateBlockSignatures(authoritySet, rootHash, signatures)
		if err != nil {
			return fmt.Errorf("failed to validate signatures for block %d: %w", index, err)
		}
		if !okSig {
			return fmt.Errorf("block %d failed signature validation", index)
		}
	}
	return nil
}

// VerifyMajorBlockSignaturesInRange queries major blocks in the given range and verifies their signatures.
// Returns an error as soon as a block fails verification.
func VerifyMajorBlockSignaturesInRange(
	ctx context.Context,
	cl *client.Client,
	startIndex, count uint64,
	authorityManager AuthorityManager,
) error {
	blocks, err := QueryMajorBlocks(ctx, cl, startIndex, count)
	if err != nil {
		return fmt.Errorf("failed to query major blocks: %w", err)
	}
	for _, block := range blocks {
		record := convertToMajorBlockRecord(block)
		index := record.Index
		authoritySet, err := authorityManager.GetAuthorityAt(index)
		if err != nil {
			return fmt.Errorf("failed to get authority set for block %d: %w", index, err)
		}

		ok, err := validateMajorBlockSignatures(block, authoritySet)
		if err != nil {
			return fmt.Errorf("block %d failed signature validation: %w", index, err)
		}
		if !ok {
			return fmt.Errorf("block %d failed signature validation: not enough valid signatures", index)
		}
	}
	return nil
}
