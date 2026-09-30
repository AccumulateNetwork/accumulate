// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
)

func TestServiceAddressJSON(t *testing.T) {
	a1 := private.ServiceTypeSequencer.AddressFor("foo")
	b, err := json.Marshal(a1)
	require.NoError(t, err)

	var a2 *api.ServiceAddress
	require.NoError(t, json.Unmarshal(b, &a2))

	require.True(t, a1.Equal(a2))
}

// Main's nodes still advertise the snapshot service (10), which this line
// removed (#3921, DIFFERENCES.md). Its binary form still decodes, by value; its
// JSON form, by name, does not. This pins the break Paul accepted, so a change
// in either direction is noticed.
func TestMainSnapshotServiceAddress(t *testing.T) {
	var a api.ServiceAddress
	require.Error(t, json.Unmarshal([]byte(`{"type":"snapshot","argument":"bvn1"}`), &a))

	b, err := (&api.ServiceAddress{Type: 10, Argument: "bvn1"}).MarshalBinary()
	require.NoError(t, err)
	require.NoError(t, a.UnmarshalBinary(b))
	require.Equal(t, api.ServiceType(10), a.Type)
}
