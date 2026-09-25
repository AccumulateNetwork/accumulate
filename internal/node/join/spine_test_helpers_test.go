// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"

	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/anchorsrc"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// genesisValues is what genesis writes: one definition naming the Directory
// and one BVN, with n nodes that validate both — a container runs a DN node
// and a BVN node.
func genesisValues(t *testing.T, n int) (*core.GlobalValues, []ed25519.PrivateKey) {
	t.Helper()
	def := new(protocol.NetworkDefinition)
	def.NetworkName = "join-test"
	def.Version = 1
	def.AddPartition(protocol.Directory, protocol.PartitionTypeDirectory)
	def.AddPartition("BVN0", protocol.PartitionTypeBlockValidator)

	var keys []ed25519.PrivateKey
	for i := 0; i < n; i++ {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		keys = append(keys, priv)
		def.AddValidator(priv[32:], protocol.Directory, true)
		def.AddValidator(priv[32:], "BVN0", true)
	}

	return &core.GlobalValues{
		Network: def,
		Globals: &protocol.NetworkGlobals{
			ValidatorAcceptThreshold: protocol.Rational{Numerator: 2, Denominator: 3},
			OperatorAcceptThreshold:  protocol.Rational{Numerator: 2, Denominator: 3},
		},
	}, keys
}

// putNetwork writes the network accounts a node holds from its own genesis or
// its own execution. The join reads them before it pulls anything: they are
// what says who may sign an anchor (#4301).
func putNetwork(t *testing.T, db *database.Database, partition *url.URL, values *core.GlobalValues) {
	t.Helper()
	batch := db.Begin(true)
	defer batch.Discard()
	put := func(name string, entry protocol.DataEntry) {
		u := partition.JoinPath(name)
		require.NoError(t, batch.Account(u).Main().Put(&protocol.DataAccount{Url: u, Entry: entry}))
	}
	put(protocol.Network, values.FormatNetwork())
	put(protocol.Globals, values.FormatGlobals())
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())
}

// noAnchorSource is an anchor source that verifies no anchor: every block a
// peer serves at is one the join has no root for.
func noAnchorSource(*testing.T, *url.URL, *core.GlobalValues) anchorSource { return noAnchorsRead{} }

type noAnchorsRead struct{}

func (noAnchorsRead) Read(context.Context) error       { return nil }
func (noAnchorsRead) Stalled() (anchorsrc.Stall, bool) { return anchorsrc.Stall{}, false }
func (noAnchorsRead) Rewind()                          {}
