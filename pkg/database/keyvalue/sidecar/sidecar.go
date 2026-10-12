// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package sidecar serves the sidecar (#4273), the history the 13 July 2025
// reorg dropped, from one node to others over HTTP (#4561). A node reads it
// through [Over] instead of holding its own copy: a hop per miss, in exchange
// for not carrying the sidecar's 182 GB.
//
// A record travels as its storage key, the 32-byte hash of its record key: GET
// /key/<hex> answers 200 with the raw value, or 404 if the sidecar does not
// hold it.
package sidecar

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/overlay"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

const keyPath = "/key/"

// maxValue bounds what the client reads from one answer. The largest record
// in the sidecar is far smaller; this only stops a broken server.
const maxValue = 64 << 20

// DefaultTimeout bounds one read from the sidecar.
const DefaultTimeout = 2 * time.Second

// Handler serves store's values by storage key. It only reads.
func Handler(store keyvalue.Beginner) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, keyPath) {
			http.NotFound(w, r)
			return
		}
		b, err := hex.DecodeString(r.URL.Path[len(keyPath):])
		if err != nil || len(b) != 32 {
			http.Error(w, "the key must be 32 bytes of hex", http.StatusBadRequest)
			return
		}

		batch := store.Begin(nil, false)
		defer batch.Discard()
		v, err := batch.Get(record.KeyFromHash(record.KeyHash(b)))
		switch {
		case err == nil:
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(v)
		case errors.Is(err, errors.NotFound):
			http.NotFound(w, r)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}

// Client reads a sidecar that [Handler] serves. It is read-only, and it fails
// soft: a read the sidecar cannot answer, because it is unreachable, slow or
// broken, is logged and reported as not found, so a node whose sidecar is down
// answers as a node without one.
type Client struct {
	URL     string
	Timeout time.Duration
	HTTP    *http.Client
	Logger  *slog.Logger
}

// Begin implements [keyvalue.Beginner]. The change set only reads.
func (c *Client) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	return memory.NewChangeSet(memory.ChangeSetOptions{
		Prefix: prefix,
		Get:    c.get,
		ForEach: func(func(*record.Key, []byte) error) error {
			return errors.NotAllowed.With("the sidecar cannot be walked")
		},
		Commit: func(map[[32]byte]memory.Entry) error {
			return errors.NotAllowed.With("the sidecar is read-only")
		},
	})
}

func (c *Client) get(key *record.Key) ([]byte, error) {
	v, found, err := c.fetch(key.Hash())
	switch {
	case err != nil:
		if c.Logger != nil {
			c.Logger.Warn("Sidecar read failed", "key", key, "error", err)
		}
		return nil, (*database.NotFoundError)(key)
	case !found:
		return nil, (*database.NotFoundError)(key)
	}
	return v, nil
}

func (c *Client) fetch(h record.KeyHash) ([]byte, bool, error) {
	timeout := c.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(c.URL, "/")+keyPath+hex.EncodeToString(h[:]), nil)
	if err != nil {
		return nil, false, err
	}
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	res, err := hc.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK:
		v, err := io.ReadAll(io.LimitReader(res.Body, maxValue))
		if err != nil {
			return nil, false, err
		}
		return v, true, nil
	case http.StatusNotFound:
		return nil, false, nil
	default:
		return nil, false, fmt.Errorf("sidecar answered %s", res.Status)
	}
}

// Over returns live with the sidecar behind it: a read that misses live is
// read from the sidecar. Nothing read from the sidecar is written to live —
// the node keeps no copy — and every write goes to live.
//
// Only the API's database is opened this way. The executor reads live alone,
// so what a node agrees on never depends on whether it has a sidecar.
func Over(live keyvalue.Beginner, side *Client) keyvalue.Beginner {
	return overlay.Open(live, side)
}
