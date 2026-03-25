// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package keyrotation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAuditLogger_LogEvent(t *testing.T) {
	tempDir := t.TempDir()

	logger, err := NewAuditLogger(tempDir, 30)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer logger.Close()

	event := &AuditEvent{
		Timestamp:   time.Now(),
		Event:       "key_generated",
		KeyID:       "test-key-123",
		ValidatorID: "test-validator",
		Operator:    "system",
	}

	if err := logger.Log(event); err != nil {
		t.Fatalf("Failed to log event: %v", err)
	}

	// Verify event was written to file
	files, err := filepath.Glob(filepath.Join(tempDir, "audit-*.jsonl"))
	if err != nil {
		t.Fatalf("Failed to list audit files: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("Expected 1 audit file, got %d", len(files))
	}

	// Read and verify the logged event
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("Failed to read audit file: %v", err)
	}

	var loggedEvent AuditEvent
	if err := json.Unmarshal(data, &loggedEvent); err != nil {
		t.Fatalf("Failed to parse audit event: %v", err)
	}

	if loggedEvent.Event != event.Event {
		t.Errorf("Expected event %s, got %s", event.Event, loggedEvent.Event)
	}

	if loggedEvent.KeyID != event.KeyID {
		t.Errorf("Expected key ID %s, got %s", event.KeyID, loggedEvent.KeyID)
	}
}

func TestAuditLogger_ChecksumChaining(t *testing.T) {
	tempDir := t.TempDir()

	logger, err := NewAuditLogger(tempDir, 30)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer logger.Close()

	// Log multiple events
	events := []*AuditEvent{
		{
			Timestamp:   time.Now(),
			Event:       "key_generated",
			KeyID:       "key-1",
			ValidatorID: "test-validator",
		},
		{
			Timestamp:   time.Now(),
			Event:       "key_activated",
			KeyID:       "key-1",
			ValidatorID: "test-validator",
		},
		{
			Timestamp:   time.Now(),
			Event:       "key_rotated",
			KeyID:       "key-2",
			ValidatorID: "test-validator",
		},
	}

	for _, event := range events {
		if err := logger.Log(event); err != nil {
			t.Fatalf("Failed to log event: %v", err)
		}
	}

	// Read all logged events
	files, err := filepath.Glob(filepath.Join(tempDir, "audit-*.jsonl"))
	if err != nil {
		t.Fatalf("Failed to list audit files: %v", err)
	}

	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("Failed to read audit file: %v", err)
	}

	lines := splitLines(data)
	if len(lines) != len(events) {
		t.Fatalf("Expected %d events, got %d", len(events), len(lines))
	}

	var loggedEvents []AuditEvent
	for _, line := range lines {
		var event AuditEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("Failed to parse event: %v", err)
		}
		loggedEvents = append(loggedEvents, event)
	}

	// Verify checksum chaining
	for i := 1; i < len(loggedEvents); i++ {
		if loggedEvents[i].PreviousChecksum != loggedEvents[i-1].Checksum {
			t.Errorf("Event %d checksum chain broken: expected %s, got %s",
				i, loggedEvents[i-1].Checksum, loggedEvents[i].PreviousChecksum)
		}
	}

	// First event should have no previous checksum
	if loggedEvents[0].PreviousChecksum != "" {
		t.Errorf("First event should have no previous checksum, got %s", loggedEvents[0].PreviousChecksum)
	}
}

func TestAuditLogger_OldLogCleanup(t *testing.T) {
	tempDir := t.TempDir()

	// Create some old log files
	oldDates := []string{
		"2024-01-01",
		"2024-02-01",
		"2024-03-01",
	}

	for _, date := range oldDates {
		filename := filepath.Join(tempDir, "audit-"+date+".jsonl")
		if err := os.WriteFile(filename, []byte("test\n"), 0600); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	// Create audit logger with 30 day retention
	logger, err := NewAuditLogger(tempDir, 30)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}

	// Log an event to trigger cleanup
	event := &AuditEvent{
		Timestamp:   time.Now(),
		Event:       "test",
		KeyID:       "test",
		ValidatorID: "test",
	}

	if err := logger.Log(event); err != nil {
		t.Fatalf("Failed to log event: %v", err)
	}

	if err := logger.Close(); err != nil {
		t.Fatalf("Failed to close logger: %v", err)
	}

	// Old files should be deleted
	files, err := filepath.Glob(filepath.Join(tempDir, "audit-*.jsonl"))
	if err != nil {
		t.Fatalf("Failed to list files: %v", err)
	}

	// Should only have today's file
	if len(files) != 1 {
		t.Errorf("Expected 1 file after cleanup, got %d", len(files))
	}
}
