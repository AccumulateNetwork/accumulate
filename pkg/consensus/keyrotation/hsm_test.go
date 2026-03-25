// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package keyrotation

import (
	"testing"
)

func TestNewHSMProvider(t *testing.T) {
	tests := []struct {
		name      string
		config    HSMConfig
		wantErr   bool
		expectNil bool
	}{
		{
			name: "no provider",
			config: HSMConfig{
				Provider: "none",
			},
			wantErr:   false,
			expectNil: true,
		},
		{
			name: "empty provider",
			config: HSMConfig{
				Provider: "",
			},
			wantErr:   false,
			expectNil: true,
		},
		{
			name: "aws provider not implemented",
			config: HSMConfig{
				Provider: "aws-cloudhsm",
			},
			wantErr:   true,
			expectNil: false,
		},
		{
			name: "gcp provider not implemented",
			config: HSMConfig{
				Provider: "google-cloud-kms",
			},
			wantErr:   true,
			expectNil: false,
		},
		{
			name: "vault provider not implemented",
			config: HSMConfig{
				Provider: "vault",
			},
			wantErr:   true,
			expectNil: false,
		},
		{
			name: "unknown provider",
			config: HSMConfig{
				Provider: "unknown-hsm",
			},
			wantErr:   true,
			expectNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewHSMProvider(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewHSMProvider() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.expectNil && provider != nil {
				t.Error("Expected nil provider")
			}
		})
	}
}
