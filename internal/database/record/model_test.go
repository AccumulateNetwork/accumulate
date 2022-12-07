// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package record_test

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-model --package record_test --out model_gen_test.go model_test.yml

// const markPower = 8

func TestRecords(t *testing.T) {
	store := memory.New(nil).Begin(true)
	cs := new(changeSet)
	cs.store = record.KvStore{store}

	txn1 := new(protocol.Transaction)
	txn1.Header.Principal = url.MustParse("foo/1")
	txn1.Body = new(protocol.CreateIdentity)

	txn2 := new(protocol.Transaction)
	txn2.Header.Principal = url.MustParse("foo/2")
	txn2.Body = new(protocol.CreateIdentity)

	// Write stuff
	require.NoError(t, cs.Entity("foo").Union().Put(&protocol.LiteTokenAccount{Url: url.MustParse("foo")}))
	require.NoError(t, cs.Entity("bar").Set().Add(txn1.ID()))
	require.NoError(t, cs.Entity("bar").Set().Add(txn1.ID()))
	require.NoError(t, cs.Entity("bar").Set().Add(txn2.ID()))
	// require.NoError(t, cs.Entity("baz").Chain().AddHash(txn1.GetHash(), true))
	// require.NoError(t, cs.Entity("baz").Chain().AddHash(txn2.GetHash(), true))
	require.NoError(t, cs.Commit())

	// Verify
	cs = new(changeSet)
	cs.store = record.KvStore{store}

	// Verify the URL of the union
	account, err := cs.Entity("foo").Union().Get()
	require.NoError(t, err)
	require.Equal(t, "foo", account.GetUrl().ShortString())

	// Verify the size of the set
	set, err := cs.Entity("bar").Set().Get()
	require.NoError(t, err)
	require.Len(t, set, 2)

	// Verify the state of the chain
	// chain, err := cs.Entity("baz").Chain().Head().Get()
	// require.NoError(t, err)
	// require.Equal(t, 2, int(chain.Count))

	// Verify the changelog was updated
	cl, err := cs.ChangeLog().GetAll()
	require.NoError(t, err)
	require.ElementsMatch(t, []string{
		"Entity.foo",
		"Entity.bar",
		// "Entity.baz",
	}, cl)
}

func (e *entity) Commit() error {
	if !e.IsDirty() {
		return nil
	}

	// Log the update
	err := e.parent.ChangeLog().Put(e.key.String())
	if err != nil {
		return err
	}

	// Do the normal commit thing
	return e.baseCommit()
}

type StructType struct{}

func (*StructType) Compare(*StructType) int             { return 0 }
func (*StructType) CopyAsInterface() interface{}        { return nil }
func (*StructType) MarshalBinary() ([]byte, error)      { return nil, nil }
func (*StructType) UnmarshalBinary(data []byte) error   { return nil }
func (*StructType) UnmarshalBinaryFrom(io.Reader) error { return nil }

type UnionType interface {
	encoding.BinaryValue
	Compare(UnionType) int
}

func UnmarshalUnionType([]byte) (UnionType, error) { return nil, nil }
