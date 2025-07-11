package blocks

import (
	"context"
	"encoding/json"
	"fmt"

	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	accurl "gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// QueryMajorBlocksV2 retrieves a paginated slice of major blocks using the v2 API.
// Each block is returned as a typed *client.MajorQueryResponse for structured access.
func QueryMajorBlocksV2(ctx context.Context, cl *client.Client, partitionUrl string, startIndex, count uint64) ([]*client.MajorQueryResponse, error) {
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

	// Unmarshal items into []*api.MajorQueryResponse
	var blocks []*client.MajorQueryResponse
	for _, item := range resp.Items {
		data, err := json.Marshal(item)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal major block item: %w", err)
		}
		var block client.MajorQueryResponse
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, fmt.Errorf("failed to unmarshal major block item: %w", err)
		}
		blocks = append(blocks, &block)
	}
	return blocks, nil
}
