package api

import (
	"errors"
	"fmt"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/networks/connections"
	"sync"
	"time"

	"github.com/AccumulateNetwork/accumulate/smt/storage"
)

type queryDispatch struct {
	QuerierOptions
	connRouter connections.ConnectionRouter
}

func (q *queryDispatch) direct(accUrl string) (*queryDirect, error) {
	url, err := url.Parse(accUrl)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidUrl, err)
	}
	route, err := q.connRouter.SelectRoute(url, true)
	if err != nil {
		return nil, err
	}
	return &queryDirect{q.QuerierOptions, route}, nil
}

func (q *queryDispatch) queryAll(query func(*queryDirect) (interface{}, error)) (interface{}, error) {
	resCh := make(chan interface{}) // Result channel
	errCh := make(chan error)       // Error channel
	doneCh := make(chan struct{})   // Completion channel

	allRoutes, err := q.connRouter.GetBvnRoutes()
	if err != nil {
		return nil, err
	}
	wg := new(sync.WaitGroup) // Wait for completion
	wg.Add(len(allRoutes))    //

	// Mark complete on return
	defer close(doneCh)

	go func() {
		// Wait for all queries to complete
		wg.Wait()

		// If all queries are done and no error or result has been produced, the
		// record must not exist
		select {
		case errCh <- storage.ErrNotFound:
		case <-doneCh:
		}
	}()

	// Create a request for each client in a separate goroutine
	for _, route := range allRoutes {
		finalRoute := route
		go func() {
			// Mark complete on return
			defer wg.Done()

			res, err := query(&queryDirect{q.QuerierOptions, finalRoute})
			switch {
			case err == nil:
				select {
				case resCh <- res:
					// Send the result
				case <-doneCh:
					// A result or error has already been sent
				}
			case !errors.Is(err, storage.ErrNotFound):
				select {
				case errCh <- err:
					// Send the error
				case <-doneCh:
					// A result or error has already been sent
				}
			}
		}()
	}

	// Wait for an error or a result
	select {
	case res := <-resCh:
		return res, nil
	case err := <-errCh:
		return nil, err
	}
}

func (q *queryDispatch) QueryUrl(url string, opts QueryOptions) (interface{}, error) {
	direct, err := q.direct(url)
	if err != nil {
		return nil, err
	}

	return direct.QueryUrl(url, opts)
}

func (q *queryDispatch) QueryKeyPageIndex(url string, key []byte) (*ChainQueryResponse, error) {
	direct, err := q.direct(url)
	if err != nil {
		return nil, err
	}
	return direct.QueryKeyPageIndex(url, key)
}

func (q *queryDispatch) QueryChain(id []byte) (*ChainQueryResponse, error) {
	res, err := q.queryAll(func(q *queryDirect) (interface{}, error) {
		return q.QueryChain(id)
	})
	if err != nil {
		return nil, err
	}

	return res.(*ChainQueryResponse), nil
}

func (q *queryDispatch) QueryDirectory(url string, pagination QueryPagination, queryOptions QueryOptions) (*MultiResponse, error) {
	direct, err := q.direct(url)
	if err != nil {
		return nil, err
	}

	return direct.QueryDirectory(url, pagination, queryOptions)
}

func (q *queryDispatch) QueryTx(id []byte, wait time.Duration, opts QueryOptions) (*TransactionQueryResponse, error) {
	res, err := q.queryAll(func(q *queryDirect) (interface{}, error) {
		return q.QueryTx(id, wait, opts)
	})
	if err != nil {
		return nil, err
	}

	return res.(*TransactionQueryResponse), nil
}

func (q *queryDispatch) QueryTxHistory(url string, pagination QueryPagination) (*MultiResponse, error) {
	direct, err := q.direct(url)
	if err != nil {
		return nil, err
	}

	return direct.QueryTxHistory(url, pagination)
}

func (q *queryDispatch) QueryData(url string, entryHash [32]byte) (*ChainQueryResponse, error) {
	direct, err := q.direct(url)
	if err != nil {
		return nil, err
	}

	return direct.QueryData(url, entryHash)
}

func (q *queryDispatch) QueryDataSet(url string, pagination QueryPagination, queryOptions QueryOptions) (*MultiResponse, error) {
	direct, err := q.direct(url)
	if err != nil {
		return nil, err
	}

	return direct.QueryDataSet(url, pagination, queryOptions)
}
