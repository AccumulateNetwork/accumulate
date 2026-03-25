// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package keyrotation

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAuditLogger(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "audit-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logger, err := NewAuditLogger(tempDir, 30)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer logger.Close()

	// Log an event
	event := &AuditEvent{
		Timestamp:    time.Now(),
		Event:        "key_generated",
		KeyID:        "test-key-1",
		ValidatorID:  "test-validator",
		RotationType: RotationTypeAutomatic,
		Operator:     "system",
	}

	err = logger.Log(event)
	if err != nil {
		t.Fatalf("Failed to log event: %v", err)
	}

	// Verify checksum was set
	if event.Checksum == "" {
		t.Error("Checksum was not set")
	}

	// Log another event
	event2 := &AuditEvent{
		Timestamp:    time.Now(),
		Event:        "key_rotated",
		KeyID:        "test-key-2",
		PreviousKeyID: "test-key-1",
		ValidatorID:  "test-validator",
		RotationType: RotationTypeManual,
		Operator:     "admin",
		Reason:       "manual rotation",
	}

	err = logger.Log(event2)
	if err != nil {
		t.Fatalf("Failed to log second event: %v", err)
	}

	// Verify chaining
	if event2.PreviousChecksum != event.Checksum {
		t.Error("Events not properly chained")
	}
}

func TestAuditLoggerDirectoryCreation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "audit-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	auditDir := filepath.Join(tempDir, "nested", "audit", "dir")

	logger, err := NewAuditLogger(auditDir, 30)
	if err != nil {
		t.Fatalf("Failed to create audit logger with nested directory: %v", err)
	}
	defer logger.Close()

	// Verify directory was created
	info, err := os.Stat(auditDir)
	if err != nil {
		t.Fatalf("Audit directory was not created: %v", err)
	}

	if !info.IsDir() {
		t.Error("Audit path is not a directory")
	}
}

func TestAuditLoggerFileRotation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "audit-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logger, err := NewAuditLogger(tempDir, 30)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer logger.Close()

	// Log an event
	event := &AuditEvent{
		Timestamp:   time.Now(),
		Event:       "test_event",
		KeyID:       "test-key",
		ValidatorID: "test-validator",
	}

	err = logger.Log(event)
	if err != nil {
		t.Fatalf("Failed to log event: %v", err)
	}

	// Verify log file was created with today's date
	today := time.Now().Format("2006-01-02")
	expectedFile := filepath.Join(tempDir, "audit-"+today+".jsonl")

	info, err := os.Stat(expectedFile)
	if err != nil {
		t.Fatalf("Audit log file not created: %v", err)
	}

	if info.Size() == 0 {
		t.Error("Audit log file is empty")
	}
}

func TestCalculateChecksum(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "audit-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logger, err := NewAuditLogger(tempDir, 30)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer logger.Close()

	event := &AuditEvent{
		Timestamp:    time.Now(),
		Event:        "test_event",
		KeyID:        "test-key",
		ValidatorID:  "test-validator",
	}

	checksum1 := logger.calculateChecksum(event)
	checksum2 := logger.calculateChecksum(event)

	// Same event should produce same checksum
	if checksum1 != checksum2 {
		t.Error("Checksums not deterministic")
	}

	// Different event should produce different checksum
	event.KeyID = "different-key"
	checksum3 := logger.calculateChecksum(event)
	if checksum1 == checksum3 {
		t.Error("Different events should have different checksums")
	}
}
