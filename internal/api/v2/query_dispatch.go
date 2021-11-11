package api

import (
	"errors"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/smt/storage"
)

type queryDispatch struct {
	clients []ABCIQueryClient
}

func (q queryDispatch) direct(i uint64) queryDirect {
	i = i % uint64(len(q.clients))
	return queryDirect{q.clients[i]}
}

func (q queryDispatch) routing(s string) (uint64, error) {
	u, err := url.Parse(s)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrInvalidUrl, err)
	}

	return u.Routing(), nil
}

func (q queryDispatch) queryAll(query func(queryDirect) (*QueryResponse, error)) ([]*QueryResponse, error) {
	res := make([]*QueryResponse, 0, 1)
	for _, c := range q.clients {
		r, err := query(queryDirect{c})
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

func (q queryDispatch) QueryUrl(url string) (*QueryResponse, error) {
	r, err := q.routing(url)
	if err != nil {
		return nil, err
	}

	return q.direct(r).QueryUrl(url)
}

func (q queryDispatch) QueryChain(id []byte) (*QueryResponse, error) {
	res, err := q.queryAll(func(q queryDirect) (*QueryResponse, error) {
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

func (q queryDispatch) QueryDirectory(url string) (*QueryResponse, error) {
	r, err := q.routing(url)
	if err != nil {
		return nil, err
	}

	return q.direct(r).QueryDirectory(url)
}

func (q queryDispatch) QueryTx(id []byte) (*QueryResponse, error) {
	res, err := q.queryAll(func(q queryDirect) (*QueryResponse, error) {
		return q.QueryTx(id)
	})
	if err != nil {
		return nil, err
	}

	if len(res) > 1 {
		return nil, fmt.Errorf("found TX %X on multiple networks", id)
	}

	return res[0], nil
}

func (q queryDispatch) QueryTxHistory(url string, start, count int64) (*QueryMultiResponse, error) {
	r, err := q.routing(url)
	if err != nil {
		return nil, err
	}

	return q.direct(r).QueryTxHistory(url, start, count)
}
