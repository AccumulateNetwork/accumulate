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
	QuerierOptions
	clients []ABCIQueryClient
}

func (q *queryDispatch) direct(i uint64) *queryDirect {
	i = i % uint64(len(q.clients))
	return &queryDirect{q.QuerierOptions, q.clients[i]}
}

func (q *queryDispatch) routing(s string) (uint64, error) {
	u, err := url.Parse(s)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrInvalidUrl, err)
	}

	return u.Routing(), nil
}

func (q *queryDispatch) queryAll(query func(*queryDirect) (*ChainQueryResponse, error)) (*ChainQueryResponse, error) {
	resCh := make(chan *ChainQueryResponse) // Result channel
	errCh := make(chan error)               // Error channel
	doneCh := make(chan struct{})           // Completion channel
	wg := new(sync.WaitGroup)               // Wait for completion
	wg.Add(len(q.clients))                  //

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
	for _, c := range q.clients {
		go func(c ABCIQueryClient) {
			// Mark complete on return
			defer wg.Done()

			res, err := query(&queryDirect{q.QuerierOptions, c})
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
		}(c)
	}

	// Wait for an error or a result
	select {
	case res := <-resCh:
		return res, nil
	case err := <-errCh:
		return nil, err
	}
}

func (q *queryDispatch) queryAllTx(query func(*queryDirect) (*TransactionQueryResponse, error)) (*TransactionQueryResponse, error) {
	resCh := make(chan *TransactionQueryResponse) // Result channel
	errCh := make(chan error)                     // Error channel
	doneCh := make(chan struct{})                 // Completion channel
	wg := new(sync.WaitGroup)                     // Wait for completion
	wg.Add(len(q.clients))                        //

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
	for _, c := range q.clients {
		go func(c ABCIQueryClient) {
			// Mark complete on return
			defer wg.Done()

			res, err := query(&queryDirect{q.QuerierOptions, c})
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
		}(c)
	}

	// Wait for an error or a result
	select {
	case res := <-resCh:
		return res, nil
	case err := <-errCh:
		return nil, err
	}
}

func (q *queryDispatch) QueryUrl(url string) (interface{}, error) {
	r, err := q.routing(url)
	if err != nil {
		return nil, err
	}

	return q.direct(r).QueryUrl(url)
}

func (q *queryDispatch) QueryKeyPageIndex(url string, key []byte) (*ChainQueryResponse, error) {
	r, err := q.routing(url)
	if err != nil {
		return nil, err
	}

	return q.direct(r).QueryKeyPageIndex(url, key)
}

func (q *queryDispatch) QueryChain(id []byte) (*ChainQueryResponse, error) {
	res, err := q.queryAll(func(q *queryDirect) (*ChainQueryResponse, error) {
		return q.QueryChain(id)
	})
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (q *queryDispatch) QueryDirectory(url string, pagination QueryPagination, queryOptions QueryOptions) (*MultiResponse, error) {
	r, err := q.routing(url)
	if err != nil {
		return nil, err
	}

	return q.direct(r).QueryDirectory(url, pagination, queryOptions)
}

func (q *queryDispatch) QueryTx(id []byte, wait time.Duration) (*TransactionQueryResponse, error) {
	res, err := q.queryAllTx(func(q *queryDirect) (*TransactionQueryResponse, error) {
		return q.QueryTx(id, wait)
	})
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (q *queryDispatch) QueryTxHistory(url string, start, count uint64) (*MultiResponse, error) {
	r, err := q.routing(url)
	if err != nil {
		return nil, err
	}

	return q.direct(r).QueryTxHistory(url, start, count)
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
