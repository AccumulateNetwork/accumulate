// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package trustbundle

import (
	"context"
	"errors"
	"testing"
)

func TestCache_EmptyReturnsErrNoBundle(t *testing.T) {
	c := NewCache()
	_, err := c.CurrentBundle(context.Background(), "Directory")
	if !errors.Is(err, ErrNoBundle) {
		t.Fatalf("err = %v, want ErrNoBundle", err)
	}
}

func TestCache_SetGet(t *testing.T) {
	c := NewCache()
	c.Set(&Bundle{Partition: "Directory", MajorBlockIndex: 100})
	c.Set(&Bundle{Partition: "Apollo", MajorBlockIndex: 200})

	dn, err := c.CurrentBundle(context.Background(), "Directory")
	if err != nil {
		t.Fatal(err)
	}
	if dn.MajorBlockIndex != 100 {
		t.Errorf("Directory MajorBlockIndex = %d, want 100", dn.MajorBlockIndex)
	}

	apollo, err := c.CurrentBundle(context.Background(), "Apollo")
	if err != nil {
		t.Fatal(err)
	}
	if apollo.MajorBlockIndex != 200 {
		t.Errorf("Apollo MajorBlockIndex = %d, want 200", apollo.MajorBlockIndex)
	}
}

func TestCache_OverwriteLatest(t *testing.T) {
	c := NewCache()
	c.Set(&Bundle{Partition: "Directory", MajorBlockIndex: 1})
	c.Set(&Bundle{Partition: "Directory", MajorBlockIndex: 2})

	got, err := c.CurrentBundle(context.Background(), "Directory")
	if err != nil {
		t.Fatal(err)
	}
	if got.MajorBlockIndex != 2 {
		t.Errorf("Set should overwrite; MajorBlockIndex = %d, want 2", got.MajorBlockIndex)
	}
}

func TestCache_NilSetIsNoOp(t *testing.T) {
	c := NewCache()
	c.Set(nil) // must not panic
	_, err := c.CurrentBundle(context.Background(), "Directory")
	if !errors.Is(err, ErrNoBundle) {
		t.Errorf("err = %v, want ErrNoBundle", err)
	}
}

// TestCache_ProducerInterface pins that Cache satisfies the Producer
// interface. Locks the implementation against an accidental rename.
func TestCache_ProducerInterface(t *testing.T) {
	var _ Producer = (*Cache)(nil)
}
