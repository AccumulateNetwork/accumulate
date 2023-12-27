// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package remote_test

import (
	"bufio"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/kvtest"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/remote"
)

func open(t testing.TB) kvtest.Opener {
	// Use a database that supports isolation
	store, err := badger.New(t.TempDir(), badger.WithPlainKeys)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, store.Close())
	})

	return func() (keyvalue.Beginner, error) {
		return remote.Connect(func() (io.ReadWriteCloser, error) {
			rd1, wr1 := io.Pipe()
			rd2, wr2 := io.Pipe()
			go func() { require.NoError(t, remote.Serve(store, &conn{rd1, wr2}, nil, true)) }()
			return &conn{rd2, wr1}, nil
		}), nil
	}
}

func TestDatabase(t *testing.T) {
	kvtest.TestDatabase(t, open(t))
}

func TestIsolation(t *testing.T) {
	kvtest.TestIsolation(t, open(t))
}

func TestSubBatch(t *testing.T) {
	kvtest.TestSubBatch(t, open(t))
}

func TestPrefix(t *testing.T) {
	kvtest.TestPrefix(t, open(t))
}

func TestDelete(t *testing.T) {
	kvtest.TestDelete(t, open(t))
}

func TestCloseWhenDone(t *testing.T) {
	t.Run("Closed", func(t *testing.T) {
		store := memory.New(nil)
		rd1, wr1 := io.Pipe()
		rd2, wr2 := io.Pipe()

		errch := make(chan error)
		go func() { errch <- remote.Serve(store, &conn{rd1, wr2}, nil, true) }()

		// Closing the connection should stop the server
		require.NoError(t, wr1.Close())
		require.NoError(t, rd2.Close())

		select {
		case err := <-errch:
			require.NoError(t, err)
		case <-time.After(time.Second):
			t.Fatal("Server did not close")
		}
	})

	t.Run("Commit", func(t *testing.T) {
		store := memory.New(nil)
		rd1, wr1 := io.Pipe()
		rd2, wr2 := io.Pipe()

		errch := make(chan error)
		go func() { errch <- remote.Serve(store, &conn{rd1, wr2}, nil, true) }()

		// Committing should stop the server (even if the connection doesn't close)
		require.NoError(t, remote.SendCommit(bufio.NewReader(rd2), wr1))

		select {
		case err := <-errch:
			require.NoError(t, err)
		case <-time.After(time.Second):
			t.Fatal("Server did not close")
		}
	})
}

type conn struct {
	io.ReadCloser
	io.WriteCloser
}

func (c *conn) Close() error {
	e1 := c.ReadCloser.Close()
	e2 := c.WriteCloser.Close()
	switch {
	case e1 != nil:
		return e1
	case e2 != nil:
		return e2
	}
	return nil
}
