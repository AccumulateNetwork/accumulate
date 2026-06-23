// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package anchorsrc

import (
	"context"
	"errors"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/orchestrator"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// nopQuerier returns an empty record range — no anchors available.
type nopQuerier struct{}

func (nopQuerier) Query(_ context.Context, _ *url.URL, _ api.Query) (api.Record, error) {
	return &api.RecordRange[api.Record]{}, nil
}

// errQuerier always errors.
type errQuerier struct{}

func (errQuerier) Query(_ context.Context, _ *url.URL, _ api.Query) (api.Record, error) {
	return nil, errors.New("transport down")
}

// TestNew_RejectsMissingInputs guards.
func TestNew_RejectsMissingInputs(t *testing.T) {
	q := nopQuerier{}
	pool := protocol.DnUrl().JoinPath(protocol.AnchorPool)
	db := database.OpenInMemory(nil)

	if _, err := New(nil, pool, db); err == nil {
		t.Error("expected err for nil querier")
	}
	if _, err := New(q, nil, db); err == nil {
		t.Error("expected err for nil pool")
	}
	if _, err := New(q, pool, nil); err == nil {
		t.Error("expected err for nil db")
	}
}

// TestSatisfiesOrchestrator — compile-time confirmation that *Source
// fits orchestrator.AnchorSource.
func TestSatisfiesOrchestrator(t *testing.T) {
	q := nopQuerier{}
	pool := protocol.DnUrl().JoinPath(protocol.AnchorPool)
	db := database.OpenInMemory(nil)
	s, err := New(q, pool, db)
	if err != nil {
		t.Fatal(err)
	}
	var _ orchestrator.AnchorSource = s
}

// TestLatestAnchor_NoRecords returns zero with no error when the
// peer's main chain is empty.
func TestLatestAnchor_NoRecords(t *testing.T) {
	pool := protocol.DnUrl().JoinPath(protocol.AnchorPool)
	db := database.OpenInMemory(nil)
	s, err := New(nopQuerier{}, pool, db)
	if err != nil {
		t.Fatal(err)
	}
	block, anchor, err := s.LatestAnchor(context.Background(), "Apollo")
	if err != nil {
		t.Fatalf("LatestAnchor: %v", err)
	}
	if block != 0 || anchor != ([32]byte{}) {
		t.Errorf("got (%d, %x), want (0, 0…)", block, anchor[:8])
	}
}

// TestLatestAnchor_TransportError surfaces the transport error.
func TestLatestAnchor_TransportError(t *testing.T) {
	pool := protocol.DnUrl().JoinPath(protocol.AnchorPool)
	db := database.OpenInMemory(nil)
	s, err := New(errQuerier{}, pool, db)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = s.LatestAnchor(context.Background(), "Apollo")
	if err == nil {
		t.Fatal("expected transport error")
	}
}
