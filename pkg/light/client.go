// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package light

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/testing"
)

type Client struct {
	v2    *client.Client
	store storage.KeyValueStore
}

func (c *Client) router(batch *DB) (routing.Router, error) {
	g := new(core.GlobalValues)
	err := g.Load(config.NetworkUrl{URL: protocol.DnUrl()}, func(accountUrl *url.URL, target interface{}) error {
		return batch.Account(accountUrl).Main().GetAs(target)
	})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load globals: %w", err)
	}

	router, err := routing.NewStaticRouter(g.Routing, nil, nil)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("construct router: %w", err)
	}

	return router, nil
}

type DB struct {
	*database.Batch
	index IndexDB
}

func (c *Client) OpenDB(writable bool) *DB {
	kvb := c.store.Begin(true)
	batch := database.NewBatch("", kvb, true, nil)
	batch.SetObserver(testing.NullObserver{}) // Ignore the BPT
	index := new(indexDB)
	index.key = record.Key{"Light", "Index"}
	index.label = "light index"
	index.store = record.KvStore{Store: kvb}
	return &DB{batch, index}
}

func (db *DB) Index() IndexDB { return db.index }

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
