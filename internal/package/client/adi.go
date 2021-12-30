package client

import (
	"context"
	"encoding/json"

	api2 "github.com/AccumulateNetwork/accumulate/internal/api/v2"
	url2 "github.com/AccumulateNetwork/accumulate/internal/url"
)

func GetAdiDirectory(actor string) (*api2.QueryResponse, error) {

	u, err := url2.Parse(actor)
	if err != nil {
		return nil, err
	}

	var res api2.QueryResponse
	params := api2.UrlQuery{}
	params.Url = u.String()

	data, err := json.Marshal(&params)
	if err != nil {
		return nil, err
	}

	if err := Client.RequestV2(context.Background(), "query-directory", json.RawMessage(data), &res); err != nil {
		return nil, err
	}

	return &res, nil
}

func GetADI(url string) (*api2.QueryResponse, error) {

	var res api2.QueryResponse

	u, err := url2.Parse(url)
	params := api2.UrlQuery{}
	params.Url = u.String()

	data, err := json.Marshal(&params)
	if err != nil {
		return nil, err
	}

	if err := Client.RequestV2(context.Background(), "query", json.RawMessage(data), &res); err != nil {
		return nil, err
	}

	return &res, nil
}
