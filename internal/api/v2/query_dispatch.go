package api

import (
	"errors"
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
	route, err := q.connRouter.SelectRoute(accUrl, true)
	if err != nil {
		return nil, err
	}
	return &queryDirect{q.QuerierOptions, route}, nil
}

func (q *queryDispatch) queryAll(query func(*queryDirect) (*QueryResponse, error)) (*QueryResponse, error) {
	resCh := make(chan *QueryResponse) // Result channel
	errCh := make(chan error)          // Error channel
	doneCh := make(chan struct{})      // Completion channel

	allRoutes, err := q.connRouter.GetAllBVNs() // TODO Only BVNs or really all?
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
		go func() {
			// Mark complete on return
			defer wg.Done()

			res, err := query(&queryDirect{q.QuerierOptions, route})
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

func (q *queryDispatch) QueryUrl(url string) (*QueryResponse, error) {
	direct, err := q.direct(url)
	if err != nil {
		return nil, err
	}
	return direct.QueryUrl(url)
}

func (q *queryDispatch) QueryKeyPageIndex(url string, key []byte) (*QueryResponse, error) {
	direct, err := q.direct(url)
	if err != nil {
		return nil, err
	}
	return direct.QueryKeyPageIndex(url, key)
}

func (q *queryDispatch) QueryChain(id []byte) (*QueryResponse, error) {
	res, err := q.queryAll(func(q *queryDirect) (*QueryResponse, error) {
		return q.QueryChain(id)
	})
	if err != nil {
		return nil, err
	}

	return res, nil
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

	return res, nil
}

func (q *queryDispatch) QueryTxHistory(url string, start, count int64) (*QueryMultiResponse, error) {
	direct, err := q.direct(url)
	if err != nil {
		return nil, err
	}

	return direct.QueryTxHistory(url, start, count)
}

func (q *queryDispatch) QueryData(url string, entryHash []byte) (*QueryResponse, error) {
	direct, err := q.direct(url)
	if err != nil {
		return nil, err
	}

	return direct.QueryData(url, entryHash)
}

func (q *queryDispatch) QueryDataSet(url string, pagination *QueryPagination, queryOptions *QueryOptions) (*QueryResponse, error) {
	direct, err := q.direct(url)
	if err != nil {
		return nil, err
	}

	return direct.QueryDataSet(url, pagination, queryOptions)
}
