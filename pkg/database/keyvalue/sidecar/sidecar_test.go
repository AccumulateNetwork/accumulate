// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package sidecar

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

func put(t *testing.T, db keyvalue.Beginner, key *record.Key, value string) {
	t.Helper()
	b := db.Begin(nil, true)
	defer b.Discard()
	require.NoError(t, b.Put(key, []byte(value)))
	require.NoError(t, b.Commit())
}

func get(db keyvalue.Beginner, key *record.Key) ([]byte, error) {
	b := db.Begin(nil, false)
	defer b.Discard()
	return b.Get(key)
}

// serve runs store behind a real listener and returns a client for it.
func serve(t *testing.T, store keyvalue.Beginner) *Client {
	srv := httptest.NewServer(Handler(store))
	t.Cleanup(srv.Close)
	return &Client{URL: srv.URL}
}

func TestReadThrough(t *testing.T) {
	side := memory.New(nil)
	live := memory.New(nil)
	onlySide := record.NewKey("Account", "old.acme", "Main")
	both := record.NewKey("Account", "both.acme", "Main")
	neither := record.NewKey("Account", "none.acme", "Main")
	put(t, side, onlySide, "from the sidecar")
	put(t, side, both, "stale")
	put(t, live, both, "live")

	db := Over(live, serve(t, side))

	v, err := get(db, onlySide)
	require.NoError(t, err)
	require.Equal(t, "from the sidecar", string(v))

	v, err = get(db, both)
	require.NoError(t, err)
	require.Equal(t, "live", string(v), "live wins over the sidecar")

	_, err = get(db, neither)
	require.ErrorIs(t, err, errors.NotFound)

	// The node keeps no copy of what it read
	_, err = get(live, onlySide)
	require.ErrorIs(t, err, errors.NotFound, "a sidecar hit was written to live")
}

// A prefixed batch, as the database layer opens them, reads the prefixed key.
func TestReadThroughPrefix(t *testing.T) {
	side := memory.New(nil)
	prefix := record.NewKey("BVN1")
	key := record.NewKey("Account", "old.acme", "Main")
	put(t, side, prefix.AppendKey(key), "prefixed")

	db := Over(memory.New(nil), serve(t, side))
	b := db.Begin(prefix, false)
	defer b.Discard()
	v, err := b.Get(key)
	require.NoError(t, err)
	require.Equal(t, "prefixed", string(v))
}

// A sidecar that cannot answer leaves a node answering as one without it.
func TestFailsSoft(t *testing.T) {
	key := record.NewKey("Account", "old.acme", "Main")

	t.Run("Unreachable", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		url := srv.URL
		srv.Close()
		_, err := get(Over(memory.New(nil), &Client{URL: url}), key)
		require.ErrorIs(t, err, errors.NotFound)
	})

	t.Run("Broken", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "broken", http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)
		logged := new(bytes.Buffer)
		c := &Client{URL: srv.URL, Logger: slog.New(slog.NewTextHandler(logged, nil))}
		_, err := get(Over(memory.New(nil), c), key)
		require.ErrorIs(t, err, errors.NotFound)
		require.Contains(t, logged.String(), "Sidecar read failed")
	})
}

func TestClientIsReadOnly(t *testing.T) {
	c := serve(t, memory.New(nil))
	b := c.Begin(nil, true)
	defer b.Discard()
	require.NoError(t, b.Put(record.NewKey("x"), []byte("y")))
	require.ErrorIs(t, b.Commit(), errors.NotAllowed)
	require.ErrorIs(t, c.Begin(nil, false).ForEach(func(*record.Key, []byte) error { return nil }), errors.NotAllowed)
}

func TestHandlerRejectsBadKeys(t *testing.T) {
	srv := httptest.NewServer(Handler(memory.New(nil)))
	t.Cleanup(srv.Close)
	for path, want := range map[string]int{
		"/key/zz":   http.StatusBadRequest,
		"/key/abcd": http.StatusBadRequest,
		"/other":    http.StatusNotFound,
	} {
		res, err := http.Get(srv.URL + path)
		require.NoError(t, err)
		res.Body.Close()
		require.Equal(t, want, res.StatusCode, path)
	}
}
