// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// stagingFixture is a destination executor with an in-memory block, and a
// fake source chain whose entries proofs are cut from.
type stagingFixture struct {
	x      *Executor
	db     *database.Database
	batch  *database.Batch
	b      *Block
	src    [][]byte // entry hashes of the fake source chain
	chain  *database.Chain
	chain2 *database.Chain2
	source *url.URL
}

func newStagingFixture(t *testing.T, entries int) *stagingFixture {
	t.Helper()
	x := new(Executor)
	x.Describe = execute.DescribeShim{NetworkType: protocol.PartitionTypeBlockValidator, PartitionId: "BVN0"}
	x.globalsPtr.Store(&Globals{Active: core.GlobalValues{ExecutorVersion: protocol.ExecutorVersionLatest, Network: &protocol.NetworkDefinition{Version: 1}}})
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)
	chain2 := batch.Account(protocol.PartitionUrl("BVN1").JoinPath(protocol.Synthetic)).MainChain()
	chain, err := chain2.Get()
	require.NoError(t, err)
	f := &stagingFixture{x: x, db: db, batch: batch, chain: chain, chain2: chain2, source: protocol.PartitionUrl("BVN1")}
	for i := 0; i < entries; i++ {
		h := sha256.Sum256([]byte(fmt.Sprintf("synthetic message %d", i)))
		f.src = append(f.src, h[:])
		require.NoError(t, chain.AddEntry(h[:], false))
	}
	f.b = &Block{positions: new(positionCache), Executor: x, Batch: batch, staging: x.staging().Begin()}
	return f
}

func (f *stagingFixture) proof(t *testing.T, start, end int64) *merkle.ReceiptList {
	t.Helper()
	list, err := merkle.GetReceiptList(f.chain2.Inner(), start, end)
	require.NoError(t, err)
	require.True(t, list.Validate(nil), "the fixture's proof must be valid")
	return list
}

func (f *stagingFixture) stream() execute.StreamID { return f.x.synthStream(f.source) }

func (f *stagingFixture) prove(t *testing.T, start, end int64) {
	t.Helper()
	require.NoError(t, f.b.staging.Prove(f.stream(), f.proof(t, start, end)))
}

// isProven reports whether hash is the validated hash at its number: the
// fixture's chain index i is number i+1. A hash not on the fixture's chain is
// checked at every number the fixture spans.
func (f *stagingFixture) isProven(hash []byte) bool {
	var h [32]byte
	copy(h[:], hash)
	for i, src := range f.src {
		if bytes.Equal(src, hash) {
			return f.b.staging.IsValidated(f.stream(), uint64(i)+1, h)
		}
	}
	for n := uint64(1); n <= uint64(len(f.src)); n++ {
		if f.b.staging.IsValidated(f.stream(), n, h) {
			return true
		}
	}
	return false
}
