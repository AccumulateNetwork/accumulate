// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
)

// A node lists its Directory consensus service first, and other services
// named after the Directory: the BVN is still the non-directory consensus
// service.
func TestBVNOfIgnoresTheDirectoryAndOtherServices(t *testing.T) {
	services := []*v3.ServiceAddress{
		{Type: v3.ServiceTypeConsensus, Argument: "Directory"},
		{Type: v3.ServiceTypeQuery, Argument: "BVN2"},
		{Type: v3.ServiceTypeNetwork, Argument: "Directory"},
		{Type: v3.ServiceTypeConsensus, Argument: "BVN1"},
	}
	id, ok := bvnOf(services)
	require.True(t, ok)
	require.Equal(t, "BVN1", id)

	_, ok = bvnOf(services[:3])
	require.False(t, ok, "a node with only the Directory's consensus service has no BVN")
}
