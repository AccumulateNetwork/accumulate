// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package keyrotation

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	config := &Config{
		Enabled:              true,
		RotationIntervalDays: 90,
		GracePeriodDays:      7,
		WarningPeriodDays:    7,
	}

	manager, err := NewManager(config, slog.Default(), "test-validator")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	if manager == nil {
		t.Fatal("Manager is nil")
	}

	if manager.validatorID != "test-validator" {
		t.Errorf("Expected validator ID 'test-validator', got '%s'", manager.validatorID)
	}
}

func TestNewManagerWithInvalidConfig(t *testing.T) {
	config := &Config{
		Enabled:              true,
		RotationIntervalDays: 10, // Too short
		GracePeriodDays:      7,
		WarningPeriodDays:    7,
	}

	_, err := NewManager(config, slog.Default(), "test-validator")
	if err == nil {
		t.Fatal("Expected error for invalid config, got nil")
	}
}

func TestManagerStartStop(t *testing.T) {
	config := &Config{
		Enabled:              true,
		RotationIntervalDays: 90,
		GracePeriodDays:      7,
		WarningPeriodDays:    7,
	}

	manager, err := NewManager(config, slog.Default(), "test-validator")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	err = manager.Start()
	if err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}

	// Verify initial key was generated
	activeKey := manager.GetActiveKey()
	if activeKey == nil {
		t.Fatal("No active key after start")
	}

	if activeKey.Status != KeyStatusActive {
		t.Errorf("Expected active key status, got %s", activeKey.Status)
	}

	err = manager.Stop()
	if err != nil {
		t.Fatalf("Failed to stop manager: %v", err)
	}
}

func TestManagerDisabled(t *testing.T) {
	config := &Config{
		Enabled:              false,
		RotationIntervalDays: 90,
		GracePeriodDays:      7,
		WarningPeriodDays:    7,
	}

	manager, err := NewManager(config, slog.Default(), "test-validator")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	err = manager.Start()
	if err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}

	// Should not generate a key when disabled
	activeKey := manager.GetActiveKey()
	if activeKey != nil {
		t.Fatal("Active key should be nil when disabled")
	}

	err = manager.Stop()
	if err != nil {
		t.Fatalf("Failed to stop manager: %v", err)
	}
}

func TestManualRotation(t *testing.T) {
	config := &Config{
		Enabled:              true,
		RotationIntervalDays: 90,
		GracePeriodDays:      7,
		WarningPeriodDays:    7,
	}

	manager, err := NewManager(config, slog.Default(), "test-validator")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	err = manager.Start()
	if err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}
	defer manager.Stop()

	firstKey := manager.GetActiveKey()
	if firstKey == nil {
		t.Fatal("No active key after start")
	}

	// Perform manual rotation
	err = manager.RotateKey("test-operator", "test rotation")
	if err != nil {
		t.Fatalf("Manual rotation failed: %v", err)
	}

	secondKey := manager.GetActiveKey()
	if secondKey == nil {
		t.Fatal("No active key after rotation")
	}

	if firstKey.KeyID == secondKey.KeyID {
		t.Error("Key was not rotated")
	}

	if secondKey.PreviousKeyID != firstKey.KeyID {
		t.Error("Previous key ID not linked correctly")
	}
}

func TestEmergencyRevocation(t *testing.T) {
	config := &Config{
		Enabled:              true,
		RotationIntervalDays: 90,
		GracePeriodDays:      7,
		WarningPeriodDays:    7,
	}

	manager, err := NewManager(config, slog.Default(), "test-validator")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	err = manager.Start()
	if err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}
	defer manager.Stop()

	firstKey := manager.GetActiveKey()
	if firstKey == nil {
		t.Fatal("No active key after start")
	}

	// Perform emergency revocation
	err = manager.RevokeKey("test-operator", "security incident")
	if err != nil {
		t.Fatalf("Emergency revocation failed: %v", err)
	}

	secondKey := manager.GetActiveKey()
	if secondKey == nil {
		t.Fatal("No active key after revocation")
	}

	if firstKey.KeyID == secondKey.KeyID {
		t.Error("Key was not rotated")
	}

	// Check that first key was revoked
	if firstKey.Status != KeyStatusRevoked {
		t.Errorf("Expected revoked status, got %s", firstKey.Status)
	}

	if firstKey.RevokedAt == nil {
		t.Error("RevokedAt timestamp not set")
	}

	if firstKey.RevocationReason != "security incident" {
		t.Errorf("Expected revocation reason 'security incident', got '%s'", firstKey.RevocationReason)
	}
}

func TestKeyValidation(t *testing.T) {
	config := &Config{
		Enabled:              true,
		RotationIntervalDays: 90,
		GracePeriodDays:      7,
		WarningPeriodDays:    7,
	}

	manager, err := NewManager(config, slog.Default(), "test-validator")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	err = manager.Start()
	if err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}
	defer manager.Stop()

	activeKey := manager.GetActiveKey()
	if activeKey == nil {
		t.Fatal("No active key")
	}

	// Active key should be valid
	if !manager.IsValidKey(activeKey.PublicKey) {
		t.Error("Active key should be valid")
	}

	// Rotate to create a grace key
	err = manager.RotateKey("test-operator", "test")
	if err != nil {
		t.Fatalf("Rotation failed: %v", err)
	}

	// Old key should still be valid during grace period
	if !manager.IsValidKey(activeKey.PublicKey) {
		t.Error("Grace key should still be valid")
	}
}

func TestAuditLogging(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "keyrotation-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	config := &Config{
		Enabled:              true,
		RotationIntervalDays: 90,
		GracePeriodDays:      7,
		WarningPeriodDays:    7,
		Audit: AuditConfig{
			Enabled:       true,
			Directory:     filepath.Join(tempDir, "audit"),
			RetentionDays: 30,
		},
	}

	manager, err := NewManager(config, slog.Default(), "test-validator")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	err = manager.Start()
	if err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}

	// Perform rotation to generate audit logs
	err = manager.RotateKey("test-operator", "test rotation")
	if err != nil {
		t.Fatalf("Rotation failed: %v", err)
	}

	err = manager.Stop()
	if err != nil {
		t.Fatalf("Failed to stop manager: %v", err)
	}

	// Check that audit log file was created
	auditDir := filepath.Join(tempDir, "audit")
	files, err := os.ReadDir(auditDir)
	if err != nil {
		t.Fatalf("Failed to read audit directory: %v", err)
	}

	if len(files) == 0 {
		t.Error("No audit log files created")
	}

	// Verify audit log file has content
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		path := filepath.Join(auditDir, file.Name())
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("Failed to stat audit file: %v", err)
			continue
		}
		if info.Size() == 0 {
			t.Error("Audit log file is empty")
		}
	}
}

func TestKeyMetadataValidation(t *testing.T) {
	now := time.Now()
	key := &KeyMetadata{
		KeyID:       "test-key",
		ActivatedAt: now,
		ExpiresAt:   now.Add(90 * 24 * time.Hour),
		GraceEndsAt: now.Add(97 * 24 * time.Hour),
		Status:      KeyStatusActive,
	}

	// Test active key validation
	if !key.IsValid(now.Add(1 * time.Hour)) {
		t.Error("Active key should be valid during active period")
	}

	if key.IsValid(now.Add(91 * 24 * time.Hour)) {
		t.Error("Active key should not be valid after expiration")
	}

	// Test grace key validation
	key.Status = KeyStatusGrace
	if !key.IsValidForVerification(now.Add(91 * 24 * time.Hour)) {
		t.Error("Grace key should be valid for verification during grace period")
	}

	if key.IsValidForVerification(now.Add(98 * 24 * time.Hour)) {
		t.Error("Grace key should not be valid after grace period")
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				Enabled:              true,
				RotationIntervalDays: 90,
				GracePeriodDays:      7,
				WarningPeriodDays:    7,
			},
			wantErr: false,
		},
		{
			name: "disabled config is valid",
			config: Config{
				Enabled: false,
			},
			wantErr: false,
		},
		{
			name: "rotation interval too short",
			config: Config{
				Enabled:              true,
				RotationIntervalDays: 20,
				GracePeriodDays:      7,
				WarningPeriodDays:    7,
			},
			wantErr: true,
		},
		{
			name: "rotation interval too long",
			config: Config{
				Enabled:              true,
				RotationIntervalDays: 400,
				GracePeriodDays:      7,
				WarningPeriodDays:    7,
			},
			wantErr: true,
		},
		{
			name: "grace period too short",
			config: Config{
				Enabled:              true,
				RotationIntervalDays: 90,
				GracePeriodDays:      0,
				WarningPeriodDays:    7,
			},
			wantErr: true,
		},
		{
			name: "warning period too long",
			config: Config{
				Enabled:              true,
				RotationIntervalDays: 90,
				GracePeriodDays:      7,
				WarningPeriodDays:    100,
			},
			wantErr: true,
		},
		{
			name: "audit enabled without directory",
			config: Config{
				Enabled:              true,
				RotationIntervalDays: 90,
				GracePeriodDays:      7,
				WarningPeriodDays:    7,
				Audit: AuditConfig{
					Enabled: true,
				},
			},
			wantErr: true,
		},
		{
			name: "audit retention too short",
			config: Config{
				Enabled:              true,
				RotationIntervalDays: 90,
				GracePeriodDays:      7,
				WarningPeriodDays:    7,
				Audit: AuditConfig{
					Enabled:       true,
					Directory:     "/tmp/audit",
					RetentionDays: 10,
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

func TestRevokeKeyWithoutActiveKey(t *testing.T) {
	config := &Config{
		Enabled:              true,
		RotationIntervalDays: 90,
		GracePeriodDays:      7,
		WarningPeriodDays:    7,
	}

	manager, err := NewManager(config, slog.Default(), "test-validator")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Try to revoke without starting (no active key)
	err = manager.RevokeKey("test-operator", "test")
	if err == nil {
		t.Error("Expected error when revoking without active key")
	}
}

func TestIsValidKeyWithInvalidKey(t *testing.T) {
	config := &Config{
		Enabled:              true,
		RotationIntervalDays: 90,
		GracePeriodDays:      7,
		WarningPeriodDays:    7,
	}

	manager, err := NewManager(config, slog.Default(), "test-validator")
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	err = manager.Start()
	if err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}
	defer manager.Stop()

	// Test with a random key that doesn't match
	randomKey := make([]byte, 32)
	if manager.IsValidKey(randomKey) {
		t.Error("Random key should not be valid")
	}
}

func TestKeyStatusTransitions(t *testing.T) {
	now := time.Now()
	key := &KeyMetadata{
		KeyID:       "test-key",
		ActivatedAt: now,
		ExpiresAt:   now.Add(90 * 24 * time.Hour),
		GraceEndsAt: now.Add(97 * 24 * time.Hour),
		Status:      KeyStatusPending,
	}

	// Pending key should not be valid
	if key.IsValid(now) {
		t.Error("Pending key should not be valid")
	}

	// Expired key should not be valid
	key.Status = KeyStatusExpired
	if key.IsValid(now) {
		t.Error("Expired key should not be valid")
	}

	// Revoked key should not be valid
	key.Status = KeyStatusRevoked
	if key.IsValid(now) {
		t.Error("Revoked key should not be valid")
	}

	if key.IsValidForVerification(now) {
		t.Error("Revoked key should not be valid for verification")
	}
}

func TestGenerateKeyID(t *testing.T) {
	t1 := time.Now()
	t2 := t1.Add(1 * time.Second)

	id1 := generateKeyID(t1)
	id2 := generateKeyID(t2)

	// Different times should produce different IDs
	if id1 == id2 {
		t.Error("Different times should produce different key IDs")
	}

	// Same time should produce same ID
	id3 := generateKeyID(t1)
	if id1 != id3 {
		t.Error("Same time should produce same key ID")
	}
}

func TestBytesEqual(t *testing.T) {
	a := []byte{1, 2, 3, 4}
	b := []byte{1, 2, 3, 4}
	c := []byte{1, 2, 3, 5}
	d := []byte{1, 2, 3}

	if !bytesEqual(a, b) {
		t.Error("Equal byte slices not recognized as equal")
	}

	if bytesEqual(a, c) {
		t.Error("Different byte slices recognized as equal")
	}

	if bytesEqual(a, d) {
		t.Error("Different length byte slices recognized as equal")
	}
}
