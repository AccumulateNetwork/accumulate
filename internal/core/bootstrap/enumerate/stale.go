// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package enumerate

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Stale is Run's stale set on its own: the accounts whose leaf the node does
// not hold or does not agree with (executor.md, "Sync", step 3: every account
// behind a leaf that is missing or stale).
//
// Nothing is written. Stale names accounts; pull.Account fetches and verifies
// them, and writing the verified state is what puts a leaf in the local BPT.
func Stale(ctx context.Context, src Source, scope *url.URL, batch *database.Batch, opts Options) ([]*url.URL, error) {
	res, err := Run(ctx, src, scope, batch, opts)
	if res == nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return res.Stale, errors.UnknownError.Wrap(err)
}
