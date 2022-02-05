package cmd

import (
	"context"
	"fmt"
	"reflect"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	url2 "gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
)

func queryUrl(url string, expand, prove bool, resp interface{}) (interface{}, error) {
	req := new(api.GeneralQuery)
	req.Url = url
	req.Expand = expand
	req.Prove = prove
	return Client.Query(context.Background(), req, resp)
}

func queryAccount(url string, expect protocol.Account) (protocol.Account, *api.ChainQueryResponse, error) {
	res, err := queryUrl(url, false, false, nil)
	if err != nil {
		return nil, nil, err
	}

	acntRes, ok := res.(*api.ChainQueryResponse)
	if !ok {
		return nil, nil, fmt.Errorf("expected account response, got %T", res)
	}

	account, ok := acntRes.Data.(protocol.Account)
	if !ok {
		return nil, nil, fmt.Errorf("expected account, got %T", acntRes.Type)
	}

	if expect == nil {
		return account, acntRes, nil
	}

	// TODO Use AccountType once AccountHeader is refactored to not have a type field
	dst := reflect.ValueOf(expect).Elem()
	src := reflect.ValueOf(account).Elem()
	if !src.Type().AssignableTo(dst.Type()) {
		return nil, nil, fmt.Errorf("expected %v, got %v", dst.Type(), src.Type())
	}

	dst.Set(src)
	return account, acntRes, nil
}

func GetTokenUrlFromAccount(u *url2.URL) (*url2.URL, error) {
	var err error
	var tokenUrl *url2.URL
	if IsLiteAccount(u.String()) {
		_, tokenUrl, err = protocol.ParseLiteTokenAddress(u)
		if err != nil {
			return nil, fmt.Errorf("cannot extract token url from lite token account, %v", err)
		}
	} else {
		ta := new(protocol.TokenAccount)
		_, _, err := queryAccount(u.String(), ta)
		if err != nil {
			return nil, err
		}
		tokenUrl, err = url2.Parse(ta.TokenUrl)
		if err != nil {
			return nil, err
		}
	}
	if tokenUrl == nil {
		return nil, fmt.Errorf("invalid token url was obtained from %s", u.String())
	}
	return tokenUrl, nil
}

func getKey(url string, key []byte) (*query.ResponseKeyPageIndex, error) {
	params := new(api.KeyPageIndexQuery)
	params.Url = url
	params.Key = key

	qres, err := Client.QueryKeyPageIndex(context.Background(), params)
	if err != nil {
		ret, err := PrintJsonRpcError(err)
		if err != nil {
			return nil, err
		}

		return nil, fmt.Errorf("%v", ret)
	}

	res := new(query.ResponseKeyPageIndex)
	err = Remarshal(qres.Data, res)
	if err != nil {
		return nil, err
	}

	return res, nil
}
