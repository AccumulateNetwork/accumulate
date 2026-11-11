// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// The v1 executor stores a network definition in the pending globals before
// it rejects one whose version does not increase, and block close makes it
// active. Replaying v1 history needs exactly that, so #4615 leaves v1 as it
// was.
func TestNetworkAccountUpdate_StaleDefinitionStillLandsInPending(t *testing.T) {
	x, err := processNetworkWrite(t, &protocol.NetworkDefinition{Version: 1, NetworkName: "stale"})
	require.ErrorContains(t, err, "version must increase: 1 <= 1")
	require.Equal(t, "stale", x.globals().Pending.Network.NetworkName)

	x, err = processNetworkWrite(t, &protocol.NetworkDefinition{Version: 2})
	require.NoError(t, err)
	require.Equal(t, uint64(2), x.globals().Pending.Network.Version)
}

// processNetworkWrite runs processNetworkAccountUpdates for a WriteData of the
// given network definition to dn.acme/network, on a Directory executor whose
// globals hold definition version 1.
func processNetworkWrite(t *testing.T, def *protocol.NetworkDefinition) (*Executor, error) {
	t.Helper()
	x := new(Executor)
	x.Describe = execute.DescribeShim{NetworkType: protocol.PartitionTypeDirectory, PartitionId: protocol.Directory}
	g := core.NewGlobals(&core.GlobalValues{ExecutorVersion: protocol.ExecutorVersionV1SignatureAnchoring, Network: &protocol.NetworkDefinition{Version: 1}, Routing: new(protocol.RoutingTable)})
	x.globalsPtr.Store(&Globals{Active: *g, Pending: *g.Copy()})

	values := core.GlobalValues{Network: def}
	txn := new(protocol.Transaction)
	txn.Header.Principal = protocol.DnUrl().JoinPath(protocol.Network)
	txn.Body = &protocol.WriteData{Entry: values.FormatNetwork(), WriteToState: true}
	principal := &protocol.DataAccount{Url: txn.Header.Principal}

	batch := database.OpenInMemory(nil).Begin(true)
	t.Cleanup(batch.Discard)
	require.NoError(t, batch.Account(x.Describe.Ledger()).Main().Put(&protocol.SystemLedger{Url: x.Describe.Ledger()}))
	return x, x.processNetworkAccountUpdates(batch, &chain.Delivery{Transaction: txn}, principal)
}
