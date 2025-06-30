package blocks

import (
	"context"
	"encoding/json"
	"fmt"

	sigs "gitlab.com/accumulatenetwork/accumulate/exp/lite-client/signatures"
	apiv2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	accurl "gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// QueryMajorBlocks retrieves a paginated slice of major blocks from the given partition.
// Each block contains all fields returned by the node, including signatures and threshold data.
// Use ExtractAuthoritySet to extract an AuthoritySet from a block for signature validation.
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

func validateMajorBlockSignatures(block map[string]interface{}, authorities *sigs.AuthoritySet) (bool, error) {
	rootHash, ok := block["rootHash"].([]byte)
	if !ok {
		return false, fmt.Errorf("failed to extract rootHash from block")
	}
	signatures, ok := block["signatures"].([]interface{})
	if !ok {
		return false, fmt.Errorf("failed to extract signatures from block")
	}
	return sigs.ValidateBlockSignatures(authorities, rootHash, signatures)
}

// FetchLatestMajorBlockRootHash fetches the latest major block and returns its root hash.
func FetchLatestMajorBlockRootHash(ctx context.Context, cl *client.Client) ([]byte, error) {
	blocks, err := ListMajorBlocks(ctx, cl)
	if err != nil || len(blocks) == 0 {
		return nil, fmt.Errorf("failed to list major blocks: %w", err)
	}
	latest := blocks[len(blocks)-1]
	rootHash, ok := latest["rootHash"].([]byte)
	if !ok {
		return nil, fmt.Errorf("failed to extract rootHash from latest major block")
	}
	return rootHash, nil
}

func ListMajorBlocks(ctx context.Context, cl *client.Client) ([]map[string]interface{}, error) {
	// By default, fetch a reasonable number of recent major blocks (e.g., 20)
	const defaultCount = 20
	return QueryMajorBlocks(ctx, cl, 0, defaultCount)
}

// QueryMajorBlock retrieves a single major block by index and returns the block and its AuthoritySet.
func QueryMajorBlock(ctx context.Context, cl *client.Client, index uint64) (map[string]interface{}, *sigs.AuthoritySet, error) {
	blocks, err := QueryMajorBlocks(ctx, cl, index, 1)
	if err != nil {
		return nil, nil, err
	}
	if len(blocks) == 0 {
		return nil, nil, fmt.Errorf("no major block found at index %d", index)
	}
	block := blocks[0]
	authSet, err := ExtractAuthoritySet(block)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to extract AuthoritySet: %w", err)
	}
	return block, authSet, nil
}

// ExtractAuthoritySet extracts the AuthoritySet (signatures and threshold) from a major block.
func ExtractAuthoritySet(block map[string]interface{}) (*sigs.AuthoritySet, error) {
	threshold, ok := block["threshold"].(float64)
	if !ok {
		return nil, fmt.Errorf("block missing or invalid threshold field")
	}
	sigsField, ok := block["signatures"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("block missing or invalid signatures field")
	}
	keys := make([][]byte, 0, len(sigsField))
	for _, sig := range sigsField {
		sigMap, ok := sig.(map[string]interface{})
		if !ok {
			continue
		}
		if pub, ok := sigMap["publicKey"].([]byte); ok {
			keys = append(keys, pub)
		}
	}
	return &sigs.AuthoritySet{Keys: keys, Threshold: uint64(threshold)}, nil
}

// BuildAuthorityTracker builds an AuthorityTracker for all blocks (map[index, AuthoritySet]).
func BuildAuthorityTracker(blocks []map[string]interface{}) (*sigs.AuthorityTracker, error) {
	if len(blocks) == 0 {
		return nil, fmt.Errorf("no blocks provided")
	}
	// Use the AuthoritySet from the first block as the initial set
	firstIdx, ok := blocks[0]["majorBlockIndex"].(float64)
	if !ok {
		return nil, fmt.Errorf("first block missing or invalid majorBlockIndex field")
	}
	initialSet, err := ExtractAuthoritySet(blocks[0])
	if err != nil {
		return nil, fmt.Errorf("failed to extract authority set for first block: %w", err)
	}
	tracker := sigs.NewAuthorityTracker(initialSet, uint64(firstIdx))
	for _, blk := range blocks[1:] {
		idx, ok := blk["majorBlockIndex"].(float64)
		if !ok {
			return nil, fmt.Errorf("block missing or invalid majorBlockIndex field")
		}
		authSet, err := ExtractAuthoritySet(blk)
		if err != nil {
			return nil, fmt.Errorf("failed to extract authority set for block %v: %w", idx, err)
		}
		tracker.RecordChange(uint64(idx), authSet)
	}
	return tracker, nil
}
