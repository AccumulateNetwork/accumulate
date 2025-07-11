package blocks

import (
	"context"
	"fmt"

	apiv3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// QueryAnchorMajorBlockChain queries the anchor pool's "major-block" chain for a given partition.
// It fetches the raw chain entries, which can be further parsed as anchor transactions.
func QueryAnchorMajorBlockChain(ctx context.Context, cl apiv3.Querier, partitionUrl string, count int) error {
	partition, err := url.Parse(partitionUrl)
	if err != nil {
		return fmt.Errorf("invalid partition url: %w", err)
	}
	anchorPool := partition.JoinPath("anchor-pool")

	// Use ChainQuery from pkg/api/v3
	c := uint64(count)
	chainQuery := &apiv3.ChainQuery{
		Name:  "major-block",
		Range: &apiv3.RangeOptions{Count: &c},
	}
	resp, err := cl.Query(ctx, anchorPool, chainQuery)
	if err != nil {
		return fmt.Errorf("failed to query anchor pool major-block chain: %w", err)
	}

	recordRange, ok := resp.(*apiv3.RecordRange[*apiv3.ChainEntryRecord[apiv3.Record]])
	if !ok {
		return fmt.Errorf("unexpected response type: %T", resp)
	}

	fmt.Printf("Found %d entries in %s#major-block chain\n", recordRange.Total, anchorPool)
	for i, rec := range recordRange.Records {
		fmt.Printf("Entry %d: %x\n", i, rec.Entry[:])
		// Optionally, try to unmarshal as protocol.BlockValidatorAnchor or DirectoryAnchor
		var anchor protocol.BlockValidatorAnchor
		if err := anchor.UnmarshalBinary(rec.Entry[:]); err == nil {
			fmt.Printf("  Parsed BlockValidatorAnchor: Block=%d, Root=%x\n", anchor.MinorBlockIndex, anchor.RootChainAnchor)
		} else {
			fmt.Printf("  (Could not parse as BlockValidatorAnchor: %v)\n", err)
		}
	}
	return nil
}
