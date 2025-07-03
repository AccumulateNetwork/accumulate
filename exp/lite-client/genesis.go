package liteclient

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	blocks "gitlab.com/accumulatenetwork/accumulate/exp/lite-client/blocks"
	api "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	accurl "gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const genesisPartitionUrl = "acc://bvn0.acme"

// queryAccountAs queries an account and unmarshals the result into target.
func queryAccountAs(ctx context.Context, cl *client.Client, url string, target interface{}) error {
	parsedUrl, err := accurl.Parse(url)
	if err != nil {
		return fmt.Errorf("failed to parse partition URL: %v", err)
	}

	req := &api.GeneralQuery{
		UrlQuery: client.UrlQuery{
			Url: parsedUrl,
		},
	}
	_, err = cl.QueryAccountAs(ctx, req, target)
	return err
}

// GetFirstAuthorityUrl queries a partition account and returns the first authority URL.
func GetFirstAuthorityUrl(ctx context.Context, cl *client.Client, partitionUrl string) (string, error) {
	var auth protocol.AccountAuth
	if err := queryAccountAs(ctx, cl, partitionUrl, &auth); err != nil {
		return "", fmt.Errorf("failed to query partition account: %w", err)
	}
	if len(auth.Authorities) == 0 {
		return "", fmt.Errorf("no authorities found for partition %s", partitionUrl)
	}
	return auth.Authorities[0].Url.String(), nil
}

// RetrieveGenesisBlock fetches the genesis major block from a specific partition.
// Returns the raw block as a map[string]interface{}.
func RetrieveGenesisBlockAndAuthority(ctx context.Context, cl *client.Client) (map[string]interface{}, *protocol.KeyBook, *protocol.KeyPage, error) {
	genesisBlock, err := blocks.QueryMajorBlocks(ctx, cl, genesisPartitionUrl, 0, 1, "v2")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to query genesis block: %w", err)
	}
	if len(genesisBlock) == 0 {
		return nil, nil, nil, fmt.Errorf("no genesis block found")
	}
	block := genesisBlock[0]

	minorBlocks := 0
	if mb, ok := block["minorBlocks"]; ok {
		if mblist, ok := mb.([]interface{}); ok {
			minorBlocks = len(mblist)
		}
	}
	fmt.Printf("Genesis Block:\n  Index: %v\n  Time: %v\n  MinorBlocks: %d\n",
		block["majorBlockIndex"], block["majorBlockTime"], minorBlocks)

	bookUrl, err := GetFirstAuthorityUrl(ctx, cl, genesisPartitionUrl)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to get authority URL")
	}

	book, err := blocks.GetKeyBook(ctx, cl, bookUrl)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to get key book")
	}

	page, err := blocks.GetKeyPage(ctx, cl, bookUrl, 0)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to get key page")
	}

	return block, book, page, nil
}
