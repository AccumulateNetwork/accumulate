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

func (m *JrpcMethods) QueryKeyPageIndex(_ context.Context, params json.RawMessage) interface{} {
	req := new(KeyPageIndexQuery)
	err := m.parse(params, req)
	if err != nil {
		return err
	}

	return jrpcFormatQuery(m.opts.Query.QueryKeyPageIndex(req.Url, req.Key))
}

func (m *JrpcMethods) QueryDirectory(_ context.Context, params json.RawMessage) interface{} {
	req := new(DirectoryQuery)
	err := m.parse(params, req)
	if err != nil {
		return err
	}

	res, err := m.opts.Query.QueryDirectory(req.Url, &req.QueryPagination, &req.QueryOptions)
	if err != nil {
		return accumulateError(err)
	}

	return res
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
	req := new(TxnQuery)
	err := m.parse(params, req)
	if err != nil {
		return err
	}

	return jrpcFormatQuery(m.opts.Query.QueryTx(req.Txid, req.Wait))
}

func (m *JrpcMethods) QueryTxHistory(_ context.Context, params json.RawMessage) interface{} {
	req := new(TxHistoryQuery)
	err := m.parse(params, req)
	if err != nil {
		return err
	}

	// If the user wants nothing, give them nothing
	if req.Count == 0 {
		return validatorError(errors.New("count must be greater than 0"))
	}

	res, err := m.opts.Query.QueryTxHistory(req.Url, int64(req.Start), int64(req.Count))
	if err != nil {
		return accumulateError(err)
	}

	return res
}

func (m *JrpcMethods) QueryData(_ context.Context, params json.RawMessage) interface{} {
	req := new(DataEntryQuery)
	err := m.parse(params, req)
	if err != nil {
		return err
	}

	res, err := m.opts.Query.QueryData(req.Url, req.EntryHash[:])
	if err != nil {
		return accumulateError(err)
	}

	return res
}

func (m *JrpcMethods) QueryDataSet(_ context.Context, params json.RawMessage) interface{} {
	req := new(DataEntrySetQuery)
	err := m.parse(params, req)
	if err != nil {
		return err
	}

	// If the user wants nothing, give them nothing
	if req.Count == 0 {
		return validatorError(errors.New("count must be greater than 0"))
	}

	res, err := m.opts.Query.QueryDataSet(req.Url, &req.QueryPagination, &req.QueryOptions)
	if err != nil {
		return accumulateError(err)
	}

	return res
}
