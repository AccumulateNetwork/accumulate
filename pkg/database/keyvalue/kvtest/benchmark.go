// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package kvtest

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

func BenchmarkCommit(b *testing.B, open Opener) {
	// Populate
	db := openDb(b, open)

	batch := db.Begin(nil, true)
	defer batch.Discard()

	for i := 0; i < b.N; i++ {
		err := batch.Put(record.NewKey("answer", i), []byte(fmt.Sprintf("%x this much data ", i)))
		require.NoError(b, err, "Put")
	}

	// Commit
	b.ResetTimer()
	require.NoError(b, batch.Commit())
}

func BenchmarkOpen(b *testing.B, open Opener) {
	// Populate
	db := openDb(b, open)

	batch := db.Begin(nil, true)
	defer batch.Discard()

	for i := 0; i < b.N; i++ {
		err := batch.Put(record.NewKey("answer", i), []byte(fmt.Sprintf("%x this much data ", i)))
		require.NoError(b, err, "Put")
	}
	require.NoError(b, batch.Commit())
	db.Close()

	// Open
	b.ResetTimer()
	db = openDb(b, open)
	db.Close()
}

func BenchmarkReadRandom(b *testing.B, open Opener) {
	const N = 1000000

	// Populate
	db := openDb(b, open)

	batch := db.Begin(nil, true)
	defer batch.Discard()

	keys := make([]*record.Key, N)
	for i := range keys {
		keys[i] = record.NewKey("answer", i)
		err := batch.Put(keys[i], []byte(fmt.Sprintf("%x this much data ", i)))
		require.NoError(b, err, "Put")
	}

	// Commit and create a new batch
	require.NoError(b, batch.Commit())
	batch = db.Begin(nil, false)
	defer batch.Discard()

	r := rand.New(rand.NewSource(0))
	indices := make([]int, b.N)
	for i := range indices {
		indices[i] = r.Intn(N)
	}

	// Read
	b.ResetTimer()
	for _, i := range indices {
		_, err := batch.Get(keys[i])
		if err != nil {
			require.NoError(b, err)
		}
	}
}
