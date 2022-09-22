package main

import (
	"context"
	"encoding/json"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2/query"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func getMajorBlockByIndex(client *client.Client, ctx context.Context, partition string, index uint64) (*api.MajorQueryResponse, error) {
	// Query
	req := new(api.MajorBlocksQuery)
	req.Url = protocol.PartitionUrl(partition)
	req.Start = index
	req.Count = 1
	resp, err := client.QueryMajorBlocks(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(resp.Items) == 0 {
		return nil, errors.NotFound("major block %d of %s not found", index, partition)
	}

	// Remarshal map to struct
	b, err := json.Marshal(resp.Items[0])
	if err != nil {
		return nil, err
	}
	block := new(api.MajorQueryResponse)
	err = json.Unmarshal(b, block)
	if err != nil {
		return nil, err
	}
	return block, nil
}

func getMinorBlockByMajorIndex(client *client.Client, ctx context.Context, partition string, majorIndex uint64) ([]*api.MinorQueryResponse, error) {
	// Query the major block
	major, err := getMajorBlockByIndex(client, ctx, partition, majorIndex)
	if err != nil {
		return nil, err
	}
	if len(major.MinorBlocks) == 0 {
		return nil, nil
	}

	// Query from the first minor block in the major block to the last
	req := new(api.MinorBlocksQuery)
	req.Url = protocol.PartitionUrl(partition)
	req.Start = major.MinorBlocks[0].BlockIndex
	req.Count = major.MinorBlocks[len(major.MinorBlocks)-1].BlockIndex - req.Start + 1
	req.BlockFilterMode = query.BlockFilterModeExcludeEmpty
	req.TxFetchMode = query.TxFetchModeExpand
	resp, err := client.QueryMinorBlocks(ctx, req)
	if err != nil {
		return nil, err
	}

	// Remarshal []map to []struct
	var blocks []*api.MinorQueryResponse
	b, err := json.Marshal(resp.Items)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(b, &blocks)
	if err != nil {
		return nil, err
	}
	return blocks, nil
}

func getTransactionsByMajorIndex(client *client.Client, ctx context.Context, partition string, majorIndex uint64, accounts []*url.URL) ([]*protocol.Transaction, error) {
	// Query the major block
	blocks, err := getMinorBlockByMajorIndex(client, ctx, partition, majorIndex)
	if err != nil {
		return nil, err
	}

	filter := map[[32]byte]bool{}
	for _, a := range accounts {
		filter[a.AccountID32()] = true
	}

	// Extract transactions
	seen := map[[32]byte]bool{}
	var txns []*protocol.Transaction
	for _, block := range blocks {
		for _, txn := range block.Transactions {
			// Ignore pending and failed transactions
			if txn.Status.Code != errors.StatusDelivered {
				continue
			}

			if !filter[txn.Transaction.Header.Principal.AccountID32()] {
				continue
			}

			// Only add each transaction once
			if seen[txn.Transaction.ID().Hash()] {
				continue
			}

			seen[txn.Transaction.ID().Hash()] = true
			txns = append(txns, txn.Transaction)
		}
	}

	return txns, nil
}

func accountsDidChangeInMajorIndex(client *client.Client, ctx context.Context, partition string, majorIndex uint64, accounts []*url.URL) ([]bool, error) {
	// Query the major block
	blocks, err := getMinorBlockByMajorIndex(client, ctx, partition, majorIndex)
	if err != nil {
		return nil, err
	}

	didChange := make([]bool, len(accounts))
	lookup := map[[32]byte]*bool{}
	for i, a := range accounts {
		lookup[a.AccountID32()] = &didChange[i]
	}

	// For block, for each transaction
	for _, block := range blocks {
		for _, txn := range block.Transactions {
			// Ignore pending and failed transactions
			if txn.Status.Code != errors.StatusDelivered {
				continue
			}

			// Mark the account as changed if it's one we care about
			v := lookup[txn.Transaction.Header.Principal.AccountID32()]
			if v != nil {
				*v = true
			}
		}
	}

	return didChange, nil
}
