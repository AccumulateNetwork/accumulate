// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package keyrotation

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Manager handles automatic key rotation for a validator.
type Manager struct {
	config      *Config
	logger      *slog.Logger
	validatorID string

	// Mutex protects access to key state
	mu sync.RWMutex

	// Current active key metadata
	activeKey *KeyMetadata

	// Key in grace period (if any)
	graceKey *KeyMetadata

	// Key metadata store
	keys map[string]*KeyMetadata

	// Audit logger
	audit *AuditLogger

	// HSM provider (if configured)
	hsm HSMProvider

	// Ticker for periodic rotation checks
	ticker *time.Ticker

	// Context for shutdown
	ctx    context.Context
	cancel context.CancelFunc

	// Wait group for goroutines
	wg sync.WaitGroup
}

// NewManager creates a new key rotation manager.
func NewManager(config *Config, logger *slog.Logger, validatorID string) (*Manager, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	m := &Manager{
		config:      config,
		logger:      logger,
		validatorID: validatorID,
		keys:        make(map[string]*KeyMetadata),
		ctx:         ctx,
		cancel:      cancel,
	}

	// Initialize audit logger if enabled
	if config.Audit.Enabled {
		var err error
		m.audit, err = NewAuditLogger(config.Audit.Directory, config.Audit.RetentionDays)
		if err != nil {
			return nil, fmt.Errorf("failed to create audit logger: %w", err)
		}
	}

	// Initialize HSM provider if configured
	if config.HSM.Provider != "" && config.HSM.Provider != "none" {
		var err error
		m.hsm, err = NewHSMProvider(config.HSM)
		if err != nil {
			return nil, fmt.Errorf("failed to create HSM provider: %w", err)
		}
	}

	return m, nil
}

// Start begins the automatic rotation process.
func (m *Manager) Start() error {
	if !m.config.Enabled {
		m.logger.Info("Key rotation is disabled")
		return nil
	}

	// If no active key exists, generate initial key
	if m.activeKey == nil {
		if err := m.generateInitialKey(); err != nil {
			return fmt.Errorf("failed to generate initial key: %w", err)
		}
	}

	// Start periodic rotation checker (runs every hour)
	m.ticker = time.NewTicker(1 * time.Hour)
	m.wg.Add(1)
	go m.rotationLoop()

	m.logger.Info("Key rotation manager started",
		"rotation_interval_days", m.config.RotationIntervalDays,
		"grace_period_days", m.config.GracePeriodDays)

	return nil
}

// Stop stops the rotation manager and cleans up resources.
func (m *Manager) Stop() error {
	if m.ticker != nil {
		m.ticker.Stop()
	}

	m.cancel()
	m.wg.Wait()

	if m.audit != nil {
		if err := m.audit.Close(); err != nil {
			return fmt.Errorf("failed to close audit logger: %w", err)
		}
	}

	return nil
}

// GetActiveKey returns the current active signing key.
func (m *Manager) GetActiveKey() *KeyMetadata {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeKey
}

// IsValidKey checks if a public key is valid for signature verification at the current time.
func (m *Manager) IsValidKey(pubKey ed25519.PublicKey) bool {
	return m.IsValidKeyAt(pubKey, time.Now())
}

// IsValidKeyAt checks if a public key is valid for signature verification at a specific time.
func (m *Manager) IsValidKeyAt(pubKey ed25519.PublicKey, at time.Time) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check active key
	if m.activeKey != nil && bytesEqual(m.activeKey.PublicKey, pubKey) {
		return m.activeKey.IsValidForVerification(at)
	}

	// Check grace key
	if m.graceKey != nil && bytesEqual(m.graceKey.PublicKey, pubKey) {
		return m.graceKey.IsValidForVerification(at)
	}

	return false
}

// RotateKey manually triggers a key rotation.
func (m *Manager) RotateKey(operator, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.rotateKeyLocked(RotationTypeManual, operator, reason)
}

// RevokeKey performs an emergency revocation of the active key.
func (m *Manager) RevokeKey(operator, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.activeKey == nil {
		return fmt.Errorf("no active key to revoke")
	}

	m.logger.Warn("Emergency key revocation",
		"key_id", m.activeKey.KeyID,
		"operator", operator,
		"reason", reason)

	// Mark current key as revoked
	now := time.Now()
	m.activeKey.Status = KeyStatusRevoked
	m.activeKey.RevokedAt = &now
	m.activeKey.RevocationReason = reason

	// Log audit event
	if m.audit != nil {
		m.audit.Log(&AuditEvent{
			Timestamp:     now,
			Event:         "key_revoked",
			KeyID:         m.activeKey.KeyID,
			ValidatorID:   m.validatorID,
			RotationType:  RotationTypeEmergency,
			Operator:      operator,
			Reason:        reason,
		})
	}

	// Immediately generate and activate new key (no grace period)
	return m.rotateKeyLocked(RotationTypeEmergency, operator, reason)
}

// rotationLoop periodically checks if rotation is needed.
func (m *Manager) rotationLoop() {
	defer m.wg.Done()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.ticker.C:
			m.checkRotation()
		}
	}
}

// checkRotation checks if rotation is needed and performs it if necessary.
func (m *Manager) checkRotation() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	// Check if active key needs rotation
	if m.activeKey != nil && now.After(m.activeKey.ExpiresAt) {
		m.logger.Info("Automatic key rotation triggered", "key_id", m.activeKey.KeyID)
		if err := m.rotateKeyLocked(RotationTypeAutomatic, "system", "scheduled rotation"); err != nil {
			m.logger.Error("Failed to rotate key", "error", err)
			return
		}
	}

	// Check if grace period has ended
	if m.graceKey != nil && now.After(m.graceKey.GraceEndsAt) {
		m.logger.Info("Grace period ended, expiring old key", "key_id", m.graceKey.KeyID)
		m.graceKey.Status = KeyStatusExpired
		m.graceKey = nil
	}
}

// rotateKeyLocked performs key rotation. Must be called with m.mu held.
func (m *Manager) rotateKeyLocked(rotationType RotationType, operator, reason string) error {
	now := time.Now()

	// Generate new key
	newKey, err := m.generateKey()
	if err != nil {
		return fmt.Errorf("failed to generate new key: %w", err)
	}

	// Move current active key to grace period (unless emergency revocation)
	if m.activeKey != nil && rotationType != RotationTypeEmergency {
		m.activeKey.Status = KeyStatusGrace
		m.activeKey.NextKeyID = newKey.KeyID
		m.graceKey = m.activeKey

		if m.audit != nil {
			m.audit.Log(&AuditEvent{
				Timestamp:      now,
				Event:          "key_grace_started",
				KeyID:          m.activeKey.KeyID,
				ValidatorID:    m.validatorID,
				GracePeriodEnd: m.activeKey.GraceEndsAt,
			})
		}
	}

	// Activate new key
	newKey.ActivatedAt = now
	newKey.Status = KeyStatusActive
	if m.activeKey != nil {
		newKey.PreviousKeyID = m.activeKey.KeyID
	}

	m.activeKey = newKey
	m.keys[newKey.KeyID] = newKey

	// Log audit event
	if m.audit != nil {
		m.audit.Log(&AuditEvent{
			Timestamp:      now,
			Event:          "key_rotated",
			KeyID:          newKey.KeyID,
			PreviousKeyID:  newKey.PreviousKeyID,
			ValidatorID:    m.validatorID,
			RotationType:   rotationType,
			GracePeriodEnd: newKey.ExpiresAt.Add(time.Duration(m.config.GracePeriodDays) * 24 * time.Hour),
			Operator:       operator,
			Reason:         reason,
		})
	}

	m.logger.Info("Key rotation completed",
		"new_key_id", newKey.KeyID,
		"previous_key_id", newKey.PreviousKeyID,
		"rotation_type", rotationType)

	return nil
}

// generateInitialKey generates the first key for a new validator.
func (m *Manager) generateInitialKey() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key, err := m.generateKey()
	if err != nil {
		return err
	}

	now := time.Now()
	key.ActivatedAt = now
	key.Status = KeyStatusActive

	m.activeKey = key
	m.keys[key.KeyID] = key

	if m.audit != nil {
		m.audit.Log(&AuditEvent{
			Timestamp:   now,
			Event:       "key_generated",
			KeyID:       key.KeyID,
			ValidatorID: m.validatorID,
			Operator:    "system",
		})
	}

	m.logger.Info("Initial key generated", "key_id", key.KeyID)
	return nil
}

// generateKey generates a new key using HSM if configured, or raw Ed25519 otherwise.
func (m *Manager) generateKey() (*KeyMetadata, error) {
	now := time.Now()
	keyID := generateKeyID(now)

	var pubKey ed25519.PublicKey
	var err error

	if m.hsm != nil {
		// Use HSM to generate key
		pubKey, err = m.hsm.GenerateKey(m.ctx, keyID)
		if err != nil {
			return nil, fmt.Errorf("HSM key generation failed: %w", err)
		}
		m.logger.Info("Generated key in HSM", "key_id", keyID)
	} else {
		// Generate raw Ed25519 key
		pub, _, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("key generation failed: %w", err)
		}
		pubKey = pub
		m.logger.Info("Generated raw Ed25519 key", "key_id", keyID)
	}

	rotationInterval := time.Duration(m.config.RotationIntervalDays) * 24 * time.Hour
	gracePeriod := time.Duration(m.config.GracePeriodDays) * 24 * time.Hour

	metadata := &KeyMetadata{
		KeyID:       keyID,
		PublicKey:   pubKey,
		CreatedAt:   now,
		ExpiresAt:   now.Add(rotationInterval),
		GraceEndsAt: now.Add(rotationInterval + gracePeriod),
		Status:      KeyStatusPending,
	}

	return metadata, nil
}

// generateKeyID creates a unique key identifier.
func generateKeyID(t time.Time) string {
	h := sha256.New()
	h.Write([]byte(t.Format(time.RFC3339Nano)))
	hash := h.Sum(nil)
	return fmt.Sprintf("key-%s-%s", t.Format("2006-01-02"), hex.EncodeToString(hash[:8]))
}

// bytesEqual compares two byte slices for equality.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
