package api

import (
	"errors"
	"fmt"
	"github.com/AccumulateNetwork/accumulate/networks/connections"
	"time"

	"github.com/AccumulateNetwork/accumulate/smt/storage"
)

type queryDispatch struct {
	QuerierOptions
	connRouter connections.ConnectionRouter
}

func (q *queryDispatch) direct(accUrl string) (*queryDirect, error) {
	route, err := q.connRouter.AcquireRoute(accUrl, true)
	if err != nil {
		return nil, err
	}
	return &queryDirect{q.QuerierOptions, route}, nil
}

func (q *queryDispatch) queryAll(query func(*queryDirect) (*QueryResponse, error)) ([]*QueryResponse, error) {
	res := make([]*QueryResponse, 0, 1)
	allRoutes, err := q.connRouter.AcquireAll()
	if err != nil {
		return nil, err
	}

	for _, route := range allRoutes {
		r, err := query(&queryDirect{q.QuerierOptions, route})
		if err == nil {
			res = append(res, r)
		} else if !errors.Is(err, storage.ErrNotFound) {
			return nil, err
		}
	}

	if len(res) == 0 {
		return nil, storage.ErrNotFound
	}

	return res, nil
}

func (q *queryDispatch) QueryUrl(url string) (*QueryResponse, error) {
	direct, err := q.direct(url)
	if err != nil {
		return nil, err
	}
	return direct.QueryUrl(url)
}

func (q *queryDispatch) QueryChain(id []byte) (*QueryResponse, error) {
	res, err := q.queryAll(func(q *queryDirect) (*QueryResponse, error) {
		return q.QueryChain(id)
	})
	if err != nil {
		return nil, err
	}

	if len(res) > 1 {
		return nil, fmt.Errorf("found chain %X on multiple networks", id)
	}

	return res[0], nil
}

func (q *queryDispatch) QueryDirectory(url string, pagination *QueryPagination, queryOptions *QueryOptions) (*QueryResponse, error) {
	direct, err := q.direct(url)
	if err != nil {
		return nil, err
	}

	return direct.QueryDirectory(url, pagination, queryOptions)
}

func (q *queryDispatch) QueryTx(id []byte, wait time.Duration) (*QueryResponse, error) {
	res, err := q.queryAll(func(q *queryDirect) (*QueryResponse, error) {
		return q.QueryTx(id, wait)
	})
	if err != nil {
		return nil, err
	}

	if len(res) > 1 {
		return nil, fmt.Errorf("found TX %X on multiple networks", id)
	}

	return res[0], nil
}

func (q *queryDispatch) QueryTxHistory(url string, start, count int64) (*QueryMultiResponse, error) {
	direct, err := q.direct(url)
	if err != nil {
		return nil, err
	}

	return direct.QueryTxHistory(url, start, count)
}
