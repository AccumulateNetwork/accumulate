// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNullSigners(t *testing.T) {
	var status *TransactionStatus
	err := json.Unmarshal([]byte(`{"signers": null}`), &status)
	require.NoError(t, err)
}
