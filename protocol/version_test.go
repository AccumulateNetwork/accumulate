// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"testing"
)

// TestLXRMiningVersionGuard verifies that the V2LXRMiningEnabled check works correctly
func TestLXRMiningVersionGuard(t *testing.T) {
	tests := []struct {
		name    string
		version ExecutorVersion
		want    bool
	}{
		{
			name:    "V1 does not enable LXR mining",
			version: ExecutorVersionV1,
			want:    false,
		},
		{
			name:    "V2 does not enable LXR mining",
			version: ExecutorVersionV2,
			want:    false,
		},
		{
			name:    "V2Baikonur does not enable LXR mining",
			version: ExecutorVersionV2Baikonur,
			want:    false,
		},
		{
			name:    "V2Vandenberg does not enable LXR mining",
			version: ExecutorVersionV2Vandenberg,
			want:    false,
		},
		{
			name:    "V2Jiuquan does not enable LXR mining",
			version: ExecutorVersionV2Jiuquan,
			want:    false,
		},
		{
			name:    "V2LXRMining enables LXR mining",
			version: ExecutorVersionV2LXRMining,
			want:    true,
		},
		{
			name:    "VNext enables LXR mining (forward compatibility)",
			version: ExecutorVersionVNext,
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.version.V2LXRMiningEnabled()
			if got != tt.want {
				t.Errorf("V2LXRMiningEnabled() = %v, want %v for version %v", got, tt.want, tt.version)
			}
		})
	}
}

// TestVersionProgression verifies the ordering of version constants
func TestVersionProgression(t *testing.T) {
	if ExecutorVersionV2Jiuquan >= ExecutorVersionV2LXRMining {
		t.Errorf("V2Jiuquan (%d) should be less than V2LXRMining (%d)",
			ExecutorVersionV2Jiuquan, ExecutorVersionV2LXRMining)
	}

	if ExecutorVersionV2LXRMining >= ExecutorVersionVNext {
		t.Errorf("V2LXRMining (%d) should be less than VNext (%d)",
			ExecutorVersionV2LXRMining, ExecutorVersionVNext)
	}

	expectedLXRVersion := ExecutorVersion(9)
	if ExecutorVersionV2LXRMining != expectedLXRVersion {
		t.Errorf("V2LXRMining = %d, want %d", ExecutorVersionV2LXRMining, expectedLXRVersion)
	}
}
