package api

import (
	"context"
	"errors"
	"fmt"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/query"
	tm "github.com/tendermint/tendermint/abci/types"
)

type queryDirect struct {
	client ABCIQueryClient
}

func (queryDirect) responseIsError(r tm.ResponseQuery) error {
	if r.Code == 0 {
		return nil
	}

	switch {
	case r.Code == protocol.CodeNotFound:
		return storage.ErrNotFound
	case r.Log != "":
		return errors.New(r.Log)
	case r.Info != "":
		return errors.New(r.Info)
	default:
		return fmt.Errorf("query failed with code %d", r.Code)
	}
}

func (q queryDirect) query(content queryRequest) (string, []byte, error) {
	return q.queryType(content.Type(), content)
}

func (q queryDirect) queryType(typ types.QueryType, content queryRequest) (string, []byte, error) {
	var err error
	req := new(query.Query)
	req.Type = typ
	req.Content, err = content.MarshalBinary()
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	b, err := req.MarshalBinary()
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	res, err := q.client.ABCIQuery(context.Background(), "/abci_query", b)
	if err != nil {
		return "", nil, fmt.Errorf("failed to send request: %v", err)
	}

	switch {
	case res.Response.Code == protocol.CodeNotFound:
		return "", nil, storage.ErrNotFound
	case res.Response.Log != "":
		return "", nil, errors.New(res.Response.Log)
	case res.Response.Info != "":
		return "", nil, errors.New(res.Response.Info)
	case res.Response.Code != 0:
		return "", nil, fmt.Errorf("query failed with code %d", res.Response.Code)
	}

	return string(res.Response.Key), res.Response.Value, nil
}

func (q queryDirect) QueryUrl(s string) (*QueryResponse, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidUrl, err)
	}

	req := new(query.RequestByUrl)
	req.Url = types.String(u.String())
	k, v, err := q.query(req)
	if err != nil {
		return nil, err
	}

	switch k {
	case "chain":
		obj, chain, err := unmarshalState(v)
		if err != nil {
			return nil, err
		}

		return packStateResponse(obj, chain)

	case "tx":
		res := new(query.ResponseByTxId)
		err := res.UnmarshalBinary(v)
		if err != nil {
			return nil, fmt.Errorf("invalid TX response: %v", err)
		}

		main, pend, pl, err := unmarshalTxResponse(res.TxState, res.TxPendingState)
		if err != nil {
			return nil, err
		}

		return packTxResponse(res.TxId, res.TxSynthTxIds, main, pend, pl)

	default:
		return nil, fmt.Errorf("unknown response type: want chain or tx, got %q", k)
	}
}

func (q queryDirect) QueryDirectory(s string, queryOptions *QueryOptions) (*QueryResponse, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidUrl, err)
	}

	req := new(query.RequestDirectory)
	req.Url = types.String(u.String())
	req.ExpandChains = types.Bool(queryOptions.ExpandChains)
	k, v, err := q.queryType(types.QueryTypeDirectoryUrl, req)
	if err != nil {
		return nil, err
	}
	if k != "directory" {
		return nil, fmt.Errorf("unknown response type: want directory, got %q", k)
	}

	protoDir := new(protocol.DirectoryQueryResult)
	err = protoDir.UnmarshalBinary(v)
	if err != nil {
		return nil, fmt.Errorf("invalid response: %v", err)
	}
	respDir, err := responseDirFromProto(protoDir)
	if err != nil {
		return nil, err
	}

	res := new(QueryResponse)
	res.Type = "directory"
	res.Data = respDir
	return res, nil
}

func responseDirFromProto(protoDir *protocol.DirectoryQueryResult) (*DirectoryQueryResult, error) {
	respDir := new(DirectoryQueryResult)
	respDir.Entries = protoDir.Entries
	respDir.ExpandedEntries = make([]*QueryResponse, len(protoDir.ExpandedEntries))
	for i, entry := range protoDir.ExpandedEntries {
		chain, err := chainFromStateObj(entry)
		if err != nil {
			return nil, err
		}
		response, err := packStateResponse(entry, chain)
		if err != nil {
			return nil, err
		}
		respDir.ExpandedEntries[i] = response
	}
	return respDir, nil
}

func (q queryDirect) QueryChain(id []byte) (*QueryResponse, error) {
	if len(id) != 32 {
		return nil, fmt.Errorf("invalid chain ID: wanted 32 bytes, got %d", len(id))
	}

	req := new(query.RequestByChainId)
	copy(req.ChainId[:], id)
	k, v, err := q.query(req)
	if err != nil {
		return nil, err
	}
	if k != "chain" {
		return nil, fmt.Errorf("unknown response type: want chain, got %q", k)
	}

	obj, chain, err := unmarshalState(v)
	if err != nil {
		return nil, err
	}

	return packStateResponse(obj, chain)
}

func (q queryDirect) QueryTx(id []byte) (*QueryResponse, error) {
	if len(id) != 32 {
		return nil, fmt.Errorf("invalid TX ID: wanted 32 bytes, got %d", len(id))
	}

	req := new(query.RequestByTxId)
	copy(req.TxId[:], id)
	k, v, err := q.query(req)
	if err != nil {
		return nil, err
	}
	if k != "tx" {
		return nil, fmt.Errorf("unknown response type: want tx, got %q", k)
	}

	res := new(query.ResponseByTxId)
	err = res.UnmarshalBinary(v)
	if err != nil {
		return nil, fmt.Errorf("invalid TX response: %v", err)
	}

	main, pend, pl, err := unmarshalTxResponse(res.TxState, res.TxPendingState)
	if err != nil {
		return nil, err
	}

	return packTxResponse(res.TxId, res.TxSynthTxIds, main, pend, pl)
}

func (q queryDirect) QueryTxHistory(s string, start, count int64) (*QueryMultiResponse, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidUrl, err)
	}

	req := new(query.RequestTxHistory)
	copy(req.ChainId[:], u.ResourceChain())
	req.Start = start
	req.Limit = count
	k, v, err := q.query(req)
	if err != nil {
		return nil, err
	}
	if k != "tx-history" {
		return nil, fmt.Errorf("unknown response type: want tx-history, got %q", k)
	}

	txh := new(query.ResponseTxHistory)
	err = txh.UnmarshalBinary(v)
	if err != nil {
		return nil, fmt.Errorf("invalid response: %v", err)
	}

	res := new(QueryMultiResponse)
	res.Items = make([]*QueryResponse, len(txh.Transactions))
	res.Start = uint64(start)
	res.Count = uint64(count)
	res.Total = uint64(txh.Total)
	for i, tx := range txh.Transactions {
		main, pend, pl, err := unmarshalTxResponse(tx.TxState, tx.TxPendingState)
		if err != nil {
			return nil, err
		}

		res.Items[i], err = packTxResponse(tx.TxId, tx.TxSynthTxIds, main, pend, pl)
		if err != nil {
			return nil, err
		}
	}

	return res, nil
}
