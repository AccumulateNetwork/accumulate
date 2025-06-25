package liteclient

import (
	"context"
	"fmt"

	apiv2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
)

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
		signatures, err := FetchBlockSignatures(ctx, cl, index)
		if err != nil {
			return fmt.Errorf("failed to fetch signatures for block %d: %w", index, err)
		}
		ok, err := ValidateBlockSignatures(ctx, cl, index, authoritySet, signatures)
		if err != nil {
			return fmt.Errorf("failed to validate signatures for block %d: %w", index, err)
		}
		if !ok {
			return fmt.Errorf("block %d failed signature validation", index)
		}
	}
	return nil
}

// FetchBlockSignatures should be implemented elsewhere.
// func FetchBlockSignatures(ctx context.Context, cl *client.Client, blockIndex uint64) ([]Signature, error)
// ValidateBlockSignatures should be implemented elsewhere.
// func ValidateBlockSignatures(ctx context.Context, cl *client.Client, blockIndex uint64, authorities *AuthoritySet, signatures []Signature) (bool, error)
