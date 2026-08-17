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

// AuditLogger handles tamper-evident audit logging for key rotation events.
type AuditLogger struct {
	directory     string
	retentionDays int

	mu               sync.Mutex
	currentFile      *os.File
	currentDate      string
	previousChecksum string
}

// NewAuditLogger creates a new audit logger.
func NewAuditLogger(directory string, retentionDays int) (*AuditLogger, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, fmt.Errorf("failed to create audit directory: %w", err)
	}

	al := &AuditLogger{
		directory:     directory,
		retentionDays: retentionDays,
	}

	// Load the last checksum from the most recent log file
	if err := al.loadLastChecksum(); err != nil {
		// If no previous logs exist, that's fine
		return al, nil
	}

	return al, nil
}

// Log writes an audit event to the log.
func (al *AuditLogger) Log(event *AuditEvent) error {
	al.mu.Lock()
	defer al.mu.Unlock()

	// Link to previous event
	event.PreviousChecksum = al.previousChecksum

	// Calculate checksum for this event
	checksum, err := al.calculateChecksum(event)
	if err != nil {
		return fmt.Errorf("failed to calculate checksum: %w", err)
	}
	event.Checksum = checksum

	// Get or create log file for current date
	if err := al.ensureCurrentFile(); err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	// Write event as JSON
	encoder := json.NewEncoder(al.currentFile)
	if err := encoder.Encode(event); err != nil {
		return fmt.Errorf("failed to write audit event: %w", err)
	}

	// Flush to disk immediately for audit trail integrity
	if err := al.currentFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync audit log: %w", err)
	}

	// Update previous checksum for next event
	al.previousChecksum = checksum

	return nil
}

// Close closes the audit logger and performs cleanup.
func (al *AuditLogger) Close() error {
	al.mu.Lock()
	defer al.mu.Unlock()

	if al.currentFile != nil {
		if err := al.currentFile.Close(); err != nil {
			return err
		}
		al.currentFile = nil
	}

	// Clean up old log files
	return al.cleanupOldLogs()
}

// ensureCurrentFile ensures a log file exists for the current date.
func (al *AuditLogger) ensureCurrentFile() error {
	currentDate := time.Now().Format("2006-01-02")

	// If we already have a file open for today, we're done
	if al.currentFile != nil && al.currentDate == currentDate {
		return nil
	}

	// Close previous file if open
	if al.currentFile != nil {
		if err := al.currentFile.Close(); err != nil {
			return err
		}
	}

	// Open new file for current date
	filename := filepath.Join(al.directory, fmt.Sprintf("audit-%s.jsonl", currentDate))
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}

	al.currentFile = file
	al.currentDate = currentDate

	return nil
}

// calculateChecksum calculates a SHA-256 checksum for an audit event.
func (al *AuditLogger) calculateChecksum(event *AuditEvent) (string, error) {
	// Create a copy without the checksum field itself
	eventCopy := *event
	eventCopy.Checksum = ""

	data, err := json.Marshal(eventCopy)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(hash[:]), nil
}

// loadLastChecksum loads the checksum from the last log entry.
func (al *AuditLogger) loadLastChecksum() error {
	// Find the most recent log file
	files, err := filepath.Glob(filepath.Join(al.directory, "audit-*.jsonl"))
	if err != nil {
		return err
	}

	if len(files) == 0 {
		// No previous logs
		return nil
	}

	// Files are sorted lexicographically, so the last one is most recent
	lastFile := files[len(files)-1]

	// Read the last line from the file
	data, err := os.ReadFile(lastFile)
	if err != nil {
		return err
	}

	// Find the last newline
	lines := splitLines(data)
	if len(lines) == 0 {
		return nil
	}

	// Parse the last event
	var lastEvent AuditEvent
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &lastEvent); err != nil {
		return err
	}

	al.previousChecksum = lastEvent.Checksum
	return nil
}

// cleanupOldLogs removes log files older than the retention period.
func (al *AuditLogger) cleanupOldLogs() error {
	cutoffDate := time.Now().AddDate(0, 0, -al.retentionDays)

	files, err := filepath.Glob(filepath.Join(al.directory, "audit-*.jsonl"))
	if err != nil {
		return err
	}

	for _, file := range files {
		// Extract date from filename
		base := filepath.Base(file)
		dateStr := base[6:16] // "audit-YYYY-MM-DD.jsonl" -> "YYYY-MM-DD"

		fileDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue // Skip files with invalid date format
		}

		if fileDate.Before(cutoffDate) {
			if err := os.Remove(file); err != nil {
				return fmt.Errorf("failed to remove old log file %s: %w", file, err)
			}
		}
	}

	return nil
}

// splitLines splits data into lines, removing empty lines.
func splitLines(data []byte) []string {
	var lines []string
	var line []byte

	for _, b := range data {
		if b == '\n' {
			if len(line) > 0 {
				lines = append(lines, string(line))
				line = nil
			}
		} else {
			line = append(line, b)
		}
	}

	if len(line) > 0 {
		lines = append(lines, string(line))
	}

	return lines
}
