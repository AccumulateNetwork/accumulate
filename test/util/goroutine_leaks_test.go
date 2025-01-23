// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testutil

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTrackGoroutines(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	err := track(ctx, t.Name(), func(ctx context.Context) {
		// Leak a goroutine
		go func() { <-ctx.Done() }()
	})

	// Verify the leak is detected
	require.Error(t, err)
	require.IsType(t, (*LeakedGoroutines)(nil), err)

	// Release the goroutine
	cancel()
}
