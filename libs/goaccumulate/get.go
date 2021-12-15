package goaccumulate

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"log"

	api2 "github.com/AccumulateNetwork/accumulate/internal/api/v2"
	url2 "github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/types/api/query"
)

func GetByChainId(chainId []byte) (*api2.QueryResponse, error) {
	var res api2.QueryResponse
	res.Data = new(query.ResponseByChainId)

	params := api2.ChainIdQuery{}
	params.ChainId = chainId

	data, err := json.Marshal(&params)
	if err != nil {
		return nil, err
	}

	if err := Client.RequestV2(context.Background(), "query-chain", json.RawMessage(data), &res); err != nil {
		log.Fatal(err)
	}

	//return PrintQueryResponseV2(res)
	return &res, nil
}

func Get(accountUrl string) (string, error) {
	u, err := url2.Parse(accountUrl)
	if err != nil {
		return "", err
	}

	var res api2.QueryResponse

	params := api2.UrlQuery{}
	params.Url = u.String()

	data, err := json.Marshal(&params)
	if err != nil {
		return "", err
	}
	if err := Client.RequestV2(context.Background(), "query", json.RawMessage(data), &res); err != nil {
		return PrintJsonRpcError(err)
	}

	return PrintQueryResponseV2(&res)
}

func GetKey(url, key string) (string, error) {
	var res api2.QueryResponse
	keyb, err := hex.DecodeString(key)
	if err != nil {
		return "", err
	}

	params := api2.KeyPageIndexQuery{}
	params.Url = url
	params.Key = keyb

	data, err := json.Marshal(&params)
	if err != nil {
		return "", err
	}

	err = Client.RequestV2(context.Background(), "query-key-index", json.RawMessage(data), &res)
	if err != nil {
		return PrintJsonRpcError(err)
	}

	return PrintQueryResponseV2(&res)
}
