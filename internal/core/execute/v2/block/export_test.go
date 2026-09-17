// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/synthcache"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// NewSeedProbe is a fresh executor over a node's store — what a restarted
// node's executor is before its first block — for tests that ask what the
// cache seed holds (#4241). It reads and never sends.
func NewSeedProbe(desc execute.DescribeShim, db *database.Database, cache *synthcache.Cache) (*Executor, error) {
	x := new(Executor)
	x.Describe = desc
	x.Database = db
	x.SynthCache = cache
	g := new(Globals)
	err := g.Active.Load(desc.NodeUrl(), func(account *url.URL, target any) error {
		return db.View(func(b *database.Batch) error { return b.Account(account).Main().GetAs(target) })
	})
	x.globalsPtr.Store(g)
	return x, err
}

// SeedSynthCache runs the start-up seed as a follower would: nothing is sent.
func (x *Executor) SeedSynthCache(batch *database.Batch, current uint64) error {
	return x.seedSynthCache(batch, current, false)
}
