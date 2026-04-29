// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package backfill

import (
	"context"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
)

// TestRun_RejectsMissingInputs guards.
func TestRun_RejectsMissingInputs(t *testing.T) {
	db := database.OpenInMemory(nil)
	m := nodestate.New()

	if _, err := Run(context.Background(), nil, db, m, Options{}); err == nil {
		t.Error("expected err for nil src")
	}
	if _, err := Run(context.Background(), nopSrc{}, nil, m, Options{}); err == nil {
		t.Error("expected err for nil db")
	}
	if _, err := Run(context.Background(), nopSrc{}, db, nil, Options{}); err == nil {
		t.Error("expected err for nil machine")
	}
}

// TestRun_RequiresActive — backfill only runs once the node is past
// BOOTING. A BOOTING machine returns an error.
func TestRun_RequiresActive(t *testing.T) {
	db := database.OpenInMemory(nil)
	m := nodestate.New() // BOOTING

	_, err := Run(context.Background(), nopSrc{}, db, m, Options{})
	if err == nil {
		t.Fatal("expected err for BOOTING machine")
	}
}

// TestRun_EmptyDB — no accounts to walk; machine promotes to COMPLETE.
func TestRun_EmptyDB(t *testing.T) {
	db := database.OpenInMemory(nil)
	m, err := nodestate.Restore(nodestate.StateActive, 100, [32]byte{0x01}, 0)
	if err != nil {
		t.Fatal(err)
	}

	res, err := Run(context.Background(), nopSrc{}, db, m, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Walked != 0 {
		t.Errorf("Walked=%d, want 0", res.Walked)
	}
	if m.State() != nodestate.StateComplete {
		t.Errorf("state=%v, want COMPLETE", m.State())
	}
}
