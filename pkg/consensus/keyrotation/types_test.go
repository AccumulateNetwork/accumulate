// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package keyrotation

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
)

func TestKeyStatusString(t *testing.T) {
	tests := []struct {
		status   KeyStatus
		expected string
	}{
		{KeyStatusPending, "pending"},
		{KeyStatusActive, "active"},
		{KeyStatusGrace, "grace"},
		{KeyStatusExpired, "expired"},
		{KeyStatusRevoked, "revoked"},
		{KeyStatus(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.status.String(); got != tt.expected {
				t.Errorf("KeyStatus.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestEncodePublicKey(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	encoded := EncodePublicKey(pub)
	if encoded == "" {
		t.Error("Encoded public key is empty")
	}

	// Should be hex string (64 characters for 32 bytes)
	if len(encoded) != 64 {
		t.Errorf("Expected 64 character hex string, got %d", len(encoded))
	}
}

func TestValidationError(t *testing.T) {
	err := &ValidationError{
		Field:   "test_field",
		Message: "test message",
	}

	expected := "validation error: test_field: test message"
	if got := err.Error(); got != expected {
		t.Errorf("ValidationError.Error() = %v, want %v", got, expected)
	}
}

func TestRotationType(t *testing.T) {
	tests := []struct {
		rotationType RotationType
		expected     string
	}{
		{RotationTypeAutomatic, "automatic"},
		{RotationTypeManual, "manual"},
		{RotationTypeEmergency, "emergency"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if string(tt.rotationType) != tt.expected {
				t.Errorf("RotationType = %v, want %v", tt.rotationType, tt.expected)
			}
		})
	}
}
