package liteclient

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const genesisPartitionUrl = "acc://bvn0.acme"

// RetrieveGenesisBlock fetches the genesis major block from a specific partition.
// Returns the raw block as a map[string]interface{}.
func RetrieveGenesisBlock(ctx context.Context, cl *client.Client) (map[string]interface{}, error) {
	blocks, err := QueryMajorBlocks(ctx, cl, 0, 1)
	if err != nil {
		return nil, fmt.Errorf("failed to query genesis block: %w", err)
	}
	if len(blocks) == 0 {
		return nil, fmt.Errorf("no genesis block found")
	}
	block := blocks[0]

	minorBlocks := 0
	if mb, ok := block["minorBlocks"]; ok {
		if mblist, ok := mb.([]interface{}); ok {
			minorBlocks = len(mblist)
		}
	}
	fmt.Printf("Genesis Block:\n  Index: %v\n  Time: %v\n  MinorBlocks: %d\n",
		block["majorBlockIndex"], block["majorBlockTime"], minorBlocks)

	return block, nil
}

// RetrieveGenesisAuthority fetches the KeyBook and first KeyPage that
// originally signed the genesis block.
func RetrieveGenesisAuthority(ctx context.Context, cl *client.Client) (*protocol.KeyBook, *protocol.KeyPage, error) {
	// Query the partition's account data to discover its authorities
	var partition protocol.AccountAuth
	err := queryAccountAs(ctx, cl, genesisPartitionUrl, &partition)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to query partition account")
	}

	bookUrl, err := getFirstAuthorityUrl(&partition)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get authority URL")
	}

	book, err := GetKeyBook(ctx, cl, bookUrl)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get key book")
	}

	page, err := GetKeyPage(ctx, cl, bookUrl, 0)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get key page")
	}

	return book, page, nil
}
