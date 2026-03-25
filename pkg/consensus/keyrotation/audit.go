// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package keyrotation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AuditLogger provides tamper-evident logging for key rotation events.
type AuditLogger struct {
	dir            string
	retentionDays  int
	mu             sync.Mutex
	lastChecksum   string
	file           *os.File
	currentLogFile string
}

// NewAuditLogger creates a new audit logger.
func NewAuditLogger(dir string, retentionDays int) (*AuditLogger, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create audit directory: %w", err)
	}

	a := &AuditLogger{
		dir:           dir,
		retentionDays: retentionDays,
	}

	// Open or create current day's log file
	if err := a.openLogFile(); err != nil {
		return nil, err
	}

	return a, nil
}

// Log records an audit event with tamper-evident chaining.
func (a *AuditLogger) Log(event *AuditEvent) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Link to previous event
	event.PreviousChecksum = a.lastChecksum

	// Calculate checksum for this event
	event.Checksum = a.calculateChecksum(event)
	a.lastChecksum = event.Checksum

	// Rotate log file if date changed
	today := time.Now().Format("2006-01-02")
	if filepath.Base(a.currentLogFile) != fmt.Sprintf("audit-%s.jsonl", today) {
		if err := a.openLogFile(); err != nil {
			return err
		}
	}

	// Write event as JSON line
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	if _, err := a.file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write audit log: %w", err)
	}

	// Sync to disk immediately for audit integrity
	if err := a.file.Sync(); err != nil {
		return fmt.Errorf("failed to sync audit log: %w", err)
	}

	return nil
}

// Close closes the audit logger.
func (a *AuditLogger) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.file != nil {
		return a.file.Close()
	}
	return nil
}

// openLogFile opens the current day's log file.
func (a *AuditLogger) openLogFile() error {
	if a.file != nil {
		a.file.Close()
	}

	today := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("audit-%s.jsonl", today)
	a.currentLogFile = filepath.Join(a.dir, filename)

	var err error
	a.file, err = os.OpenFile(a.currentLogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open audit log file: %w", err)
	}

	return nil
}

// calculateChecksum computes a SHA-256 checksum of the event.
func (a *AuditLogger) calculateChecksum(event *AuditEvent) string {
	h := sha256.New()

	// Include all fields in checksum
	fmt.Fprintf(h, "%s|%s|%s|%s|%s|%s|%s|%s|%s",
		event.Timestamp.Format(time.RFC3339Nano),
		event.Event,
		event.KeyID,
		event.PreviousKeyID,
		event.ValidatorID,
		event.RotationType,
		event.Operator,
		event.Reason,
		event.PreviousChecksum)

	return hex.EncodeToString(h.Sum(nil))
}
