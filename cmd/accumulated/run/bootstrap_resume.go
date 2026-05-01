// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Bootstrap-v3 resume hook for `accumulated run`.
//
// `accumulated bootstrap` drives a fresh node to ACTIVE atomically:
// fetch a per-partition snapshot, restore it, verify via CometBFT
// header.app_hash, persist state, exit. There is no in-flight phase
// for the daemon to resume — either the launcher reached ACTIVE/WAITING
// or it didn't.
//
// This file remains as the integration point for the deferred
// daemon-side WAITING → ACTIVE verify loop (#107) and persistent-state
// startup gate (#111). Until those land, ResumeIfBooting only enforces
// one invariant: a stale BOOTING marker means the previous bootstrap
// crashed before applying the snapshot, so the data dir is not
// trustworthy and the user must re-run `accumulated bootstrap`.
package run

import (
	"context"
	"errors"
	"fmt"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/bootpersist"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
)

// ResumeIfBooting reads the bootstrap-v3 persisted state from dataDir.
//
//   - No persisted state → nil (legacy node, not bootstrap-v3).
//   - WAITING / ACTIVE / COMPLETE → nil (daemon proceeds; the
//     deferred verify loop in #107 will pick up WAITING and promote).
//   - BOOTING → error. The previous bootstrap crashed before the
//     snapshot was applied; the local DB is incomplete or absent and
//     the only safe path is to re-run `accumulated bootstrap`.
func ResumeIfBooting(_ context.Context, dataDir string, _ *database.Database) error {
	art, err := bootpersist.Load(dataDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load bootstrap state: %w", err)
	}

	state, err := nodestate.ParseState(art.State.Current)
	if err != nil {
		return fmt.Errorf("parse persisted state %q: %w", art.State.Current, err)
	}
	if state != nodestate.StateBooting {
		return nil
	}

	return fmt.Errorf("bootstrap-state.json reports BOOTING — previous `accumulated bootstrap` did not complete; re-run it before starting `accumulated run`")
}
