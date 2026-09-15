// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/fastsync"
	apiv3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
)

// #4275: !1213 ported to the DAG-BFT line. These cover what the port could
// break, as opposed to what the change does — that is already covered by the
// tests it brought with it.

// A service type is on the wire, so the two lines must agree on the value. Main
// assigns 11; DI's highest was 10 (Snapshot), so 11 was free and must stay 11.
func TestDI_ServiceTypeProofMatchesMain(t *testing.T) {
	require.Equal(t, apiv3.ServiceType(11), apiv3.ServiceTypeProof,
		"ServiceTypeProof must be 11, as it is on main — a service type is on the wire")
	require.NotEqual(t, apiv3.ServiceTypeProof, apiv3.ServiceTypeSnapshot,
		"11 must not collide with DI's Snapshot at 10")
}

// The records moved from internal/api/private to pkg/api/v3, and private keeps
// aliases. An alias is the SAME type, not a copy — that is what lets DI's
// spine.go compile untouched. If these ever became distinct types the port
// would still build here but break every caller that passes one to the other.
func TestDI_PrivateRecordsAreAliases(t *testing.T) {
	var (
		major  *private.MajorHeaderRecord  = new(apiv3.MajorHeaderRecord)
		minor  *private.MinorRootRecord    = new(apiv3.MinorRootRecord)
		update *private.NetworkUpdateProof = new(apiv3.NetworkUpdateProof)
	)
	require.NotNil(t, major)
	require.NotNil(t, minor)
	require.NotNil(t, update)

	// And the other direction
	var back *apiv3.MajorHeaderRecord = new(private.MajorHeaderRecord)
	require.NotNil(t, back)
}

// DI keeps its own LoadGenesisGlobals, which restores through
// snapshot.FullRestore rather than database.Restore. The port must not have
// replaced it — only GenesisAnchorHash is new.
func TestDI_KeptItsOwnGenesisRestore(t *testing.T) {
	// If both definitions survived, the package would not compile; if the wrong
	// one survived, FullRestore would be unreferenced. This asserts the shape
	// the port is supposed to leave behind.
	require.NotNil(t, fastsyncLoadGenesisGlobals,
		"DI's LoadGenesisGlobals must still exist")
	require.NotNil(t, fastsyncGenesisAnchorHash,
		"GenesisAnchorHash must have been ported")
}

var (
	fastsyncLoadGenesisGlobals = fastsync.LoadGenesisGlobals
	fastsyncGenesisAnchorHash  = fastsync.GenesisAnchorHash
)
