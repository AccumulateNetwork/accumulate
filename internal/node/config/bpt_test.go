// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBPTValidation(t *testing.T) {
	tests := []struct {
		name      string
		enabled   bool
		depth     int64
		expectErr bool
	}{
		{"disabled, any depth ok", false, 99, false},
		{"enabled, valid depth 1", true, 1, false},
		{"enabled, valid depth 4", true, 4, false},
		{"enabled, valid depth 8", true, 8, false},
		{"enabled, depth too low", true, 0, true},
		{"enabled, depth too high", true, 9, true},
		{"enabled, negative depth", true, -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bpt := &BPT{
				Sharding: BPTSharding{
					Enabled: tt.enabled,
					Depth:   tt.depth,
				},
			}

			err := bpt.Validate()
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
