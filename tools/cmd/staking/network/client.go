package network

import (
	"context"
	"encoding/json"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2/query"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
	"gitlab.com/accumulatenetwork/accumulate/tools/cmd/staking/app"
)

type Network struct {
	client   *client.Client
	paramUrl *url.URL
	params   *app.Parameters
}

var _ app.Accumulate = (*Network)(nil)

func New(server string, parameters *url.URL) (*Network, error) {
	c, err := client.New(server)
	if err != nil {
		return nil, err
	}

	return &Network{client: c, paramUrl: parameters}, nil
}

func (n *Network) Debug() { n.client.DebugRequest = true }

func (n *Network) Run()               {}
func (n *Network) Init()              {}
func (n *Network) TokensIssued(int64) {}

func (n *Network) GetParameters() (*app.Parameters, error) {
	n.params = new(app.Parameters)
	n.params.Init()
	return n.params, nil
}

func (n *Network) xGetParameters() (*app.Parameters, error) {
	// Get the latest data entry and unmarshal it
	req1 := new(api.DataEntryQuery)
	req1.Url = n.paramUrl
	res1, err := n.client.QueryData(context.Background(), req1)
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "query parameters: %w", err)
	}

	b, err := json.Marshal(res1.Data)
	if err != nil {
		return nil, errors.Format(errors.StatusEncodingError, "marshal parameters data entry: %w", err)
	}

	entry, err := protocol.UnmarshalDataEntryJSON(b)
	if err != nil {
		return nil, errors.Format(errors.StatusEncodingError, "unmarshal parameters data entry: %w", err)
	}

	params := new(app.Parameters)
	err = params.UnmarshalBinary(entry.GetData()[0])
	if err != nil {
		return nil, errors.Format(errors.StatusEncodingError, "unmarshal parameters: %w", err)
	}

	n.params = params
	return params, nil
}

func (n *Network) GetTokensIssued() (int64, error) {
	req := new(api.GeneralQuery)
	req.Url = n.params.Account.TokenIssuance
	issuer := new(protocol.TokenIssuer)
	res := new(api.ChainQueryResponse)
	res.Data = issuer
	err := n.client.RequestAPIv2(context.Background(), "query", req, res)
	if err != nil {
		return 0, err
	}

	// TODO: deal with RC3 issued amount, remove for mainnet
	issued := issuer.Issued
	for issued.Cmp(issuer.SupplyLimit) > 0 {
		issued.Sub(&issued, issuer.SupplyLimit)
	}

	return issued.Int64(), nil
}

func (n *Network) GetBlock(index int64) (*app.Block, error) {
	// Get the block metadata
	block, err := n.getMajorBlockMetadata(uint64(index))
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "get anchor hash: %w", err)
	}

	block.Transactions = map[[32]byte][]*protocol.Transaction{}

	// TODO: add all the accounts we care about
	block.Transactions[protocol.AccountUrl("foo").AccountID32()] = nil
	block.Transactions[protocol.AccountUrl("bar").AccountID32()] = nil
	block.Transactions[protocol.AccountUrl("baz").AccountID32()] = nil

	// Describe the network
	desc, err := n.client.Describe(context.Background())
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "describe: %w", err)
	}

	// For each partition
	for _, part := range desc.Values.Network.Partitions {
		// Get the major block
		major, err := n.getMajorBlock(part.ID, uint64(index))
		if err != nil {
			return nil, errors.Format(errors.StatusUnknownError, "query %s major block %d: %w", part.ID, index, err)
		}

		// Get all the corresponding minor blocks
		minor, err := n.getMinorBlocks(part.ID, major)
		if err != nil {
			return nil, errors.Format(errors.StatusUnknownError, "query %s minor blocks for major block %d: %w", part.ID, index, err)
		}

		if part.Type == protocol.PartitionTypeDirectory {
			block.Timestamp = *major.MajorBlockTime
		}

		for _, b := range minor {
			for _, txn := range b.Transactions {
				// Ignore pending and failed transactions
				if txn.Status.Code != errors.StatusDelivered {
					continue
				}

				id := txn.Transaction.Header.Principal.AccountID32()
				txns, ok := block.Transactions[id]
				if ok {
					block.Transactions[id] = append(txns, txn.Transaction)
				}
			}
		}
	}

	return block, nil
}

func (n *Network) getMajorBlockMetadata(blockIndex uint64) (*app.Block, error) {
	major := new(protocol.IndexEntry)
	_, err := n.queryChainEntry(major, protocol.DnUrl().JoinPath(protocol.AnchorPool).WithFragment(fmt.Sprintf("chain/major-block/%d", blockIndex)))
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "query major block %d: %w", blockIndex, err)
	}

	index := new(protocol.IndexEntry)
	_, err = n.queryChainEntry(index, protocol.DnUrl().JoinPath(protocol.Ledger).WithFragment(fmt.Sprintf("chain/root-index/%d", major.RootIndexIndex)))
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "query root index entry %d: %w", major.RootIndexIndex, err)
	}

	entry, err := n.queryChainEntry(nil, protocol.DnUrl().JoinPath(protocol.Ledger).WithFragment(fmt.Sprintf("chain/root/%d", index.Source)))
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "query root entry %d: %w", index.Source, err)
	}

	ms := new(managed.MerkleState)
	ms.Count = int64(entry.Height)
	ms.Pending = entry.State

	block := new(app.Block)
	block.MajorHeight = int64(blockIndex)
	block.BlockHash = *(*[32]byte)(ms.GetMDRoot())
	block.Timestamp = *major.BlockTime
	return block, nil
}

func (n *Network) queryChainEntry(value any, url *url.URL) (*api.ChainEntry, error) {
	entry := new(api.ChainEntry)
	entry.Value = value
	res := new(api.ChainQueryResponse)
	res.Data = entry
	req := new(api.GeneralQuery)
	req.Url = url
	err := n.client.RequestAPIv2(context.Background(), "query", req, res)
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "query major block: %w", err)
	}
	return entry, nil
}

func (n *Network) getMajorBlock(partition string, index uint64) (*api.MajorQueryResponse, error) {
	// Query
	req := new(api.MajorBlocksQuery)
	req.Url = protocol.PartitionUrl(partition)
	req.Start = index
	req.Count = 1
	resp, err := n.client.QueryMajorBlocks(context.Background(), req)
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

func (n *Network) getMinorBlocks(partition string, major *api.MajorQueryResponse) ([]*api.MinorQueryResponse, error) {
	if len(major.MinorBlocks) == 0 {
		return nil, nil
	}

	// Query from the first minor block in the major block to the last
	req := new(api.MinorBlocksQuery)
	req.Url = protocol.PartitionUrl(partition)
	req.Start = major.MinorBlocks[0].BlockIndex
	if req.Start == 1 {
		req.Start++ // Skip Genesis
	}
	req.Count = major.MinorBlocks[len(major.MinorBlocks)-1].BlockIndex - req.Start + 1
	req.BlockFilterMode = query.BlockFilterModeExcludeEmpty
	req.TxFetchMode = query.TxFetchModeExpand
	resp, err := n.client.QueryMinorBlocks(context.Background(), req)
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
