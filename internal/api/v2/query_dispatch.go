package api

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
)

type queryDispatch struct {
	Options
}

func (q *queryDispatch) direct(s string) *queryDirect {
	return &queryDirect{q.Options, s}
}

func (q *queryDispatch) routing(s string) (string, error) {
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidUrl, err)
	}

	return q.Router.Route(u)
}

func (q *queryDispatch) queryAll(query func(*queryDirect) (interface{}, error)) (interface{}, error) {
	resCh := make(chan interface{})  // Result channel
	errCh := make(chan error)        // Error channel
	doneCh := make(chan struct{})    // Completion channel
	wg := new(sync.WaitGroup)        // Wait for completion
	wg.Add(len(q.Network.Addresses)) //

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
	for subnet := range q.Network.Addresses {
		go func(subnet string) {
			// Mark complete on return
			defer wg.Done()

			res, err := query(q.direct(subnet))
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
		}(subnet)
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
	r, err := q.routing(url)
	if err != nil {
		return nil, err
	}

	return q.direct(r).QueryUrl(url, opts)
}

func (q *queryDispatch) QueryKeyPageIndex(url string, key []byte) (*ChainQueryResponse, error) {
	r, err := q.routing(url)
	if err != nil {
		return nil, err
	}

	return q.direct(r).QueryKeyPageIndex(url, key)
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
	r, err := q.routing(url)
	if err != nil {
		return nil, err
	}

	return q.direct(r).QueryDirectory(url, pagination, queryOptions)
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
	r, err := q.routing(url)
	if err != nil {
		return nil, err
	}

	return q.direct(r).QueryTxHistory(url, pagination)
}

func (q *queryDispatch) QueryData(url string, entryHash [32]byte) (*ChainQueryResponse, error) {
	r, err := q.routing(url)
	if err != nil {
		return nil, err
	}

	return q.direct(r).QueryData(url, entryHash)
}

func (q *queryDispatch) QueryDataSet(url string, pagination QueryPagination, queryOptions QueryOptions) (*MultiResponse, error) {
	r, err := q.routing(url)
	if err != nil {
		return nil, err
	}

	return q.direct(r).QueryDataSet(url, pagination, queryOptions)
}
