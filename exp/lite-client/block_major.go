package liteclient

import (
	"context"
	"encoding/json"
	"fmt"

	apiv2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	accurl "gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// QueryMajorBlocks retrieves a paginated slice of major blocks from the given partition.
func QueryMajorBlocks(ctx context.Context, cl *client.Client, startIndex uint64, count uint64) ([]map[string]interface{}, error) {
	partitionUrl, err := parseUrl("acc://bvn0.acme")
	if err != nil {
		return nil, fmt.Errorf("failed to parse partition URL: %v", err)
	}

	query := createQueryMajorBlock(startIndex, count, partitionUrl)
	fmt.Printf("Querying for major blocks starting at %d (count: %d)...\n", startIndex, count)

	resp, err := executeQueryMajorBlock(ctx, cl, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query major blocks: %v", err)
	}

	blocks, err := processMajorBlock(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to process major blocks: %v", err)
	}

	fmt.Printf("Retrieved %d major blocks\n", len(blocks))
	return blocks, nil
}

func createQueryMajorBlock(startIndex uint64, count uint64, partitionUrl *accurl.URL) *client.MajorBlocksQuery {
	return &client.MajorBlocksQuery{
		QueryPagination: client.QueryPagination{
			Start: startIndex,
			Count: count,
		},
		UrlQuery: client.UrlQuery{
			Url: partitionUrl,
		},
	}
}

func executeQueryMajorBlock(ctx context.Context, cl *client.Client, query *client.MajorBlocksQuery) (*apiv2.MultiResponse, error) {
	return cl.QueryMajorBlocks(ctx, query)
}

func processMajorBlock(resp *apiv2.MultiResponse) ([]map[string]interface{}, error) {
	var blocks []map[string]interface{}
	for i, item := range resp.Items {
		raw := make(map[string]interface{})
		bz, err := json.Marshal(item)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal block %d: %v", i, err)
		}
		if err := json.Unmarshal(bz, &raw); err != nil {
			return nil, fmt.Errorf("failed to unmarshal block %d: %v", i, err)
		}
		if _, ok := raw["majorBlockIndex"]; !ok {
			return nil, fmt.Errorf("block %d missing majorBlockIndex field", i)
		}
		fmt.Printf("Retrieved major block %v\n", raw["majorBlockIndex"])
		blocks = append(blocks, raw)
	}
	return blocks, nil
}

func convertToMajorBlockRecord(item interface{}) api.MajorBlockRecord {
	var record api.MajorBlockRecord
	// Marshal the generic item back to JSON, then unmarshal into the strongly typed struct
	bz, err := json.Marshal(item)
	if err != nil {
		fmt.Printf("failed to marshal item for MajorBlockRecord: %v\n", err)
		return record
	}
	if err := json.Unmarshal(bz, &record); err != nil {
		fmt.Printf("failed to unmarshal item to MajorBlockRecord: %v\n", err)
		return record
	}
	return record
}

func validateMajorBlockSignatures(block map[string]interface{}, authorities *AuthoritySet) (bool, error) {
	rootHash, ok := block["rootHash"].([]byte)
	if !ok {
		return false, fmt.Errorf("failed to extract rootHash from block")
	}
	signatures, ok := block["signatures"].([]interface{})
	if !ok {
		return false, fmt.Errorf("failed to extract signatures from block")
	}
	return ValidateBlockSignatures(authorities, rootHash, signatures)
}
