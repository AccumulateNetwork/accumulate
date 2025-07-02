package blocks

import (
	"context"
	"encoding/json"
	"fmt"

	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	accurl "gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// QueryMajorBlocksV2 retrieves a paginated slice of major blocks using the v2 API.
// Each block is returned as a map[string]interface{} for compatibility with downstream code.
func QueryMajorBlocksV2(ctx context.Context, cl *client.Client, partitionUrl string, startIndex, count uint64) ([]map[string]interface{}, error) {
	parsedUrl, err := accurl.Parse(partitionUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse partition URL: %v", err)
	}

	query := &client.MajorBlocksQuery{
		QueryPagination: client.QueryPagination{
			Start: startIndex,
			Count: count,
		},
		UrlQuery: client.UrlQuery{
			Url: parsedUrl,
		},
	}

	resp, err := cl.QueryMajorBlocks(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query major blocks (v2): %v", err)
	}

	if resp == nil || len(resp.Items) == 0 {
		return nil, fmt.Errorf("no major block records returned (v2)")
	}

	var blocks []map[string]interface{}
	for _, mb := range resp.Items {
		bz, err := json.Marshal(mb)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal MajorBlockRecord (v2): %v", err)
		}
		raw := make(map[string]interface{})
		if err := json.Unmarshal(bz, &raw); err != nil {
			return nil, fmt.Errorf("failed to unmarshal MajorBlockRecord (v2): %v", err)
		}
		// v2 field name is likely "majorBlockIndex"
		if _, ok := raw["majorBlockIndex"]; !ok {
			return nil, fmt.Errorf("major block missing 'majorBlockIndex' field (v2)")
		}
		blocks = append(blocks, raw)
	}
	return blocks, nil
}
