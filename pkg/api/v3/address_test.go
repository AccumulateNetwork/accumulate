// Copyright 2023 The Accumulate Authors
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
