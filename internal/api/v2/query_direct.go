package api

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/query"
)

type queryDirect struct {
	QuerierOptions
	client ABCIQueryClient
}

func (q *queryDirect) query(content queryRequest) (string, []byte, error) {
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

	res, err := q.client.ABCIQuery(context.Background(), "/abci_query", b)
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

func (q *queryDirect) QueryUrl(s string) (*QueryResponse, error) {
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

func (q *queryDirect) QueryDirectory(s string, pagination QueryPagination, opts QueryOptions) (*QueryResponse, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidUrl, err)
	}

	req := new(query.RequestDirectory)
	req.Url = types.String(u.String())
	req.Start = pagination.Start
	req.Limit = pagination.Count
	req.ExpandChains = opts.ExpandChains
	key, val, err := q.query(req)
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
	respDir, err := responseDirFromProto(protoDir, pagination)
	if err != nil {
		return nil, err
	}

	res := new(QueryResponse)
	res.Type = "directory"
	res.Data = respDir
	return res, nil
}

func responseDirFromProto(protoDir *protocol.DirectoryQueryResult, pagination QueryPagination) (*DirectoryQueryResult, error) {
	respDir := new(DirectoryQueryResult)
	respDir.Entries = protoDir.Entries
	respDir.Start = pagination.Start
	respDir.Count = pagination.Count
	respDir.Total = protoDir.Total
	respDir.ExpandedEntries = make([]*QueryResponse, len(protoDir.ExpandedEntries))
	for i, entry := range protoDir.ExpandedEntries {
		chain, err := protocol.UnmarshalChain(entry.Entry)
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

func (q *queryDirect) QueryChain(id []byte) (*QueryResponse, error) {
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

func (q *queryDirect) QueryTx(id []byte, wait time.Duration) (*QueryResponse, error) {
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
	k, v, err := q.query(req)
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

	return packTxResponse(res.TxId, res.TxSynthTxIds, main, pend, pl)
}

func (q *queryDirect) QueryTxHistory(s string, start, count uint64) (*MultiResponse, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidUrl, err)
	}

	if count == 0 {
		// TODO Return an empty array plus the total count?
		return nil, validatorError(errors.New("count must be greater than 0"))
	}

	if start > math.MaxInt64 {
		return nil, errors.New("start is too large")
	}

	if count > math.MaxInt64 {
		return nil, errors.New("count is too large")
	}

	req := new(query.RequestTxHistory)
	req.Start = int64(start)
	req.Limit = int64(count)
	copy(req.ChainId[:], u.ResourceChain())
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

	res := new(MultiResponse)
	res.Items = make([]interface{}, len(txh.Transactions))
	res.Start = start
	res.Count = count
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

func (q *queryDirect) QueryData(url string, entryHash [32]byte) (*QueryResponse, error) {

	qr, err := q.QueryUrl(url)
	if err != nil {
		return nil, fmt.Errorf("chain state for data not found for %s, %v", url, err)
	}

	req := new(query.RequestDataEntry)
	req.Url = url
	req.EntryHash = entryHash
	k, v, err := q.query(req)
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

func (q *queryDirect) QueryDataSet(url string, pagination QueryPagination, opts QueryOptions) (*QueryResponse, error) {
	qr, err := q.QueryUrl(url)
	if err != nil {
		return nil, fmt.Errorf("chain state for data not found for %s, %v", url, err)
	}

	if pagination.Count == 0 {
		// TODO Return an empty array plus the total count?
		return nil, validatorError(errors.New("count must be greater than 0"))
	}

	req := new(query.RequestDataEntrySet)
	req.Url = url
	req.Start = pagination.Start
	req.Count = pagination.Count
	req.ExpandChains = opts.ExpandChains

	k, v, err := q.query(req)
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
	desResponse, err := responseDataSetFromProto(des, pagination)
	if err != nil {
		return nil, err
	}

	qr.Type = "dataSet"
	qr.Data = desResponse
	return qr, nil
}

//responseDataSetFromProto map the response structs to protocol structs, maybe someday they should be the same thing
func responseDataSetFromProto(protoDataSet *protocol.ResponseDataEntrySet, pagination QueryPagination) (*DataEntrySetQueryResponse, error) {
	respDataSet := new(DataEntrySetQueryResponse)
	respDataSet.Start = pagination.Start
	respDataSet.Count = pagination.Count
	respDataSet.Total = protoDataSet.Total
	for _, entry := range protoDataSet.DataEntries {
		de := DataEntryQueryResponse{}
		de.EntryHash = entry.EntryHash
		de.Entry.Data = entry.Entry.Data
		for _, eh := range entry.Entry.ExtIds {
			de.Entry.ExtIds = append(de.Entry.ExtIds, eh)
		}
		respDataSet.DataEntries = append(respDataSet.DataEntries, de)
	}
	return respDataSet, nil
}

func (q *queryDirect) QueryKeyPageIndex(s string, key []byte) (*QueryResponse, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidUrl, err)
	}

	req := new(query.RequestKeyPageIndex)
	req.Url = u.String()
	req.Key = key
	k, v, err := q.query(req)
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

	res := new(QueryResponse)
	res.Data = qr
	res.Type = "key-page-index"
	return res, nil
}
