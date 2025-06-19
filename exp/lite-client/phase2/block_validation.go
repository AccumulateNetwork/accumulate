package liteclient

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const genesisPartitionUrl = "acc://bvn0.acme"

// RetrieveGenesisBlock fetches the genesis major block from a specific partition.
// Returns the raw block as a map[string]interface{}.
func RetrieveGenesisBlock(ctx context.Context, cl *client.Client) (map[string]interface{}, error) {
	partitionUrl, err := parsePartitionURL()
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse partition URL")
	}

	query := &client.MajorBlocksQuery{
		UrlQuery: client.UrlQuery{Url: partitionUrl},
		QueryPagination: client.QueryPagination{
			Start: 0,
			Count: 1,
		},
	}

	resp, err := cl.QueryMajorBlocks(ctx, query)
	if err != nil {
		return nil, errors.Wrap(err, "failed to query genesis block")
	}
	if resp == nil || len(resp.Items) == 0 {
		return nil, errors.New("no genesis block found")
	}

	block, err := decodeItemToMap(resp.Items[0])
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode genesis block")
	}

	fmt.Printf("Genesis Block:\n  Index: %v\n  Time: %v\n  MinorBlocks: %d\n",
		block["majorBlockIndex"], block["majorBlockTime"], len(block["minorBlocks"].([]interface{})))

	return block, nil
}

// RetrieveGenesisAuthority fetches the KeyBook and first KeyPage that
// originally signed the genesis block.
func RetrieveGenesisAuthority(ctx context.Context, cl *client.Client) (*protocol.KeyBook, *protocol.KeyPage, error) {
	partitionUrl, err := parsePartitionURL()
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to parse partition URL")
	}

	// Query the partition's account data to discover its authorities
	var partition protocol.AccountAuth
	query := &client.GeneralQuery{
		UrlQuery: client.UrlQuery{Url: partitionUrl},
	}
	_, err = cl.QueryAccountAs(ctx, query, &partition)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to query partition account")
	}
	if len(partition.Authorities) == 0 {
		return nil, nil, errors.New("partition has no authorities")
	}

	// The first authority URL is the KeyBook
	bookUrl := partition.Authorities[0].Url

	// Fetch the KeyBook
	var book protocol.KeyBook
	bookQuery := &client.GeneralQuery{
		UrlQuery: client.UrlQuery{Url: bookUrl},
	}
	_, err = cl.QueryAccountAs(ctx, bookQuery, &book)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to query key book")
	}

	// Manually construct the URL for the first KeyPage (page 0)
	pageUrlStr := fmt.Sprintf("%s/page/0", book.Url.String())
	pageUrl, err := url.Parse(pageUrlStr)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to parse key page URL")
	}

	// Fetch the first KeyPage
	var page protocol.KeyPage
	pageQuery := &client.GeneralQuery{
		UrlQuery: client.UrlQuery{Url: pageUrl},
	}
	_, err = cl.QueryAccountAs(ctx, pageQuery, &page)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to query key page")
	}

	return &book, &page, nil
}

// FetchMajorBlocks retrieves a paginated slice of major blocks from the given partition,
// starting at 'start' and returning up to 'count' blocks.
// Used to obtain a range of major blocks for validation or inspection.
//
// TODO: Implement API call to fetch major blocks for the specified partition.
func FetchMajorBlocks(partition string, start, count int) ([]*client.MajorBlocksQuery, error) {
	return nil, fmt.Errorf("not implemented: FetchMajorBlocks for partition %s, start %d, count %d", partition, start, count)
}

// ValidateMajorBlockSignature confirms that the given major block's root hash
// was signed by the correct authority set at the time.
// Used to verify the authenticity of a major block.
//
// TODO: Implement signature validation logic using the provided authority set.
func ValidateMajorBlockSignature(block *client.MajorBlocksQuery, authoritySet *AuthoritySet) error {
	return fmt.Errorf("not implemented: ValidateMajorBlockSignature for block %v", block)
}

// VerifyMajorBlockSequence ensures that the provided major blocks are sequentially numbered
// and that their timestamps follow the expected schedule.
// Used to check for missing or out-of-order blocks.
//
// TODO: Implement sequence and timestamp validation for major blocks.
func VerifyMajorBlockSequence(blocks []*client.MajorBlocksQuery) error {
	return fmt.Errorf("not implemented: VerifyMajorBlockSequence for %d blocks", len(blocks))
}

// TrackAuthorityChanges builds up a timeline of authority keybook/keypage changes
// across the provided major blocks. This is necessary for dynamic signature verification
// as authorities may change over time.
//
// TODO: Implement authority tracking logic to handle keybook/keypage updates.
func TrackAuthorityChanges(blocks []*client.MajorBlocksQuery) (*AuthorityTracker, error) {
	return nil, fmt.Errorf("not implemented: TrackAuthorityChanges for %d blocks", len(blocks))
}

func parsePartitionURL() (*url.URL, error) {
	return url.Parse(genesisPartitionUrl)
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
