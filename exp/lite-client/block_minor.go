package liteclient

import (
	"context"
	"fmt"
	"time"

	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
)

// Validate all minor blocks from the past 24h and their signatures.
func ValidateRecentMinorBlocks(ctx context.Context, cl *client.Client, authorities *AuthoritySet) error {
	oneDayAgo := time.Now().Add(-24 * time.Hour)

	startBlock, err := findMajorBlockByTime(ctx, cl, oneDayAgo)
	if err != nil {
		return fmt.Errorf("failed to find starting block: %v", err)
	}

	query := &client.MajorBlocksQuery{
		QueryPagination: client.QueryPagination{
			Start: uint64(startBlock),
		},
	}
	query.Url, err = parseUrl("acc://dn.acme")
	if err != nil {
		return fmt.Errorf("failed to parse DN URL: %v", err)
	}

	resp, err := cl.QueryMajorBlocks(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to query major blocks: %v", err)
	}
	if resp == nil || len(resp.Items) == 0 {
		return fmt.Errorf("no major blocks found")
	}

	for _, item := range resp.Items {
		majorBlock := convertToMajorBlockRecord(item)

		minorBlocks, err := getMinorBlocksForMajorBlock(ctx, cl, majorBlock)
		if err != nil {
			return fmt.Errorf("failed to get minor blocks for major block %d: %v", majorBlock.Index, err)
		}
		for _, minorBlock := range minorBlocks {
			valid, err := validateMinorBlockSignatures(ctx, cl, minorBlock, authorities)
			if err != nil {
				return fmt.Errorf("failed to validate signatures for minor block %d: %v", minorBlock.Index, err)
			}
			if !valid {
				return fmt.Errorf("invalid signatures for minor block %d", minorBlock.Index)
			}
			if !verifyMinorBlockInMajor(minorBlock, majorBlock) {
				return fmt.Errorf("minor block %d not correctly referenced in major block %d", minorBlock.Index, majorBlock.Index)
			}
		}
	}
	return nil
}

func getMinorBlocksForMajorBlock(ctx context.Context, cl *client.Client, majorBlock api.MajorBlockRecord) ([]api.MinorBlockRecord, error) {
	// TODO: Fetch minor blocks for a given major block
	return nil, nil
}

// validateMinorBlockSignatures confirms that the given minor block's root hash
// was signed by the correct authority set at the time.
// Used to verify the authenticity of a minor block.
func validateMinorBlockSignatures(ctx context.Context, cl *client.Client, minorBlock api.MinorBlockRecord, authorities *AuthoritySet) (bool, error) {
	return ValidateMinorBlockSignatures(ctx, cl, minorBlock, authorities)
}

func verifyMinorBlockInMajor(minorBlock api.MinorBlockRecord, majorBlock api.MajorBlockRecord) bool {
	// TODO: Check reference consistency
	return true
}
