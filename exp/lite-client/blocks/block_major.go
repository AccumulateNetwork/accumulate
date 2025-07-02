package blocks

import (
	"context"
	"encoding/json"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"

	sigs "gitlab.com/accumulatenetwork/accumulate/exp/lite-client/signatures"
	apiint "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
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

	query := createQueryMajorBlock(startIndex, &count, partitionUrl)
	fmt.Printf("Querying for major blocks starting at %d (count: %d)...\n", startIndex, count)

	// v3 Querier2 instance for future v3 API usage
	// TODO: Fill in the actual QuerierParams values as needed for your application
	restQuerier := apiint.NewQuerier(apiint.QuerierParams{
		Logger:    logging.NullLogger{},
		Database:  database.OpenInMemory(logging.NullLogger{}),
		Partition: "acc://bvn0.acme",
		Consensus: nil, // No real consensus service available
	})
	querier2 := api.Querier2{
		Querier: restQuerier,
	}

	resp, err := executeQueryMajorBlock(ctx, partitionUrl, querier2, query)
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

func createQueryMajorBlock(startIndex uint64, count *uint64, partitionUrl *accurl.URL) api.BlockQuery {
	major_query := api.BlockQuery{
		MajorRange: &api.RangeOptions{
			Start: startIndex,
			Count: count,
		},
	}
	return major_query
}

func executeQueryMajorBlock(ctx context.Context, url *accurl.URL, q api.Querier2, query api.BlockQuery) (*MajorBlockRecord, error) {
	return q.QueryMajorBlock(ctx, url, &query)
}

func processMajorBlock(resp *MajorBlockRecord) ([]map[string]interface{}, error) {
	if resp == nil {
		return nil, fmt.Errorf("no major block record returned")
	}

	// Marshal the MajorBlockRecord to JSON, then unmarshal to map[string]interface{}
	bz, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal MajorBlockRecord: %v", err)
	}

	raw := make(map[string]interface{})
	if err := json.Unmarshal(bz, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal MajorBlockRecord: %v", err)
	}

	// For v3, the block index field is likely "index" (per struct definition)
	if _, ok := raw["index"]; !ok {
		return nil, fmt.Errorf("major block missing 'index' field")
	}
	fmt.Printf("Retrieved major block %v\n", raw["index"])

	blocks := []map[string]interface{}{raw}
	return blocks, nil
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
	idx, ok := block["majorBlockIndex"].(float64)
	if !ok {
		return nil, fmt.Errorf("block missing or invalid majorBlockIndex field")
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
	return &sigs.AuthoritySet{Keys: keys, Threshold: uint64(threshold), Index: uint64(idx)}, nil
}

func BuildAuthorityTracker(authoritySets []*sigs.AuthoritySet) (*sigs.AuthorityTracker, error) {
	if len(authoritySets) == 0 {
		return nil, fmt.Errorf("authoritySets must be non-empty")
	}

	tracker := sigs.NewAuthorityTracker(authoritySets[0], authoritySets[0].Index)
	for i := 1; i < len(authoritySets); i++ {
		tracker.RecordChange(authoritySets[i].Index, authoritySets[i])
	}
	return tracker, nil
}
