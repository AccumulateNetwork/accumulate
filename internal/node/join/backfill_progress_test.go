// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// TestALongBackfillSaysHowFarItHasGot — #4472. One account of the Directory
// is minutes of pages, and on Docker three followers said nothing of it for
// ten minutes. An account's backfill says nothing in its first minute, then
// names the account, the chain and the entries written of the chain's
// height, at most once a minute.
func TestALongBackfillSaysHowFarItHasGot(t *testing.T) {
	var buf bytes.Buffer
	clock := time.Unix(1_000_000, 0)
	s := &PulledState{
		partition: url.MustParse("dn.acme"),
		log:       slog.New(slog.NewTextHandler(&buf, nil)),
		now:       func() time.Time { return clock },
	}
	const line = "Backfilling a large account"
	lines := func() int { return strings.Count(buf.String(), line) }

	progress := s.backfillProgress(url.MustParse("dn.acme/anchors"))
	page := uint64(0)
	next := func(after time.Duration) {
		clock = clock.Add(after)
		page += 256
		progress("anchor(directory)-bpt", page, 40_000)
	}

	next(10 * time.Second)
	next(40 * time.Second)
	require.Zero(t, lines(), "an account that has taken less than a minute says nothing")

	next(11 * time.Second)
	require.Equal(t, 1, lines(), "an account that has taken a minute says how far it has got")
	require.Contains(t, buf.String(), "account=acc://dn.acme/anchors")
	require.Contains(t, buf.String(), "chain=anchor(directory)-bpt")
	require.Contains(t, buf.String(), "entries=768 of=40000")

	for i := 0; i < 5; i++ {
		next(10 * time.Second)
	}
	require.Equal(t, 1, lines(), "said again within the minute")

	next(11 * time.Second)
	require.Equal(t, 2, lines(), "not said again a minute later")
}
