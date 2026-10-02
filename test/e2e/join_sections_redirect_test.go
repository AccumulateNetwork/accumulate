// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
)

// countingSource counts the section reads a node asks of a peer.
type countingSource struct {
	pull.Source
	pages, states *atomic.Int64
}

func (c countingSource) QueryBptPage(ctx context.Context, scope *url.URL, q *api.BptPageQuery) (*api.BptPageRecord, error) {
	c.pages.Add(1)
	return c.Source.QueryBptPage(ctx, scope, q)
}

func (c countingSource) QueryAccountStates(ctx context.Context, scope *url.URL, q *api.AccountStatesQuery) (*api.AccountStatesRecord, error) {
	c.states.Add(1)
	return c.Source.QueryAccountStates(ctx, scope, q)
}

// refusal is the first error a validation reached, in its error or its
// statuses.
func refusal(statuses []*protocol.TransactionStatus, err error) error {
	if err != nil {
		return err
	}
	for _, st := range statuses {
		if st.Error != nil {
			return st.Error
		}
	}
	return nil
}

// TestAValidationThatNeedsASectionNotLoadedIsRefusedAndLoadsNothing — #4485,
// Paul 2026-10-02 (DIFFERENCES E25): validation does not load. dave's key
// page is in a section the joiner has not loaded. The joiner's executor,
// asked to validate a transaction dave signs, refuses it NotReady -- which
// the submit path redirects -- and asks no peer for the section; a validator,
// which holds every section, validates it.
func TestAValidationThatNeedsASectionNotLoadedIsRefusedAndLoadsNothing(t *testing.T) {
	j := prepareSectionsJoin(t)
	var pages, states atomic.Int64
	j.part.SetSourceWrap(j.fresh, func(srcs []pull.Source) []pull.Source {
		out := make([]pull.Source, len(srcs))
		for i, s := range srcs {
			out[i] = countingSource{Source: s, pages: &pages, states: &states}
		}
		return out
	})
	j.run()
	page := j.dave.JoinPath("book", "1")
	require.True(t, j.notLoaded(page), "precondition: dave's key page is in a section the node has loaded")

	env := MustBuild(t, build.Transaction().For(j.dave, "tokens").
		SendTokens(3, 0).To(j.bob, "tokens").
		SignWith(j.dave, "book", "1").Version(1).Timestamp(1).PrivateKey(j.daveKey))

	p0, s0 := pages.Load(), states.Load()
	statuses, verr := j.part.NodeExecutor(j.fresh).Validate(env, false)
	err := refusal(statuses, verr)
	t.Logf("validate: error %v; refusal %v", verr, err)
	require.Error(t, err, "the joiner validated a transaction whose signer is in a section it has not loaded")
	require.Equal(t, errors.NotReady, errors.Code(fmt.Errorf("validate: %w", err)), "the refusal is not NotReady, so the submit path would not redirect it: %v", err)
	require.Equal(t, p0, pages.Load(), "a validation asked a peer for a section's pages")
	require.Equal(t, s0, states.Load(), "a validation asked a peer for a section's accounts")
	require.True(t, j.notLoaded(page))

	require.NoError(t, refusal(j.part.NodeExecutor(0).Validate(env, false)), "a validator holding every section refused it")
}
