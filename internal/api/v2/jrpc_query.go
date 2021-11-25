package api

import (
	"context"
	"encoding/json"
	"errors"
)

func (m *JrpcMethods) Query(_ context.Context, params json.RawMessage) interface{} {
	req := new(UrlQuery)
	err := m.parse(params, req)
	if err != nil {
		return err
	}

	return jrpcFormatQuery(m.opts.Query.QueryUrl(req.Url))
}

func (m *JrpcMethods) QueryDirectory(_ context.Context, params json.RawMessage) interface{} {
	urlQuery := new(UrlQuery)
	err := m.parse(params, urlQuery)
	if err != nil {
		return err
	}
	queryOptions := new(QueryOptions)
	err = m.parse(params, urlQuery)
	if err != nil {
		return err
	}

	return jrpcFormatQuery(m.opts.Query.QueryDirectory(urlQuery.Url, queryOptions.ExpandChains))
}

func (m *JrpcMethods) QueryChain(_ context.Context, params json.RawMessage) interface{} {
	req := new(ChainIdQuery)
	err := m.parse(params, req)
	if err != nil {
		return err
	}

	return jrpcFormatQuery(m.opts.Query.QueryChain(req.ChainId))
}

func (m *JrpcMethods) QueryTx(_ context.Context, params json.RawMessage) interface{} {
	req := new(TxIdQuery)
	err := m.parse(params, req)
	if err != nil {
		return err
	}

	return jrpcFormatQuery(m.opts.Query.QueryTx(req.Txid))
}

func (m *JrpcMethods) QueryTxHistory(_ context.Context, params json.RawMessage) interface{} {
	urlQuery := new(UrlQuery)
	err := m.parse(params, urlQuery)
	if err != nil {
		return err
	}
	queryPagination := new(QueryPagination)
	err = m.parse(params, urlQuery)
	if err != nil {
		return err
	}

	// If the user wants nothing, give them nothing
	if queryPagination.Count == 0 {
		return validatorError(errors.New("count must be greater than 0"))
	}

	res, err := m.opts.Query.QueryTxHistory(urlQuery.Url, int64(queryPagination.Start), int64(queryPagination.Count))
	if err != nil {
		return accumulateError(err)
	}

	return res
}
