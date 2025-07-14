package blocks

import (
	"context"
	"encoding/json"
	"fmt"

	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	v2 "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	accurl "gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// QueryMajorBlocks retrieves a paginated slice of major blocks from the given partition.
// Each block contains all fields returned by the node, including signatures and threshold data.
// Use ExtractAuthoritySet to extract an AuthoritySet from a block for signature validation.
// QueryMajorBlocks is a simplified wrapper around the v2 API.
func QueryMajorBlocks(ctx context.Context, cl *v2.Client, partitionUrl string, startIndex uint64, count uint64) ([]*v2.MajorQueryResponse, error) {
	return QueryMajorBlocksV2(ctx, cl, partitionUrl, startIndex, count)
}

// QueryMajorBlocksV3 retrieves a paginated slice of major blocks from the given partition using the v3 API.
// Each block contains all fields returned by the node, including signatures and threshold data.
func QueryMajorBlocksV3(ctx context.Context, querier api.Querier, partitionUrl string, startIndex uint64, count uint64) ([]*api.MajorBlockRecord, error) {
	parsedUrl, err := parseUrl(partitionUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse partition URL: %w", err)
	}

	// In v3, we query the major block chain's anchor chain using the generic Query method
	query := &api.ChainQuery{
		Name:  "major-block",
		Range: &api.RangeOptions{Start: startIndex, Count: &count},
	}

	resp, err := querier.Query(ctx, parsedUrl, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query major block chain: %w", err)
	}

	recordRange, ok := resp.(*api.RecordRange[*api.MajorBlockRecord])
	if !ok {
		return nil, fmt.Errorf("unexpected response type: expected RecordRange[*api.MajorBlockRecord], got %T", resp)
	}

	return recordRange.Records, nil
}

func QueryMajorBlocksV2(ctx context.Context, cl *v2.Client, partitionUrl string, startIndex, count uint64) ([]*v2.MajorQueryResponse, error) {
	parsedUrl, err := accurl.Parse(partitionUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse partition URL: %v", err)
	}

	query := &v2.MajorBlocksQuery{
		QueryPagination: v2.QueryPagination{
			Start: startIndex,
			Count: count,
		},
		UrlQuery: v2.UrlQuery{
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
	var blocks []*v2.MajorQueryResponse
	for _, item := range resp.Items {
		data, err := json.Marshal(item)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal major block item: %w", err)
		}
		var block v2.MajorQueryResponse
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, fmt.Errorf("failed to unmarshal major block item: %w", err)
		}
		blocks = append(blocks, &block)
	}
	return blocks, nil
}

func parseUrl(urlStr string) (*accurl.URL, error) {
	return accurl.Parse(urlStr)
}
