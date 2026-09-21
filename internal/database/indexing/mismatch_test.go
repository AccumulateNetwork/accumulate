// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// TestDebugMismatch covers the build-tagged halves of debugMismatch directly.
//
// The proof-level test for a mismatched receipt can only exercise the
// production half: under -tags debug the observer collapses the state
// components, so no receipt is retained, no mismatch can occur, and the loud
// path is never reached. Calling it here is the only way the debug half is
// tested at all.
func TestDebugMismatch(t *testing.T) {
	anchor := []byte{0x01, 0x02}
	start := []byte{0x03, 0x04}

	err := debugMismatch(anchor, start)
	if debugBuild {
		require.Error(t, err, "a mismatch must be loud under -tags debug")
		require.Equal(t, errors.InternalError, errors.Code(err))
		require.Contains(t, err.Error(), "retained state receipt")
		return
	}
	require.NoError(t, err, "a production node degrades rather than failing")
}
