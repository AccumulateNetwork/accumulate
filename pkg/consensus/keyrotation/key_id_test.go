// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package keyrotation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// #4183: two keys minted at the same instant must not share an ID. On
// Windows time.Now() repeats across consecutive calls, so the clock alone
// is not a name.
func TestGenerateKeyID_SameInstantDiffers(t *testing.T) {
	now := time.Now()
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := generateKeyID(now)
		require.False(t, seen[id], "duplicate key ID %s", id)
		seen[id] = true
	}
}
