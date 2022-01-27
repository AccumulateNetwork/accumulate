package api

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/query"
	"github.com/tendermint/tendermint/rpc/client"
)

type queryDirect struct {
	QuerierOptions
	client ABCIQueryClient
}

func (q *queryDirect) query(content queryRequest, opts QueryOptions) (string, []byte, error) {
	var err error
	req := new(query.Query)
	req.Type = content.Type()
	req.Content, err = content.MarshalBinary()
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	b, err := req.MarshalBinary()
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	res, err := q.client.ABCIQueryWithOptions(context.Background(), "/abci_query", b, client.ABCIQueryOptions{Height: int64(opts.Height), Prove: opts.Prove})
	if err != nil {
		return "", nil, fmt.Errorf("failed to send request: %v", err)
	}

	if res.Response.Code == 0 {
		return string(res.Response.Key), res.Response.Value, nil
	}
	if res.Response.Code == protocol.CodeNotFound {
		return "", nil, storage.ErrNotFound
	}

	perr := new(protocol.Error)
	perr.Code = protocol.ErrorCode(res.Response.Code)
	if res.Response.Info != "" {
		perr.Message = errors.New(res.Response.Info)
	} else {
		perr.Message = errors.New(res.Response.Log)
	}
	return "", nil, perr
}

func (q *queryDirect) QueryUrl(s string, opts QueryOptions) (interface{}, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidUrl, err)
	}

	req := new(query.RequestByUrl)
	req.Url = types.String(u.String())
	k, v, err := q.query(req, opts)
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

		var ms *MerkleState
		if res.Height >= 0 || len(res.ChainState) > 0 {
			ms = new(MerkleState)
			ms.Height = uint64(res.Height)
			ms.Roots = res.ChainState
		}

		packed, err := packTxResponse(res.TxId, res.TxSynthTxIds, ms, main, pend, pl)
		if err != nil {
			return nil, err
		}

		packed.Receipts = res.Receipts
		return packed, nil

	case "tx-history":
		txh := new(query.ResponseTxHistory)
		err = txh.UnmarshalBinary(v)
		if err != nil {
			return nil, fmt.Errorf("invalid response: %v", err)
		}

		res := new(MultiResponse)
		res.Items = make([]interface{}, len(txh.Transactions))
		res.Start = uint64(txh.Start)
		res.Count = uint64(txh.End - txh.Start)
		res.Total = uint64(txh.Total)
		for i, tx := range txh.Transactions {
			main, pend, pl, err := unmarshalTxResponse(tx.TxState, tx.TxPendingState)
			if err != nil {
				return nil, err
			}

			res.Items[i], err = packTxResponse(tx.TxId, tx.TxSynthTxIds, nil, main, pend, pl)
			if err != nil {
				return nil, err
			}
		}

		return res, nil

	case "chain-range":
		res := new(query.ResponseChainRange)
		err := res.UnmarshalBinary(v)
		if err != nil {
			return nil, fmt.Errorf("invalid response: %v", err)
		}

		mr := new(MultiResponse)
		mr.Type = "chainEntrySet"
		mr.Start = uint64(res.Start)
		mr.Count = uint64(res.End - res.Start)
		mr.Total = uint64(res.Total)
		mr.Items = make([]interface{}, len(res.Entries))
		for i, entry := range res.Entries {
			qr := new(ChainQueryResponse)
			mr.Items[i] = qr
			qr.Type = "hex"
			qr.Data = hex.EncodeToString(entry)
		}
		return mr, nil

	case "chain-entry":
		res := new(query.ResponseChainEntry)
		err := res.UnmarshalBinary(v)
		if err != nil {
			return nil, fmt.Errorf("invalid response: %v", err)
		}

		qr := new(ChainQueryResponse)
		qr.Type = "chainEntry"
		qr.Data = res
		qr.MainChain = new(MerkleState)
		qr.MainChain.Height = uint64(res.Height)
		qr.MainChain.Roots = res.State
		return qr, nil

	case "data-entry":
		res := new(protocol.ResponseDataEntry)
		err := res.UnmarshalBinary(v)
		if err != nil {
			return nil, fmt.Errorf("invalid response: %v", err)
		}

		qr := new(ChainQueryResponse)
		qr.Type = "dataEntry"
		qr.Data = res
		return qr, nil

	case "data-entry-set":
		res := new(protocol.ResponseDataEntrySet)
		err := res.UnmarshalBinary(v)
		if err != nil {
			return nil, fmt.Errorf("invalid response: %v", err)
		}

		qr := new(ChainQueryResponse)
		qr.Type = "dataEntry"
		qr.Data = res
		return qr, nil

	case "pending":
		res := new(query.ResponsePending)
		err := res.UnmarshalBinary(v)
		if err != nil {
			return nil, fmt.Errorf("invalid response: %v", err)
		}

		qr := new(ChainQueryResponse)
		qr.Type = "pending-transaction"
		qr.Data = res
		return qr, nil

	default:
		return nil, fmt.Errorf("unknown response type %q", k)
	}
}

func (q *queryDirect) QueryDirectory(s string, pagination QueryPagination, opts QueryOptions) (*MultiResponse, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidUrl, err)
	}

	req := new(query.RequestDirectory)
	req.Url = types.String(u.String())
	req.Start = pagination.Start
	req.Limit = pagination.Count
	req.ExpandChains = opts.Expand
	key, val, err := q.query(req, opts)
	if err != nil {
		return nil, err
	}
	if key != "directory" {
		return nil, fmt.Errorf("unknown response type: want directory, got %q", key)
	}

	protoDir := new(protocol.DirectoryQueryResult)
	err = protoDir.UnmarshalBinary(val)
	if err != nil {
		return nil, fmt.Errorf("invalid response: %v", err)
	}

	return responseDirFromProto(protoDir, pagination)
}

func responseDirFromProto(src *protocol.DirectoryQueryResult, pagination QueryPagination) (*MultiResponse, error) {
	dst := new(MultiResponse)
	dst.Type = "directory"
	dst.Start = pagination.Start
	dst.Count = pagination.Count
	dst.Total = src.Total

	dst.Items = make([]interface{}, len(src.Entries))
	for i, entry := range src.Entries {
		dst.Items[i] = entry
	}

	dst.OtherItems = make([]interface{}, len(src.ExpandedEntries))
	for i, entry := range src.ExpandedEntries {
		chain, err := protocol.UnmarshalChain(entry.Entry)
		if err != nil {
			return nil, err
		}
		response, err := packStateResponse(entry, chain)
		if err != nil {
			return nil, err
		}
		dst.OtherItems[i] = response
	}
	return dst, nil
}

func (q *queryDirect) QueryChain(id []byte) (*ChainQueryResponse, error) {
	if len(id) != 32 {
		return nil, fmt.Errorf("invalid chain ID: wanted 32 bytes, got %d", len(id))
	}

	req := new(query.RequestByChainId)
	copy(req.ChainId[:], id)
	k, v, err := q.query(req, QueryOptions{})
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

func (q *queryDirect) QueryTx(id []byte, wait time.Duration, opts QueryOptions) (*TransactionQueryResponse, error) {
	if len(id) != 32 {
		return nil, fmt.Errorf("invalid TX ID: wanted 32 bytes, got %d", len(id))
	}

	var start time.Time
	if wait < time.Second/2 {
		wait = 0
	} else {
		if wait > q.TxMaxWaitTime {
			wait = q.TxMaxWaitTime
		}
		start = time.Now()
	}

	req := new(query.RequestByTxId)
	copy(req.TxId[:], id)

query:
	k, v, err := q.query(req, opts)
	switch {
	case err == nil:
		// Found
	case !errors.Is(err, storage.ErrNotFound):
		// Unknown error
		return nil, err
	case wait == 0 || time.Since(start) > wait:
		// Not found, wait not specified or exceeded
		return nil, err
	default:
		// Not found, try again
		time.Sleep(time.Second / 2)
		goto query
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

	packed, err := packTxResponse(res.TxId, res.TxSynthTxIds, nil, main, pend, pl)
	if err != nil {
		return nil, err
	}

	packed.Receipts = res.Receipts
	return packed, nil
}

func (q *queryDirect) QueryTxHistory(s string, pagination QueryPagination) (*MultiResponse, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidUrl, err)
	}

	if pagination.Count == 0 {
		// TODO Return an empty array plus the total count?
		return nil, validatorError(errors.New("count must be greater than 0"))
	}

	if pagination.Start > math.MaxInt64 {
		return nil, errors.New("start is too large")
	}

	if pagination.Count > math.MaxInt64 {
		return nil, errors.New("count is too large")
	}

	req := new(query.RequestTxHistory)
	req.Start = int64(pagination.Start)
	req.Limit = int64(pagination.Count)
	copy(req.ChainId[:], u.AccountID())
	k, v, err := q.query(req, QueryOptions{})
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

	res := new(MultiResponse)
	res.Type = "txHistory"
	res.Items = make([]interface{}, len(txh.Transactions))
	res.Start = pagination.Start
	res.Count = pagination.Count
	res.Total = uint64(txh.Total)
	for i, tx := range txh.Transactions {
		main, pend, pl, err := unmarshalTxResponse(tx.TxState, tx.TxPendingState)
		if err != nil {
			return nil, err
		}

		res.Items[i], err = packTxResponse(tx.TxId, tx.TxSynthTxIds, nil, main, pend, pl)
		if err != nil {
			return nil, err
		}
	}

	return res, nil
}

func (q *queryDirect) QueryData(url string, entryHash [32]byte) (*ChainQueryResponse, error) {
	r, err := q.QueryUrl(url, QueryOptions{})
	if err != nil {
		return nil, fmt.Errorf("chain state for data not found for %s, %v", url, err)
	}
	qr, ok := r.(*ChainQueryResponse)
	if !ok {
		return nil, fmt.Errorf("%q is not a record", url)
	}

	req := new(query.RequestDataEntry)
	req.Url = url
	req.EntryHash = entryHash
	k, v, err := q.query(req, QueryOptions{})
	if err != nil {
		return nil, err
	}
	if k != "data" {
		return nil, fmt.Errorf("unknown response type: want data, got %q", k)
	}

	//do I need to do anything with v?
	rde := protocol.ResponseDataEntry{}
	err = rde.UnmarshalBinary(v)
	if err != nil {
		return nil, err
	}

	qr.Type = "dataEntry"
	qr.Data = &rde
	return qr, nil
}

func (q *queryDirect) QueryDataSet(url string, pagination QueryPagination, opts QueryOptions) (*MultiResponse, error) {
	if pagination.Count == 0 {
		// TODO Return an empty array plus the total count?
		return nil, validatorError(errors.New("count must be greater than 0"))
	}

	req := new(query.RequestDataEntrySet)
	req.Url = url
	req.Start = pagination.Start
	req.Count = pagination.Count
	req.ExpandChains = opts.Expand

	k, v, err := q.query(req, opts)
	if err != nil {
		return nil, err
	}
	if k != "dataSet" {
		return nil, fmt.Errorf("unknown response type: want dataSet, got %q", k)
	}

	des := new(protocol.ResponseDataEntrySet)
	err = des.UnmarshalBinary(v)
	if err != nil {
		return nil, fmt.Errorf("invalid response: %v", err)
	}
	return responseDataSetFromProto(des, pagination)
}

//responseDataSetFromProto map the response structs to protocol structs, maybe someday they should be the same thing
func responseDataSetFromProto(protoDataSet *protocol.ResponseDataEntrySet, pagination QueryPagination) (*MultiResponse, error) {
	respDataSet := new(MultiResponse)
	respDataSet.Type = "dataSet"
	respDataSet.Start = pagination.Start
	respDataSet.Count = pagination.Count
	respDataSet.Total = protoDataSet.Total
	for _, entry := range protoDataSet.DataEntries {
		de := DataEntryQueryResponse{}
		de.EntryHash = entry.EntryHash
		de.Entry.Data = entry.Entry.Data
		de.Entry.ExtIds = entry.Entry.ExtIds
		respDataSet.Items = append(respDataSet.Items, &de)
	}
	return respDataSet, nil
}

func (q *queryDirect) QueryKeyPageIndex(s string, key []byte) (*ChainQueryResponse, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidUrl, err)
	}

	req := new(query.RequestKeyPageIndex)
	req.Url = u.String()
	req.Key = key
	k, v, err := q.query(req, QueryOptions{})
	if err != nil {
		return nil, err
	}
	if k != "key-page-index" {
		return nil, fmt.Errorf("unknown response type: want key-page-index, got %q", k)
	}

	qr := new(query.ResponseKeyPageIndex)
	err = qr.UnmarshalBinary(v)
	if err != nil {
		return nil, err
	}

	res := new(ChainQueryResponse)
	res.Data = qr
	res.Type = "key-page-index"
	return res, nil
}
