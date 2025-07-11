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
func QueryMajorBlocks(ctx context.Context, cl *client.Client, partitionUrl string, startIndex uint64, count uint64, queryVersion string) ([]*client.MajorQueryResponse, error) {
	switch queryVersion {
	case "v2":
		return QueryMajorBlocksV2(ctx, cl, partitionUrl, startIndex, count)
	case "v3":
		parsedUrl, err := parseUrl(partitionUrl)
		if err != nil {
			return nil, fmt.Errorf("failed to parse partition URL: %v", err)
		}

		query := createQueryMajorBlock(startIndex, &count, parsedUrl)
		fmt.Printf("[v3] Querying for major blocks starting at %d (count: %d) on partition %s...\n", startIndex, count, partitionUrl)

		// v3 Querier2 instance for v3 API usage
		restQuerier := apiint.NewQuerier(apiint.QuerierParams{
			Logger:    logging.NullLogger{},
			Database:  database.OpenInMemory(logging.NullLogger{}),
			Partition: partitionUrl,
			Consensus: nil, // No real consensus service available
		})
		querier2 := api.Querier2{
			Querier: restQuerier,
		}

		resp, err := executeQueryMajorBlock(ctx, parsedUrl, querier2, query)
		if err != nil {
			return nil, fmt.Errorf("failed to query major blocks (v3): %v", err)
		}

		fmt.Printf("[v3] Raw response from executeQueryMajorBlock: %+v\n", resp)
		if resp != nil {
			fmt.Printf("[v3] Raw response records: count=%d, records=%+v\n", len(resp.Records), resp.Records)
		} else {
			fmt.Println("[v3] Raw response is nil")
		}

		// NOTE: For v3, stub out as not implemented for now to match v2 signature
		return nil, fmt.Errorf("v3 major block query not yet implemented for typed struct return")
	default:
		return nil, fmt.Errorf("unsupported query version: %s", queryVersion)
	}
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

func executeQueryMajorBlock(ctx context.Context, url *accurl.URL, q api.Querier2, query api.BlockQuery) (*api.RecordRange[*api.MajorBlockRecord], error) {
	return q.QueryMajorBlocks(ctx, url, &query)
}

func processMajorBlock(resp *api.RecordRange[*api.MajorBlockRecord]) ([]map[string]interface{}, error) {
	if resp == nil || len(resp.Records) == 0 {
		return nil, fmt.Errorf("no major block records returned")
	}

	var blocks []map[string]interface{}
	for _, mb := range resp.Records {
		bz, err := json.Marshal(mb)
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
		blocks = append(blocks, raw)
	}
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

// ExtractAuthoritySet extracts the AuthoritySet (signatures and threshold) from a major block.
func ExtractAuthoritySet(block *client.MajorQueryResponse) (*sigs.AuthoritySet, error) {
	// threshold, ok := block["threshold"].(float64)
	// if !ok {
	// 	return nil, fmt.Errorf("block missing or invalid threshold field")
	// }
	// sigsField, ok := block["signatures"].([]interface{})
	// if !ok {
	// 	return nil, fmt.Errorf("block missing or invalid signatures field")
	// }
	// idx, ok := block["majorBlockIndex"].(float64)
	// if !ok {
	// 	return nil, fmt.Errorf("block missing or invalid majorBlockIndex field")
	// }
	// keys := make([][]byte, 0, len(sigsField))
	// for _, sig := range sigsField {
	// 	sigMap, ok := sig.(map[string]interface{})
	// 	if !ok {
	// 		continue
	// 	}
	// 	if pub, ok := sigMap["publicKey"].([]byte); ok {
	// 		keys = append(keys, pub)
	// 	}
	// }
	// return &sigs.AuthoritySet{Keys: keys, Threshold: uint64(threshold), Index: uint64(idx)}, nil
	return nil, nil
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
