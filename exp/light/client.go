// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package light

import (
	"io"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// Client is a light client instance.
type Client struct {
	v2          *client.Client
	query       api.Querier2
	store       keyvalue.Beginner
	storePrefix string
	router      routing.Router
}

type ClientOption func(c *Client) error

func BadgerStore(filepath string, opts ...badger.Option) ClientOption {
	return func(c *Client) error {
		// Open the badger database
		var err error
		c.store, err = badger.New(filepath, opts...)
		return errors.UnknownError.Wrap(err)
	}
}

func MemoryStore() ClientOption {
	return func(c *Client) error {
		c.store = memory.New(nil)
		return nil
	}
}

func Store(s keyvalue.Beginner, prefix string) ClientOption {
	return func(c *Client) error {
		c.store = s
		c.storePrefix = prefix
		return nil
	}
}

func Router(r routing.Router) ClientOption {
	return func(c *Client) error {
		c.router = r
		return nil
	}
}

func RouterFromStore() ClientOption {
	return func(c *Client) error {
		batch := c.OpenDB(false)
		defer batch.Discard()

		g := new(core.GlobalValues)
		err := g.Load(protocol.DnUrl(), func(accountUrl *url.URL, target interface{}) error {
			return batch.Account(accountUrl).Main().GetAs(target)
		})
		if err != nil {
			return errors.UnknownError.WithFormat("load globals: %w", err)
		}

		c.router = routing.NewRouter(routing.RouterOptions{Initial: g.Routing})
		return nil
	}
}

// Logger sets the logger.
//
// Deprecated: Unused - using slog instead.
func Logger(logger log.Logger, keyVals ...any) ClientOption {
	return func(c *Client) error {
		return nil
	}
}

func Server(server string) ClientOption {
	return func(c *Client) error {
		// Create the API client - use API v2 until v3 is deployed on the mainnet
		var err error
		c.v2, err = client.New(server)
		return errors.UnknownError.Wrap(err)
	}
}

func ClientV2(client *client.Client) ClientOption {
	return func(c *Client) error {
		c.v2 = client
		return nil
	}
}

func Querier(querier api.Querier) ClientOption {
	return func(c *Client) error {
		c.query.Querier = querier
		return nil
	}
}

// NewClient creates a new light client instance.
func NewClient(opts ...ClientOption) (*Client, error) {
	c := new(Client)

	for _, opt := range opts {
		err := opt(c)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	if c.store == nil {
		return nil, errors.BadRequest.WithFormat("missing store")
	}
	if c.v2 == nil {
		return nil, errors.BadRequest.WithFormat("missing server")
	}
	if c.query.Querier == nil {
		return nil, errors.BadRequest.WithFormat("missing querier")
	}

	return c, nil
}

// Close frees any resources opened by the light client.
func (c *Client) Close() error {
	if c, ok := c.store.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// OpenDB opens a [DB].
func (c *Client) OpenDB(writable bool) *DB {
	var prefix *record.Key
	if c.storePrefix != "" {
		prefix = record.NewKey(c.storePrefix)
	}
	return OpenDB(c.store, prefix, writable)
}

func OpenDB(store keyvalue.Beginner, prefix *record.Key, writable bool) *DB {
	db := new(DB)
	kvb := store.Begin(prefix, writable)
	db.Batch = database.NewBatch("", kvb, writable, nil)
	db.Batch.SetObserver(testing.NullObserver{}) // Ignore the BPT
	db.index = new(IndexDB)
	db.index.parent = db
	db.index.key = record.NewKey("Light", "Index")
	db.index.store = keyvalue.RecordStore{Store: kvb}
	return db
}

// DB is the light client database.
type DB struct {
	*database.Batch
	index *IndexDB
}

func (db *DB) Index() *IndexDB { return db.index }

// Commit commits changes to the database.
func (db *DB) Commit() error {
	// IndexDB reuses the main batch's key-value store so it must be committed
	// first
	err := db.index.Commit()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// This commits the main batch and the key-value store batch
	err = db.Batch.Commit()
	return errors.UnknownError.Wrap(err)
}
