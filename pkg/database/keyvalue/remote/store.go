// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package remote

import (
	"bufio"
	"crypto/sha256"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// ServeStore serves a key-value store over a connection. ServeStore returns
// once the connection is closed. See [ConnectStore].
func ServeStore(store keyvalue.Store, conn io.ReadWriteCloser) error {
	rd := bufio.NewReader(conn)
	for {
		c, err := read(rd, unmarshalCall)
		switch {
		case err == nil:
			// Ok
		case errors.Is(err, io.EOF):
			return nil
		default:
			return err
		}

		r := executeCall(store, rd, conn, c, func(c call) response {
			return &unsupportedCallResponse{CallType: c.Type()}
		})

		err = write(conn, r)
		if err != nil {
			return err
		}
	}
}

// Store is a remote key-value store client that uses RPC to interact with the
// remote store. See [ServeStore].
type Store struct {
	rd *bufio.Reader
	wr io.Writer
}

// ConnectStore returns a Store that uses that uses RPC to interact with a
// remote store. See [ServeStore].
func ConnectStore(conn io.ReadWriter) *Store {
	return &Store{rd: bufio.NewReader(conn), wr: conn}
}

// Get implements [keyvalue.Store.Get].
func (c *Store) Get(key *record.Key) ([]byte, error) {
	r, err := roundTrip[*valueResponse](c.rd, c.wr, &getCall{keyOrHash: wrap(key)})
	if err != nil {
		return nil, err
	}
	return r.Value, nil
}

// GetBatch is a batched version of [Store.Get]. The request and response are
// transmitted as a single round-trip, which avoids the overhead of individual
// Get calls.
func (c *Store) GetBatch(keys []*record.Key) ([][]byte, error) {
	calls := make([]call, 0, len(keys))
	for _, key := range keys {
		calls = append(calls, &getCall{keyOrHash: wrap(key)})
	}
	r, err := roundTrip[*batchResponse](c.rd, c.wr, &batchCall{Calls: calls})
	if err != nil {
		return nil, err
	}
	values := make([][]byte, 0, len(keys))
	for _, r := range r.Responses {
		r, err := asResponse[*valueResponse](r)
		if err != nil {
			return nil, err
		}
		values = append(values, r.Value)
	}
	return values, nil
}

// Put implements [keyvalue.Store.Put].
func (c *Store) Put(key *record.Key, value []byte) error {
	_, err := roundTrip[*okResponse](c.rd, c.wr, &putCall{keyOrHash: wrap(key), Value: value})
	if err != nil {
		return err
	}
	return nil
}

// Delete implements [keyvalue.Store.Delete].
func (c *Store) Delete(key *record.Key) error {
	_, err := roundTrip[*okResponse](c.rd, c.wr, &deleteCall{keyOrHash: wrap(key)})
	if err != nil {
		return err
	}
	return nil
}

// ForEach implements [keyvalue.Store.ForEach].
func (c *Store) ForEach(fn func(*record.Key, []byte) error) error {
	return callForEach(c.rd, c.wr, forEachCall{}, fn)
}

// ForEachHash calls the callback for each key-value pair in the store. The
// callback is called with the value's hash instead of the value. See
// [Store.ForEach] for comparison.
func (c *Store) ForEachHash(fn func(*record.Key, [32]byte) error) error {
	return callForEach(c.rd, c.wr, forEachCall{Hash: true}, func(key *record.Key, hash []byte) error {
		return fn(key, *(*[32]byte)(hash))
	})
}

func callForEach(rd *bufio.Reader, wr io.Writer, d forEachCall, fn func(*record.Key, []byte) error) error {
	err := write(wr, &d)
	if err != nil {
		return err
	}

	for {
		r, err := read(rd, unmarshalResponse)
		if err != nil {
			return err
		}

		switch r := r.(type) {
		case *okResponse:
			return nil
		case *errorResponse:
			return r.Error
		case *entryResponse:
			err = fn(r.unwrap(), r.Value)
		case *batchResponse:
			for _, r := range r.Responses {
				var e *entryResponse
				e, err = asResponse[*entryResponse](r)
				if err != nil {
					break
				}
				err = fn(e.unwrap(), e.Value)
				if err != nil {
					break
				}
			}
		default:
			err = errors.InternalError.WithFormat("unexpected response type %T", r)
		}
		if err == nil {
			r = &okResponse{}
		} else {
			r = errResp(err)
		}
		err = write(wr, r)
		if err != nil {
			return err
		}
	}
}

func executeCall(store keyvalue.Store, rd *bufio.Reader, wr io.Writer, c call, other func(call) response) response {
	switch c := c.(type) {
	case *getCall:
		v, err := store.Get(c.unwrap())
		if err != nil {
			return errResp(err)
		} else {
			return &valueResponse{Value: v}
		}

	case *putCall:
		err := store.Put(c.unwrap(), c.Value)
		if err != nil {
			return errResp(err)
		} else {
			return new(okResponse)
		}

	case *deleteCall:
		err := store.Delete(c.unwrap())
		if err != nil {
			return errResp(err)
		} else {
			return new(okResponse)
		}

	case *forEachCall:
		const N, S = 10000, 16 << 20
		var batch []response
		var size uint64
		err := store.ForEach(func(key *record.Key, value []byte) error {
			if c.Hash {
				h := sha256.Sum256(value)
				value = h[:]
			} else {
				v := make([]byte, len(value))
				copy(v, value)
				value = v
			}

			// If the batch count or size (ignoring keys) is greater than the
			// threshold, send the batch
			size += uint64(len(value))
			batch = append(batch, &entryResponse{keyOrHash: wrap(key), Value: value})
			if len(batch) < N && size < S {
				return nil
			}

			_, err := roundTrip[*okResponse](rd, wr, &batchResponse{Responses: batch})
			batch, size = batch[:0], 0
			return err
		})
		if err == nil && len(batch) > 0 {
			_, err = roundTrip[*okResponse](rd, wr, &batchResponse{Responses: batch})
		}
		if err != nil {
			return errResp(err)
		}
		return &okResponse{}

	case *batchCall:
		r := make([]response, 0, len(c.Calls))
		for _, c := range c.Calls {
			r = append(r, executeCall(store, rd, wr, c, other))
		}
		return &batchResponse{Responses: r}

	default:
		return other(c)
	}
}

func errResp(err error) response {
	if err, ok := err.(*database.NotFoundError); ok {
		return &notFoundResponse{keyOrHash: wrap((*record.Key)(err))}
	}
	return &errorResponse{Error: errors.UnknownError.Wrap(err).(*errors.Error)}
}

func roundTrip[T response](rd *bufio.Reader, wr io.Writer, d encoding.BinaryValue) (T, error) {
	err := write(wr, d)
	if err != nil {
		var z T
		return z, err
	}
	return readResponse[T](rd)
}

func readResponse[T response](rd *bufio.Reader) (T, error) {
	r, err := read(rd, unmarshalResponse)
	if err != nil {
		var z T
		return z, err
	}

	return asResponse[T](r)
}

func asResponse[T response](r response) (T, error) {
	var z T
	switch r := r.(type) {
	case T:
		return r, nil
	case *notFoundResponse:
		return z, (*database.NotFoundError)(r.unwrap())
	case *errorResponse:
		return z, r.Error
	case *unsupportedCallResponse:
		return z, errors.NotAllowed.WithFormat("unsupported call type %v", r.CallType)
	default:
		return z, errors.InternalError.WithFormat("unexpected response type %T", r)
	}
}
