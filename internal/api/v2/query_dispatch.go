package api

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
)

type queryDispatch struct {
	Options
}

func (q *queryDispatch) direct(s string) *queryDirect {
	return &queryDirect{q.Options, s}
}

func (q *queryDispatch) queryAll(query func(*queryDirect) (interface{}, error), errNotFound error) (interface{}, error) {
	resCh := make(chan interface{}) // Result channel
	errCh := make(chan error)       // Error channel
	doneCh := make(chan struct{})   // Completion channel
	wg := new(sync.WaitGroup)       // Wait for completion

	wg.Add(len(q.Describe.Network.Partitions)) //

	// Mark complete on return
	defer close(doneCh)

	go func() {
		// Wait for all queries to complete
		wg.Wait()

		// If all queries are done and no error or result has been produced, the
		// record must not exist
		select {
		case errCh <- errNotFound:
		case <-doneCh:
		}
	}()

	// Create a request for each client in a separate goroutine
	for _, partition := range q.Describe.Network.Partitions {
		go func(partitionId string) {
			// Mark complete on return
			defer wg.Done()

			res, err := query(q.direct(partitionId))
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
		}(partition.Id)
	}

	// Wait for an error or a result
	select {
	case res := <-resCh:
		return res, nil
	case err := <-errCh:
		return nil, err
	}
}

func (q *queryDispatch) QueryUrl(url *url.URL, opts QueryOptions) (interface{}, error) {
	r, err := q.Router.RouteAccount(url)
	if err != nil {
		return nil, err
	}

	return q.direct(r).QueryUrl(url, opts)
}

func (q *queryDispatch) QueryKeyPageIndex(url *url.URL, key []byte) (*ChainQueryResponse, error) {
	r, err := q.Router.RouteAccount(url)
	if err != nil {
		return nil, err
	}

	return q.direct(r).QueryKeyPageIndex(url, key)
}

func (q *queryDispatch) QueryChain(id []byte) (*ChainQueryResponse, error) {
	res, err := q.queryAll(func(q *queryDirect) (interface{}, error) {
		return q.QueryChain(id)
	}, fmt.Errorf("chain %X not found", id))
	if err != nil {
		return nil, err
	}

	return res.(*ChainQueryResponse), nil
}

func (q *queryDispatch) QueryDirectory(url *url.URL, pagination QueryPagination, queryOptions QueryOptions) (*MultiResponse, error) {
	r, err := q.Router.RouteAccount(url)
	if err != nil {
		return nil, err
	}

	return q.direct(r).QueryDirectory(url, pagination, queryOptions)
}

func (q *queryDispatch) QueryTx(id []byte, wait time.Duration, ignorePending bool, opts QueryOptions) (*TransactionQueryResponse, error) {
	res, err := q.queryAll(func(qdr *queryDirect) (interface{}, error) {
		return qdr.QueryTx(id, wait, ignorePending, opts)
	}, fmt.Errorf("transaction %X not found", id))
	if err != nil {
		return nil, err
	}

	return res.(*TransactionQueryResponse), nil
}

func (q *queryDispatch) QueryTxHistory(url *url.URL, pagination QueryPagination, scratch bool) (*MultiResponse, error) {
	r, err := q.Router.RouteAccount(url)
	if err != nil {
		return nil, err
	}

	return q.direct(r).QueryTxHistory(url, pagination, scratch)
}

func (q *queryDispatch) QueryData(url *url.URL, entryHash [32]byte) (*ChainQueryResponse, error) {
	r, err := q.Router.RouteAccount(url)
	if err != nil {
		return nil, err
	}

	return q.direct(r).QueryData(url, entryHash)
}

func (q *queryDispatch) QueryDataSet(url *url.URL, pagination QueryPagination, queryOptions QueryOptions) (*MultiResponse, error) {
	r, err := q.Router.RouteAccount(url)
	if err != nil {
		return nil, err
	}

	return q.direct(r).QueryDataSet(url, pagination, queryOptions)
}

func (q *queryDispatch) QueryMinorBlocks(url *url.URL, pagination QueryPagination, txFetchMode query.TxFetchMode, blockFilter query.BlockFilterMode) (*MultiResponse, error) {
	r, err := q.Router.RouteAccount(url)
	if err != nil {
		return nil, err
	}

	return q.direct(r).QueryMinorBlocks(url, pagination, txFetchMode, blockFilter)
}

func (q *queryDispatch) QueryMajorBlocks(url *url.URL, pagination QueryPagination) (*MultiResponse, error) {
	r, err := q.Router.RouteAccount(url)
	if err != nil {
		return nil, err
	}

	return q.direct(r).QueryMajorBlocks(url, pagination)
}

func (q queryDispatch) QuerySynth(source, destination *url.URL, number uint64, anchor bool) (*TransactionQueryResponse, error) {
	r, err := q.Router.RouteAccount(source)
	if err != nil {
		return nil, err
	}

	return q.direct(r).QuerySynth(source, destination, number, anchor)
}
