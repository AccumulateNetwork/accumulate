// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package keyrotation

import (
	"crypto/ed25519"
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestManager_InitialKeyGeneration(t *testing.T) {
	config := &Config{
		Enabled:              true,
		RotationIntervalDays: 90,
		GracePeriodDays:      7,
		WarningPeriodDays:    7,
		Audit: AuditConfig{
			Enabled: false,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	manager, err := NewManager(config, logger, "test-validator")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Stop()

	if err := manager.Start(); err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}

	activeKey := manager.GetActiveKey()
	if activeKey == nil {
		t.Fatal("Expected active key to be generated")
	}

	if activeKey.Status != KeyStatusActive {
		t.Errorf("Expected key status to be Active, got %s", activeKey.Status)
	}

	if len(activeKey.PublicKey) != ed25519.PublicKeySize {
		t.Errorf("Expected public key size %d, got %d", ed25519.PublicKeySize, len(activeKey.PublicKey))
	}
}

func TestManager_KeyValidation(t *testing.T) {
	config := &Config{
		Enabled:              true,
		RotationIntervalDays: 90,
		GracePeriodDays:      7,
		WarningPeriodDays:    7,
		Audit: AuditConfig{
			Enabled: false,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	manager, err := NewManager(config, logger, "test-validator")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Stop()

	if err := manager.Start(); err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}

	activeKey := manager.GetActiveKey()

	// Active key should be valid
	if !manager.IsValidKey(activeKey.PublicKey) {
		t.Error("Active key should be valid")
	}

	// Random key should not be valid
	_, randomKey, _ := ed25519.GenerateKey(nil)
	if manager.IsValidKey(randomKey.Public().(ed25519.PublicKey)) {
		t.Error("Random key should not be valid")
	}
}

func TestManager_ManualRotation(t *testing.T) {
	config := &Config{
		Enabled:              true,
		RotationIntervalDays: 90,
		GracePeriodDays:      7,
		WarningPeriodDays:    7,
		Audit: AuditConfig{
			Enabled: false,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	manager, err := NewManager(config, logger, "test-validator")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Stop()

	if err := manager.Start(); err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}

	originalKey := manager.GetActiveKey()

	// Perform manual rotation
	if err := manager.RotateKey("operator", "manual rotation test"); err != nil {
		t.Fatalf("Failed to rotate key: %v", err)
	}

	newKey := manager.GetActiveKey()

	// Should have a new active key
	if newKey.KeyID == originalKey.KeyID {
		t.Error("Expected new active key after rotation")
	}

	// New key should be active
	if newKey.Status != KeyStatusActive {
		t.Errorf("Expected new key status to be Active, got %s", newKey.Status)
	}

	// Both keys should be valid during grace period
	if !manager.IsValidKey(originalKey.PublicKey) {
		t.Error("Original key should still be valid during grace period")
	}

	if !manager.IsValidKey(newKey.PublicKey) {
		t.Error("New key should be valid")
	}
}

func TestManager_EmergencyRevocation(t *testing.T) {
	config := &Config{
		Enabled:              true,
		RotationIntervalDays: 90,
		GracePeriodDays:      7,
		WarningPeriodDays:    7,
		Audit: AuditConfig{
			Enabled: false,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	manager, err := NewManager(config, logger, "test-validator")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Stop()

	if err := manager.Start(); err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}

	originalKey := manager.GetActiveKey()

	// Perform emergency revocation
	if err := manager.RevokeKey("operator", "key compromise detected"); err != nil {
		t.Fatalf("Failed to revoke key: %v", err)
	}

	// Should have a new active key
	newKey := manager.GetActiveKey()
	if newKey.KeyID == originalKey.KeyID {
		t.Error("Expected new active key after revocation")
	}

	// Old key should NOT be valid (no grace period for emergency revocation)
	if manager.IsValidKey(originalKey.PublicKey) {
		t.Error("Revoked key should not be valid")
	}

	// New key should be valid
	if !manager.IsValidKey(newKey.PublicKey) {
		t.Error("New key should be valid")
	}
}

func TestManager_GracePeriodExpiration(t *testing.T) {
	config := &Config{
		Enabled:              true,
		RotationIntervalDays: 30, // Minimum valid interval
		GracePeriodDays:      7,
		WarningPeriodDays:    7,
		Audit: AuditConfig{
			Enabled: false,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	manager, err := NewManager(config, logger, "test-validator")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Stop()

	if err := manager.Start(); err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}

	originalKey := manager.GetActiveKey()

	// Rotate key manually
	if err := manager.RotateKey("operator", "test rotation"); err != nil {
		t.Fatalf("Failed to rotate key: %v", err)
	}

	// Original key should still be valid during grace period
	if !manager.IsValidKey(originalKey.PublicKey) {
		t.Error("Original key should be valid during grace period")
	}

	// Simulate grace period ending by checking with future timestamp
	// Grace period is 7 days, so check 38 days in future (30 days rotation + 8 days past grace)
	futureTime := time.Now().Add(38 * 24 * time.Hour)
	if manager.IsValidKeyAt(originalKey.PublicKey, futureTime) {
		t.Error("Original key should not be valid after grace period")
	}
}

func TestKeyMetadata_IsValid(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		metadata *KeyMetadata
		checkAt  time.Time
		want     bool
	}{
		{
			name: "active key in valid range",
			metadata: &KeyMetadata{
				Status:      KeyStatusActive,
				ActivatedAt: now.Add(-1 * time.Hour),
				ExpiresAt:   now.Add(1 * time.Hour),
			},
			checkAt: now,
			want:    true,
		},
		{
			name: "active key before activation",
			metadata: &KeyMetadata{
				Status:      KeyStatusActive,
				ActivatedAt: now.Add(1 * time.Hour),
				ExpiresAt:   now.Add(2 * time.Hour),
			},
			checkAt: now,
			want:    false,
		},
		{
			name: "active key after expiration",
			metadata: &KeyMetadata{
				Status:      KeyStatusActive,
				ActivatedAt: now.Add(-2 * time.Hour),
				ExpiresAt:   now.Add(-1 * time.Hour),
			},
			checkAt: now,
			want:    false,
		},
		{
			name: "grace key in grace period",
			metadata: &KeyMetadata{
				Status:      KeyStatusGrace,
				ActivatedAt: now.Add(-2 * time.Hour),
				GraceEndsAt: now.Add(1 * time.Hour),
			},
			checkAt: now,
			want:    true,
		},
		{
			name: "grace key after grace period",
			metadata: &KeyMetadata{
				Status:      KeyStatusGrace,
				ActivatedAt: now.Add(-2 * time.Hour),
				GraceEndsAt: now.Add(-1 * time.Hour),
			},
			checkAt: now,
			want:    false,
		},
		{
			name: "revoked key",
			metadata: &KeyMetadata{
				Status:      KeyStatusRevoked,
				ActivatedAt: now.Add(-1 * time.Hour),
				ExpiresAt:   now.Add(1 * time.Hour),
			},
			checkAt: now,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.metadata.IsValid(tt.checkAt)
			if got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				Enabled:              true,
				RotationIntervalDays: 90,
				GracePeriodDays:      7,
				WarningPeriodDays:    7,
				Audit: AuditConfig{
					Enabled:       true,
					Directory:     "/tmp/audit",
					RetentionDays: 365,
				},
			},
			wantErr: false,
		},
		{
			name: "disabled config",
			config: &Config{
				Enabled: false,
			},
			wantErr: false,
		},
		{
			name: "rotation interval too short",
			config: &Config{
				Enabled:              true,
				RotationIntervalDays: 20, // Too short
				GracePeriodDays:      7,
				WarningPeriodDays:    7,
			},
			wantErr: true,
		},
		{
			name: "rotation interval too long",
			config: &Config{
				Enabled:              true,
				RotationIntervalDays: 400, // Too long
				GracePeriodDays:      7,
				WarningPeriodDays:    7,
			},
			wantErr: true,
		},
		{
			name: "grace period too short",
			config: &Config{
				Enabled:              true,
				RotationIntervalDays: 90,
				GracePeriodDays:      0, // Too short
				WarningPeriodDays:    7,
			},
			wantErr: true,
		},
		{
			name: "missing audit directory",
			config: &Config{
				Enabled:              true,
				RotationIntervalDays: 90,
				GracePeriodDays:      7,
				WarningPeriodDays:    7,
				Audit: AuditConfig{
					Enabled:       true,
					Directory:     "", // Missing
					RetentionDays: 365,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
