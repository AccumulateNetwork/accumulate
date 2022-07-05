package api

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/tendermint/tendermint/rpc/client"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
)

const QueryBlocksMaxCount = 1000 // Hardcoded ceiling for now

type queryDirect struct {
	Options
	Partition string
}

func (q *queryDirect) query(req query.Request, opts QueryOptions) (string, []byte, error) {
	var err error
	b, err := req.MarshalBinary()
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	res, err := q.Router.Query(context.Background(), q.Partition, b, client.ABCIQueryOptions{Height: int64(opts.Height), Prove: opts.Prove})
	if err != nil {
		return "", nil, fmt.Errorf("failed to send request: %v", err)
	}

	if res.Response.Code == 0 {
		return string(res.Response.Key), res.Response.Value, nil
	}
	if res.Response.Code != uint32(protocol.ErrorCodeFailed) {
		return "", nil, errors.New(errors.StatusUnknownError, res.Response.Info)
	}

	var err2 *errors.Error
	if json.Unmarshal([]byte(res.Response.Info), &err2) == nil {
		return "", nil, err2
	}
	return "", nil, errors.New(errors.StatusUnknownError, res.Response.Info)
}

func (q *queryDirect) QueryUrl(u *url.URL, opts QueryOptions) (interface{}, error) {
	req := new(query.RequestByUrl)
	req.Url = u
	req.Scratch = opts.Scratch
	k, v, err := q.query(req, opts)
	if err != nil {
		return nil, err
	}

	switch k {
	case "account":
		resp := new(query.ResponseAccount)
		err = resp.UnmarshalBinary(v)
		if err != nil {
			return nil, err
		}

		return packStateResponse(resp.Account, resp.ChainState, resp.Receipt)

	case "tx":
		res := new(query.ResponseByTxId)
		err := res.UnmarshalBinary(v)
		if err != nil {
			return nil, fmt.Errorf("invalid TX response: %v", err)
		}

		var ms *MerkleState
		if res.Height >= 0 || len(res.ChainState) > 0 {
			ms = new(MerkleState)
			ms.Height = uint64(res.Height)
			ms.Roots = res.ChainState
		}

		return packTxResponse(res, ms, res.Envelope, res.Status)

	case "tx-history":
		txh := new(query.ResponseTxHistory)
		err = txh.UnmarshalBinary(v)
		if err != nil {
			return nil, fmt.Errorf("invalid response: %v", err)
		}

		res := new(MultiResponse)
		res.Type = "txHistory"
		res.Items = make([]interface{}, len(txh.Transactions))
		res.Start = uint64(txh.Start)
		res.Count = uint64(txh.End - txh.Start)
		res.Total = uint64(txh.Total)
		for i, tx := range txh.Transactions {
			queryRes, err := packTxResponse(&tx, nil, tx.Envelope, tx.Status) //nolint:rangevarref
			if err != nil {
				return nil, err
			}

			res.Items[i] = queryRes
		}

		return res, nil

	case "chain-range":
		res := new(query.ResponseChainRange)
		err := res.UnmarshalBinary(v)
		if err != nil {
			return nil, fmt.Errorf("invalid response: %v", err)
		}

		return packChainValues(res), nil

	case "chain-entry":
		res := new(query.ResponseChainEntry)
		err := res.UnmarshalBinary(v)
		if err != nil {
			return nil, fmt.Errorf("invalid response: %v", err)
		}

		return packChainValue(res), nil

	case "data-entry":
		res := new(query.ResponseDataEntry)
		err := res.UnmarshalBinary(v)
		if err != nil {
			return nil, fmt.Errorf("invalid response: %v", err)
		}

		qr := new(ChainQueryResponse)
		qr.Type = "dataEntry"
		qr.Data = res
		return qr, nil

	case "data-entry-set":
		res := new(query.ResponseDataEntrySet)
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

		qr := new(MultiResponse)
		qr.Type = "pending"
		qr.Items = make([]interface{}, len(res.Transactions))
		for i, txid := range res.Transactions {
			txid := txid.Hash()
			qr.Items[i] = hex.EncodeToString(txid[:])
		}
		return qr, nil

	default:
		return nil, fmt.Errorf("unknown response type %q", k)
	}
}

func (q *queryDirect) QueryDirectory(u *url.URL, pagination QueryPagination, opts QueryOptions) (*MultiResponse, error) {
	req := new(query.RequestDirectory)
	req.Url = u
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

	protoDir := new(query.DirectoryQueryResult)
	err = protoDir.UnmarshalBinary(val)
	if err != nil {
		return nil, fmt.Errorf("invalid response: %v", err)
	}

	return responseDirFromProto(protoDir, pagination)
}

func responseDirFromProto(src *query.DirectoryQueryResult, pagination QueryPagination) (*MultiResponse, error) {
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
		response, err := packStateResponse(entry, nil, nil)
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
	if k != "account" {
		return nil, fmt.Errorf("unknown response type: want chain, got %q", k)
	}

	resp := new(query.ResponseAccount)
	err = resp.UnmarshalBinary(v)
	if err != nil {
		return nil, err
	}

	return packStateResponse(resp.Account, resp.ChainState, nil)
}

func (q *queryDirect) QueryTx(id []byte, wait time.Duration, ignorePending bool, opts QueryOptions) (*TransactionQueryResponse, error) {
	if len(id) != 32 {
		return nil, fmt.Errorf("invalid TX ID: wanted 32 bytes, got %d", len(id))
	}

	var start time.Time
	var sleepIncr time.Duration
	var sleep time.Duration
	if wait < time.Second/2 {
		wait = 0
	} else {
		if wait > q.TxMaxWaitTime {
			wait = q.TxMaxWaitTime
		}
		sleepIncr = wait / 50
		sleep = sleepIncr
		start = time.Now()
	}

	req := new(query.RequestByTxId)
	copy(req.TxId[:], id)

query:
	// Execute the query
	res := new(query.ResponseByTxId)
	k, v, err := q.query(req, opts)
	switch {
	case err == nil:
		// Check the code
		if k != "tx" {
			return nil, fmt.Errorf("unknown response type: want tx, got %q", k)
		}

		// Unmarshal the response
		err = res.UnmarshalBinary(v)
		if err != nil {
			return nil, fmt.Errorf("invalid TX response: %v", err)
		}

	case !errors.Is(err, storage.ErrNotFound):
		// Unknown error
		return nil, err
	}

	// Did we find it?
	switch {
	case err == nil && (!res.Status.Pending() || !ignorePending):
		// Found
		return packTxResponse(res, nil, res.Envelope, res.Status)

	case wait == 0 || time.Since(start) > wait:
		// Not found or pending, wait not specified or exceeded
		if err == nil {
			err = errors.NotFound("transaction %X still pending", id)
		} else {
			err = errors.Wrap(errors.StatusUnknownError, err)
		}
		return nil, err

	default:
		// Not found or pending, try again, linearly increasing the wait time
		time.Sleep(sleep)
		sleep += sleepIncr
		goto query
	}
}

func (q *queryDirect) QueryTxHistory(u *url.URL, pagination QueryPagination, scratch bool) (*MultiResponse, error) {
	if pagination.Count == 0 {
		// TODO Return an empty array plus the total count?
		return nil, validatorError(errors.New(errors.StatusBadRequest, "count must be greater than 0"))
	}

	if pagination.Start > math.MaxInt64 {
		return nil, errors.New(errors.StatusBadRequest, "start is too large")
	}

	if pagination.Count > math.MaxInt64 {
		return nil, errors.New(errors.StatusBadRequest, "count is too large")
	}

	req := new(query.RequestTxHistory)
	req.Account = u
	req.Scratch = scratch
	req.Start = pagination.Start
	req.Limit = pagination.Count
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
		queryRes, err := packTxResponse(&tx, nil, tx.Envelope, tx.Status) //nolint:rangevarref
		if err != nil {
			return nil, err
		}
		res.Items[i] = queryRes
	}

	return res, nil
}

func (q *queryDirect) QueryData(url *url.URL, entryHash [32]byte) (*ChainQueryResponse, error) {
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
	rde := query.ResponseDataEntry{}
	err = rde.UnmarshalBinary(v)
	if err != nil {
		return nil, err
	}

	qr.Type = "dataEntry"
	qr.Data = &rde
	return qr, nil
}

func (q *queryDirect) QueryDataSet(url *url.URL, pagination QueryPagination, opts QueryOptions) (*MultiResponse, error) {
	if pagination.Count == 0 {
		// TODO Return an empty array plus the total count?
		return nil, validatorError(errors.New(errors.StatusBadRequest, "count must be greater than 0"))
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

	des := new(query.ResponseDataEntrySet)
	err = des.UnmarshalBinary(v)
	if err != nil {
		return nil, fmt.Errorf("invalid response: %v", err)
	}
	return responseDataSetFromProto(des, pagination)
}

//responseDataSetFromProto map the response structs to protocol structs, maybe someday they should be the same thing
func responseDataSetFromProto(protoDataSet *query.ResponseDataEntrySet, pagination QueryPagination) (*MultiResponse, error) {
	respDataSet := new(MultiResponse)
	respDataSet.Type = "dataSet"
	respDataSet.Start = pagination.Start
	respDataSet.Count = pagination.Count
	respDataSet.Total = protoDataSet.Total
	for _, entry := range protoDataSet.DataEntries {
		de := DataEntryQueryResponse{}
		de.EntryHash = entry.EntryHash
		de.Entry = entry.Entry
		respDataSet.Items = append(respDataSet.Items, &de)
	}
	return respDataSet, nil
}

func (q *queryDirect) QueryKeyPageIndex(u *url.URL, key []byte) (*ChainQueryResponse, error) {
	req := new(query.RequestKeyPageIndex)
	req.Url = u
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

func (q *queryDirect) QueryMinorBlocks(u *url.URL, pagination QueryPagination, txFetchMode query.TxFetchMode, blockFilterMode query.BlockFilterMode) (*MultiResponse, error) {
	if pagination.Start > math.MaxInt64 {
		return nil, errors.New(errors.StatusBadRequest, "start is too large")
	}

	if pagination.Count > QueryBlocksMaxCount {
		return nil, fmt.Errorf("count is too large, the ceiling is fixed to %d", QueryBlocksMaxCount)
	}

	req := &query.RequestMinorBlocks{
		Account:         u,
		Start:           pagination.Start,
		Limit:           pagination.Count,
		TxFetchMode:     txFetchMode,
		BlockFilterMode: blockFilterMode,
	}
	k, v, err := q.query(req, QueryOptions{})
	if err != nil {
		return nil, err
	}
	if k != "minor-block" {
		return nil, fmt.Errorf("unknown response type: want minor-block, got %q", k)
	}

	res := new(query.ResponseMinorBlocks)
	err = res.UnmarshalBinary(v)
	if err != nil {
		return nil, fmt.Errorf("invalid response: %v", err)
	}

	mres := new(MultiResponse)
	mres.Type = "minorBlock"
	mres.Items = make([]interface{}, 0)
	mres.Start = pagination.Start
	mres.Count = pagination.Count
	mres.Total = res.TotalBlocks
	for _, entry := range res.Entries {
		queryRes, err := packMinorQueryResponse(entry)
		if err != nil {
			return nil, err
		}
		mres.Items = append(mres.Items, queryRes)
	}

	return mres, nil
}

func (q *queryDirect) QueryMajorBlocks(u *url.URL, pagination QueryPagination) (*MultiResponse, error) {
	if pagination.Start > math.MaxInt64 {
		return nil, errors.New(errors.StatusBadRequest, "start is too large")
	}

	if pagination.Count > QueryBlocksMaxCount {
		return nil, fmt.Errorf("count is too large, the ceiling is fixed to %d", QueryBlocksMaxCount)
	}

	req := &query.RequestMajorBlocks{
		Account: u,
		Start:   pagination.Start,
		Limit:   pagination.Count,
	}
	k, v, err := q.query(req, QueryOptions{})
	if err != nil {
		return nil, err
	}
	if k != "major-block" {
		return nil, fmt.Errorf("unknown response type: want major-block, got %q", k)
	}

	res := new(query.ResponseMajorBlocks)
	err = res.UnmarshalBinary(v)
	if err != nil {
		return nil, fmt.Errorf("invalid response: %v", err)
	}

	mres := new(MultiResponse)
	mres.Type = "majorBlock"
	mres.Items = make([]interface{}, 0)
	mres.Start = pagination.Start
	mres.Count = pagination.Count
	mres.Total = res.TotalBlocks
	for _, entry := range res.Entries {
		queryRes, err := packMajorQueryResponse(entry)
		if err != nil {
			return nil, err
		}
		mres.Items = append(mres.Items, queryRes)
	}

	return mres, nil
}

func (q *queryDirect) QuerySynth(source, destination *url.URL, number uint64, anchor bool) (*TransactionQueryResponse, error) {
	req := new(query.RequestSynth)
	req.Source = source
	req.Destination = destination
	req.SequenceNumber = number
	req.Anchor = anchor
	_, v, err := q.query(req, QueryOptions{})
	if err != nil {
		return nil, err
	}

	res := new(query.ResponseByTxId)
	err = res.UnmarshalBinary(v)
	if err != nil {
		return nil, fmt.Errorf("invalid TX response: %v", err)
	}

	return packTxResponse(res, nil, res.Envelope, res.Status)
}
