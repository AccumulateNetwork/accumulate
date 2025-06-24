package liteclient

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	accurl "gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// QueryMajorBlocks retrieves a paginated slice of major blocks from the given partition.
// If count=1, returns a slice with a single block.
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

// getKeyBook queries and returns a KeyBook given its URL.
func getKeyBook(ctx context.Context, cl *client.Client, urlStr string) (*protocol.KeyBook, error) {
	var book protocol.KeyBook
	err := queryAccountAs(ctx, cl, urlStr, &book)
	if err != nil {
		return nil, fmt.Errorf("failed to query key book: %v", err)
	}
	return &book, nil
}

// getKeyPage queries and returns a KeyPage given its KeyBook URL (and page index).
func getKeyPage(ctx context.Context, cl *client.Client, bookUrl string, pageIndex int) (*protocol.KeyPage, error) {
	pageUrl := fmt.Sprintf("%s/page/%d", bookUrl, pageIndex)
	var page protocol.KeyPage
	err := queryAccountAs(ctx, cl, pageUrl, &page)
	if err != nil {
		return nil, fmt.Errorf("failed to query key page: %v", err)
	}
	return &page, nil
}

func decodeItemToMap(item interface{}) (map[string]interface{}, error) {
	raw := make(map[string]interface{})
	bz, err := json.Marshal(item)
	if err != nil {
		return nil, errors.Wrap(err, "marshal item")
	}
	if err := json.Unmarshal(bz, &raw); err != nil {
		return nil, errors.Wrap(err, "unmarshal into map")
	}
	return raw, nil
}

func createQueryMajorBlock(startIndex uint64, count uint64, partitionUrl *accurl.URL) *client.MajorBlocksQuery {
	query := &client.MajorBlocksQuery{
		UrlQuery: client.UrlQuery{Url: partitionUrl},
		QueryPagination: client.QueryPagination{
			Start: startIndex,
			Count: count,
		},
	}
	return query
}

func executeQueryMajorBlock(ctx context.Context, cl *client.Client, query *client.MajorBlocksQuery) (*api.MultiResponse, error) {
	// Execute the query
	resp, err := cl.QueryMajorBlocks(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query major blocks: %v", err)
	}

	// Check if we have results
	if resp == nil || len(resp.Items) == 0 {
		return nil, fmt.Errorf("no major blocks found")
	}

	return resp, nil
}

func processMajorBlock(resp *api.MultiResponse) ([]map[string]interface{}, error) {
	var blocks []map[string]interface{}
	for i, item := range resp.Items {
		// Convert the interface{} to a map
		block := make(map[string]interface{})
		blockData, err := json.Marshal(item)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal block data: %v", err)
		}

		err = json.Unmarshal(blockData, &block)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal block data: %v", err)
		}

		// Validate that we have the expected fields
		majorBlockIndex, ok := block["majorBlockIndex"]
		if !ok {
			return nil, fmt.Errorf("block %d missing majorBlockIndex field", i)
		}

		fmt.Printf("Retrieved major block %v\n", majorBlockIndex)
		blocks = append(blocks, block)
	}
	return blocks, nil
}

func parseUrl(url string) (*accurl.URL, error) {
	partitionUrl, err := accurl.Parse(url)
	if err != nil {
		return nil, fmt.Errorf("failed to parse partition URL: %v", err)
	}
	return partitionUrl, nil
}

// queryAccountAs queries an account and unmarshals the result into out.
func queryAccountAs(ctx context.Context, cl *client.Client, urlStr string, out interface{}) error {
	partitionUrl, err := parseUrl(urlStr)
	if err != nil {
		return fmt.Errorf("failed to parse URL: %v", err)
	}
	query := &client.GeneralQuery{UrlQuery: client.UrlQuery{Url: partitionUrl}}
	_, err = cl.QueryAccountAs(ctx, query, out)
	if err != nil {
		return fmt.Errorf("failed to query account %s: %v", urlStr, err)
	}
	return nil
}

// getFirstAuthorityUrl extracts the first authority URL from an AccountAuth.
func getFirstAuthorityUrl(auth *protocol.AccountAuth) (string, error) {
	if len(auth.Authorities) == 0 {
		return "", fmt.Errorf("no authorities found")
	}
	return auth.Authorities[0].Url.String(), nil
}
