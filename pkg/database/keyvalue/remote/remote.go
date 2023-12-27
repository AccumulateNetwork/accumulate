// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package remote

import (
	"bufio"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// Serve opens a batch and serves it over the connection. Serve returns once the
// connection is closed or the remote side calls Commit. See [Connect].
func Serve(db keyvalue.Beginner, conn io.ReadWriteCloser, prefix *record.Key, writable bool) error {
	defer conn.Close()
	batch := db.Begin(prefix, writable)
	defer batch.Discard()

	rd := bufio.NewReader(conn)
	run := true
	for run {
		c, err := read(rd, unmarshalCall)
		switch {
		case err == nil:
			// Ok
		case errors.Is(err, io.EOF):
			return nil
		default:
			return err
		}

		r := executeCall(batch, rd, conn, c, func(c call) response {
			switch c := c.(type) {
			case *commitCall:
				run = false
				err := batch.Commit()
				if err != nil {
					return errResp(err)
				} else {
					return new(okResponse)
				}

			default:
				return &unsupportedCallResponse{CallType: c.Type()}
			}
		})

		err = write(conn, r)
		if err != nil {
			return err
		}
	}
	return nil
}

// DB is a remote key-value database client that creates a connection to the
// remote database when [DB.Begin] is called. See [Serve].
type DB struct {
	connect func() (io.ReadWriteCloser, error)
}

// Connect returns a DB that uses the given function to connect to a remote
// key-value database. See [Serve].
func Connect(connect func() (io.ReadWriteCloser, error)) *DB {
	return &DB{connect: connect}
}

// Begin opens a connection to the remote key-value database and returns a
// change set that uses RPC to interact with the remote database.
func (c *DB) Begin(prefix *database.Key, writable bool) keyvalue.ChangeSet {
	conn, err := c.connect()

	var rd *bufio.Reader
	if err == nil {
		rd = bufio.NewReader(conn)
	}

	get := func(k *record.Key) ([]byte, error) {
		if err != nil {
			return nil, err
		}
		return c.get(rd, conn, k)
	}

	forEach := func(fn func(*record.Key, []byte) error) error {
		if err != nil {
			return err
		}
		return c.forEach(rd, conn, fn)
	}

	commit := func(entries map[[32]byte]memory.Entry) error {
		if err != nil {
			return err
		}
		return c.commit(rd, conn, entries)
	}

	discard := func() {
		if err == nil {
			_ = conn.Close()
		}
	}

	return memory.NewChangeSet(memory.ChangeSetOptions{
		Prefix:  prefix,
		Get:     get,
		ForEach: forEach,
		Commit:  commit,
		Discard: discard,
	})
}

func (c *DB) get(rd *bufio.Reader, wr io.Writer, key *record.Key) ([]byte, error) {
	r, err := roundTrip[*valueResponse](rd, wr, &getCall{Key: key})
	if err != nil {
		return nil, err
	}
	return r.Value, nil
}

func (c *DB) commit(rd *bufio.Reader, wr io.WriteCloser, entries map[[32]byte]memory.Entry) error {
	var err error
	for _, e := range entries {
		if e.Delete {
			_, err = roundTrip[*okResponse](rd, wr, &deleteCall{Key: e.Key})
		} else {
			_, err = roundTrip[*okResponse](rd, wr, &putCall{Key: e.Key, Value: e.Value})
		}
		if err != nil {
			return err
		}
	}

	_, err = roundTrip[*okResponse](rd, wr, &commitCall{})
	return err
}

func (c *DB) forEach(rd *bufio.Reader, wr io.Writer, fn func(*record.Key, []byte) error) error {
	return callForEach(rd, wr, forEachCall{}, fn)
}
