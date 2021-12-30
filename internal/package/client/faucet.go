package client

import (
	"context"
	"encoding/json"

	api2 "github.com/AccumulateNetwork/accumulate/internal/api/v2"
	url2 "github.com/AccumulateNetwork/accumulate/internal/url"
)

func Faucet(url string) (*api2.TxResponse, error) {
	res := new(api2.TxResponse)
	params := api2.UrlQuery{}

	u, err := url2.Parse(url)
	if err != nil {
		return nil, err
	}

	params.Url = u.String()

	data, err := json.Marshal(&params)
	if err != nil {
		return nil, err
	}
	if err := Client.RequestV2(context.Background(), "faucet", json.RawMessage(data), &res); err != nil {
		return nil, err
	}

	return res, nil

}
