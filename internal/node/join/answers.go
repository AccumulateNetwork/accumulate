// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// answers keeps what the peers answered for one account's pull, by peer and
// question, so the pull that writes it asks nothing the parallel fetch asked
// already (pullAll). An answer as of a block does not change, so keeping it
// for the length of one pull is exact.
type answers struct {
	mu   sync.Mutex
	kept map[string]answer
}

type answer struct {
	value any
	err   error
}

func newAnswers() *answers { return &answers{kept: map[string]answer{}} }

func ask[T any](a *answers, key string, fn func() (T, error)) (T, error) {
	a.mu.Lock()
	if v, ok := a.kept[key]; ok {
		a.mu.Unlock()
		t, _ := v.value.(T)
		return t, v.err
	}
	a.mu.Unlock()
	t, err := fn()
	if err == nil {
		a.mu.Lock()
		a.kept[key] = answer{t, nil}
		a.mu.Unlock()
	}
	return t, err
}

// cachedSource is one peer, whose answers are kept in answers.
type cachedSource struct {
	pull.Source
	answers *answers
}

func (c cachedSource) key(method string, u fmt.Stringer, q any) string {
	b, _ := json.Marshal(q)
	return fmt.Sprintf("%s|%s|%v|%s", sourceKey(c.Source), method, u, b)
}

func (c cachedSource) String() string { return sourceKey(c.Source) }

func (c cachedSource) QueryAccount(ctx context.Context, u *url.URL, q *api.DefaultQuery) (*api.AccountRecord, error) {
	return ask(c.answers, c.key("account", u, q), func() (*api.AccountRecord, error) { return c.Source.QueryAccount(ctx, u, q) })
}

func (c cachedSource) QueryDirectoryUrls(ctx context.Context, u *url.URL, q *api.DirectoryQuery) (*api.RecordRange[*api.UrlRecord], error) {
	return ask(c.answers, c.key("directory", u, q), func() (*api.RecordRange[*api.UrlRecord], error) { return c.Source.QueryDirectoryUrls(ctx, u, q) })
}

func (c cachedSource) QueryPendingIds(ctx context.Context, u *url.URL, q *api.PendingQuery) (*api.RecordRange[*api.TxIDRecord], error) {
	return ask(c.answers, c.key("pending", u, q), func() (*api.RecordRange[*api.TxIDRecord], error) { return c.Source.QueryPendingIds(ctx, u, q) })
}

func (c cachedSource) QueryAccountChains(ctx context.Context, u *url.URL, q *api.ChainQuery) (*api.RecordRange[*api.ChainRecord], error) {
	return ask(c.answers, c.key("chains", u, q), func() (*api.RecordRange[*api.ChainRecord], error) { return c.Source.QueryAccountChains(ctx, u, q) })
}

func (c cachedSource) QueryChainEntries(ctx context.Context, u *url.URL, q *api.ChainQuery) (*api.RecordRange[*api.ChainEntryRecord[api.Record]], error) {
	return ask(c.answers, c.key("entries", u, q), func() (*api.RecordRange[*api.ChainEntryRecord[api.Record]], error) {
		return c.Source.QueryChainEntries(ctx, u, q)
	})
}

func (c cachedSource) QueryMessage(ctx context.Context, id *url.TxID, q *api.DefaultQuery) (*api.MessageRecord[messaging.Message], error) {
	return ask(c.answers, c.key("message", id, q), func() (*api.MessageRecord[messaging.Message], error) { return c.Source.QueryMessage(ctx, id, q) })
}
