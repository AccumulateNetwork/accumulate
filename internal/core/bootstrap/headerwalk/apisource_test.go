// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package headerwalk

import (
	"context"
	"errors"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Compile-time check that APISource still satisfies HeaderSource
// after the rewrite.
var _ HeaderSource = (*APISource)(nil)

func TestAPISource_Constructor(t *testing.T) {
	bvn := protocol.PartitionUrl("Apollo").JoinPath(protocol.AnchorPool)
	src := NewAPISource(api.Querier2{}, bvn)
	if src == nil {
		t.Fatal("NewAPISource returned nil")
	}
	if src.bvnAnchorPool != bvn {
		t.Errorf("bvnAnchorPool not stored")
	}
	if src.PageSize != 256 {
		t.Errorf("default PageSize = %d, want 256", src.PageSize)
	}
}

func TestAPISource_SetOperatorsPage(t *testing.T) {
	bvn := protocol.PartitionUrl("Apollo").JoinPath(protocol.AnchorPool)
	src := NewAPISource(api.Querier2{}, bvn)

	op := protocol.DnUrl().JoinPath(protocol.Operators, "1")
	src.SetOperatorsPage(op)
	src.mu.Lock()
	got := src.dnOperatorsPage
	src.mu.Unlock()
	if got != op {
		t.Errorf("dnOperatorsPage not stored")
	}
}

// TestAPISource_OperatorsDeltaAt_NoOperatorsPage ensures the stub
// returns nil deltas with no error when SetOperatorsPage hasn't been
// called. Used by the keybookat-aware walker as the no-rotation hot
// path.
func TestAPISource_OperatorsDeltaAt_NoOperatorsPage(t *testing.T) {
	bvn := protocol.PartitionUrl("Apollo").JoinPath(protocol.AnchorPool)
	src := NewAPISource(api.Querier2{}, bvn)

	deltas, err := src.OperatorsDeltaAt(context.Background(), 1)
	if err != nil {
		t.Fatalf("OperatorsDeltaAt: %v", err)
	}
	if deltas != nil {
		t.Errorf("expected nil deltas without operators page, got %+v", deltas)
	}
}

// TestAPISource_OperatorsDeltaAt_StubReturnsNilWithPageSet
// documents the steady-state path: even with operators page
// configured, the source returns nil because no rotations have
// happened on mainnet. When the wiring lands (TODO in the source),
// this test should be replaced with one that fakes UpdateKeyPage
// txns in the right minor-block range and asserts deltas come back.
func TestAPISource_OperatorsDeltaAt_StubReturnsNilWithPageSet(t *testing.T) {
	bvn := protocol.PartitionUrl("Apollo").JoinPath(protocol.AnchorPool)
	src := NewAPISource(api.Querier2{}, bvn)
	src.SetOperatorsPage(protocol.DnUrl().JoinPath(protocol.Operators, "1"))

	deltas, err := src.OperatorsDeltaAt(context.Background(), 5)
	if err != nil {
		t.Fatalf("OperatorsDeltaAt: %v", err)
	}
	if deltas != nil {
		t.Errorf("steady-state stub should return nil; got %+v", deltas)
	}
}

// TestAPISource_HeaderRejectsZeroMajorBlock pins the genesis-numbering
// invariant: major-block 0 doesn't exist, so Header(0) returns
// ErrNoSuchHeight. The walker terminates against major-block 1.
func TestAPISource_HeaderRejectsZeroMajorBlock(t *testing.T) {
	bvn := protocol.PartitionUrl("Apollo").JoinPath(protocol.AnchorPool)
	src := NewAPISource(api.Querier2{Querier: &errOnlyQuerier{}}, bvn)

	_, err := src.Header(context.Background(), 0)
	if err == nil {
		t.Fatal("expected error on Header(0)")
	}
	if !errors.Is(err, ErrNoSuchHeight) {
		t.Errorf("err = %v, want ErrNoSuchHeight chain", err)
	}
}

// errOnlyQuerier is the simplest possible fake — every Query call
// returns an error. Used by tests that exercise pre-query error
// paths (input validation, etc.).
type errOnlyQuerier struct{}

func (e *errOnlyQuerier) Query(_ context.Context, _ *url.URL, _ api.Query) (api.Record, error) {
	return nil, errors.New("errOnlyQuerier: not supported by this test")
}
