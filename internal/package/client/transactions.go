package client

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	api2 "github.com/AccumulateNetwork/accumulate/internal/api/v2"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"golang.org/x/sync/errgroup"
)

var (
	TxWait      time.Duration
	TxWaitSynth time.Duration
)

func getTX(hash []byte, wait time.Duration) (*api2.QueryResponse, error) {
	var res api2.QueryResponse
	var err error

	params := new(api2.TxnQuery)
	params.Txid = hash

	if wait > 0 {
		params.Wait = wait
	}

	data, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	err = Client.RequestV2(context.Background(), "query-tx", json.RawMessage(data), &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func GetTX(hash string) (*api2.QueryResponse, error) {
	txid, err := hex.DecodeString(hash)
	if err != nil {
		return nil, err
	}

	t := Client.Timeout
	defer func() { Client.Timeout = t }()

	if TxWait > 0 {
		Client.Timeout = TxWait * 2
	}

	res, err := getTX(txid, TxWait)
	if err != nil {
		var rpcErr jsonrpc2.Error
		if errors.As(err, &rpcErr) {
			return nil, err
		}
		return nil, err
	}

	out, err := res, nil
	if err != nil {
		return nil, err
	}

	if TxWaitSynth == 0 || len(res.SyntheticTxids) == 0 {
		return out, nil
	}

	if TxWaitSynth > 0 {
		Client.Timeout = TxWaitSynth * 2
	}

	errg := new(errgroup.Group)
	err = errg.Wait()

	if err != nil {
		var rpcErr jsonrpc2.Error
		if errors.As(err, &rpcErr) {
			return nil, err
		}
		return nil, err
	}

	return out, nil
}

func GetTXHistory(accountUrl string, s string, e string) (*api2.QueryResponse, error) {

	start, err := strconv.Atoi(s)
	if err != nil {
		return nil, err
	}
	end, err := strconv.Atoi(e)
	if err != nil {
		return nil, err
	}

	u, err := url.Parse(accountUrl)
	if err != nil {
		return nil, err
	}

	var res api2.QueryResponse

	params := new(api2.TxHistoryQuery)
	params.UrlQuery.Url = u.String()
	params.QueryPagination.Start = uint64(start)
	params.QueryPagination.Count = uint64(end)
	data, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	if err := Client.RequestV2(context.Background(), "query-tx-history", json.RawMessage(data), &res); err != nil {
		return nil, err
	}

	return &res, nil
}

func CreateTX(sender string, args []string) (*api2.TxResponse, error) {
	//sender string, receiver string, amount string
	res := new(api2.TxResponse)
	var err error
	u, err := url.Parse(sender)
	if err != nil {
		return nil, err
	}

	args, si, pk, err := prepareSigner(u, args)

	if len(args) < 2 {
		return nil, fmt.Errorf("invalid number of arguments for tx create")
	}

	u2, err := url.Parse(args[0])
	if err != nil {
		return nil, fmt.Errorf("invalid receiver url %s, %v", args[0], err)
	}
	amount := args[1]

	tokentx := new(api2.TokenSend)
	tokentx.From = (u.String())

	to := []api2.TokenDeposit{}
	r := api2.TokenDeposit{}

	amt, err := strconv.ParseFloat(amount, 64)

	r.Amount = uint64(amt * 1e8)
	r.Url = (u2.String())

	to = append(to, r)

	tokentx.To = to

	data, err := json.Marshal(tokentx)
	if err != nil {
		return nil, err
	}

	dataBinary, err := json.RawMessage(tokentx.From).MarshalJSON()
	if err != nil {
		return nil, err
	}

	nonce := nonceFromTimeNow()
	params, err := prepareGenTxV2(data, dataBinary, u, si, pk, nonce)
	if err != nil {
		return nil, err
	}

	if err := Client.RequestV2(context.Background(), "send-tokens", params, &res); err != nil {
		return nil, err
	}

	return res, nil
}
