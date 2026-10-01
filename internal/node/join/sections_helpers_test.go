// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// loadSectionOf has the pull load the sections the accounts are in, as it
// does when a record names an account there (executor spec, "Sync", §3a): a
// round to take the top of the tree, if none has, then the sections opened,
// and a round that loads them. A pull loads nothing nobody names; a test of
// how the pull takes an account names it this way.
func loadSectionOf(t *testing.T, s *PulledState, urls ...*url.URL) {
	t.Helper()
	ctx := context.Background()
	if s.sync == nil || !s.sync.topTaken {
		require.NoError(t, s.Pull(ctx))
	}
	require.NotNil(t, s.sync, "the pull did not start")
	require.True(t, s.sync.topTaken, "the pull did not take the top of the tree")
	for _, u := range urls {
		sec := database.SectionOfAccount(u)
		s.openSection(s.sync, sec)
		if _, ok := s.sync.opened[sec]; !ok {
			// Loaded already: load it again, as a record naming the
			// account would have it brought current.
			s.sync.opened[sec] = false
		}
	}
	require.NoError(t, s.Pull(ctx))
}
