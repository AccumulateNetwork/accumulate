// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestMachineJSON(t *testing.T) {
	acctesting.EnableDebugFeatures()

	x := errors.NotFound.With("I couldn't find the thing")
	x = errors.UnknownError.WithFormat("error doing thing: %w", x)

	b, err := json.Marshal(x)
	require.NoError(t, err)

	y := new(errors.Error)
	require.NoError(t, json.Unmarshal(b, y))

	require.True(t, x.Equal(y))
}
