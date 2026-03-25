// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package keyrotation

import (
	"crypto/ed25519"
	"time"
)

// KeyStatus represents the current status of a validator signing key.
type KeyStatus int

const (
	// KeyStatusPending indicates the key has been generated but not yet activated.
	KeyStatusPending KeyStatus = iota
	// KeyStatusActive indicates the key is currently active for signing.
	KeyStatusActive
	// KeyStatusGrace indicates the key is in the grace period after rotation.
	KeyStatusGrace
	// KeyStatusExpired indicates the key is past its grace period.
	KeyStatusExpired
	// KeyStatusRevoked indicates the key has been emergency-revoked.
	KeyStatusRevoked
)

// String returns the string representation of the key status.
func (s KeyStatus) String() string {
	switch s {
	case KeyStatusPending:
		return "pending"
	case KeyStatusActive:
		return "active"
	case KeyStatusGrace:
		return "grace"
	case KeyStatusExpired:
		return "expired"
	case KeyStatusRevoked:
		return "revoked"
	default:
		return "unknown"
	}
}

// KeyMetadata contains metadata about a validator signing key.
type KeyMetadata struct {
	// KeyID is the unique identifier for this key.
	KeyID string

	// PublicKey is the Ed25519 public key.
	PublicKey ed25519.PublicKey

	// CreatedAt is when the key was generated.
	CreatedAt time.Time

	// ActivatedAt is when the key became active for signing.
	// This may be after CreatedAt if the key was pre-generated.
	ActivatedAt time.Time

	// ExpiresAt is when the key should be rotated.
	ExpiresAt time.Time

	// GraceEndsAt is when the grace period ends and the key becomes fully expired.
	GraceEndsAt time.Time

	// RevokedAt is set if the key was emergency-revoked.
	RevokedAt *time.Time

	// Status is the current status of the key.
	Status KeyStatus

	// PreviousKeyID links to the previous key in the rotation chain.
	PreviousKeyID string

	// NextKeyID links to the next key in the rotation chain.
	NextKeyID string

	// RevocationReason is set if the key was revoked.
	RevocationReason string
}

// IsValid returns true if the key is valid for signing at the given time.
func (m *KeyMetadata) IsValid(at time.Time) bool {
	switch m.Status {
	case KeyStatusActive:
		return !at.Before(m.ActivatedAt) && at.Before(m.ExpiresAt)
	case KeyStatusGrace:
		return !at.Before(m.ActivatedAt) && at.Before(m.GraceEndsAt)
	default:
		return false
	}
}

// IsValidForVerification returns true if the key is valid for signature verification at the given time.
// This is more permissive than IsValid - keys in grace period can still verify signatures.
func (m *KeyMetadata) IsValidForVerification(at time.Time) bool {
	switch m.Status {
	case KeyStatusActive, KeyStatusGrace:
		return !at.Before(m.ActivatedAt) && at.Before(m.GraceEndsAt)
	default:
		return false
	}
}

// RotationType indicates how a key rotation was triggered.
type RotationType string

const (
	// RotationTypeAutomatic indicates a scheduled automatic rotation.
	RotationTypeAutomatic RotationType = "automatic"
	// RotationTypeManual indicates an operator-initiated rotation.
	RotationTypeManual RotationType = "manual"
	// RotationTypeEmergency indicates an emergency revocation and rotation.
	RotationTypeEmergency RotationType = "emergency"
)

// AuditEvent represents an auditable key rotation event.
type AuditEvent struct {
	// Timestamp of the event.
	Timestamp time.Time

	// Event type (key_generated, key_activated, key_rotated, key_revoked, etc.)
	Event string

	// KeyID of the key involved.
	KeyID string

	// PreviousKeyID for rotation events.
	PreviousKeyID string

	// ValidatorID that owns this key.
	ValidatorID string

	// RotationType for rotation events.
	RotationType RotationType

	// GracePeriodEnd for rotation events.
	GracePeriodEnd time.Time

	// Operator who triggered the event (empty for automatic events).
	Operator string

	// Reason for revocation or manual rotation.
	Reason string

	// Checksum provides tamper-evident chaining of audit events.
	Checksum string

	// PreviousChecksum links to the previous audit event.
	PreviousChecksum string
}

// Config contains configuration for the key rotation system.
type Config struct {
	// Enabled controls whether automatic key rotation is enabled.
	Enabled bool

	// RotationIntervalDays is the interval between key rotations (90-180 recommended).
	RotationIntervalDays int

	// GracePeriodDays is the overlap period where both old and new keys are valid.
	GracePeriodDays int

	// WarningPeriodDays is when to generate a new key before rotation (should equal GracePeriodDays).
	WarningPeriodDays int

	// AuditConfig contains audit logging configuration.
	Audit AuditConfig

	// HSMConfig contains HSM/KMS provider configuration.
	HSM HSMConfig

	// AlertConfig contains alerting configuration.
	Alert AlertConfig
}

// AuditConfig contains audit logging configuration.
type AuditConfig struct {
	// Enabled controls whether audit logging is enabled.
	Enabled bool

	// Directory where audit logs are stored.
	Directory string

	// RetentionDays is how long to retain audit logs.
	RetentionDays int

	// LogSigningOperations controls whether individual signing operations are logged.
	// WARNING: This can generate a lot of log data.
	LogSigningOperations bool
}

// HSMConfig contains HSM/KMS provider configuration.
type HSMConfig struct {
	// Provider: "aws-cloudhsm", "google-cloud-kms", "vault", or "none"
	Provider string

	// AWS CloudHSM configuration
	AWSClusterID string
	AWSRegion    string
	AWSKeyLabel  string

	// Google Cloud KMS configuration
	GCPProjectID string
	GCPLocation  string
	GCPKeyRing   string
	GCPKeyName   string

	// HashiCorp Vault configuration
	VaultAddress   string
	VaultTokenFile string
	VaultMountPath string
	VaultKeyName   string
}

// AlertConfig contains alerting configuration.
type AlertConfig struct {
	// Email addresses to notify on key events.
	Email []string

	// PagerDutyIntegrationKey for critical alerts.
	PagerDutyIntegrationKey string

	// SlackWebhook for notifications.
	SlackWebhook string
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}

	if c.RotationIntervalDays < 30 || c.RotationIntervalDays > 365 {
		return &ValidationError{Field: "rotation_interval_days", Message: "must be between 30 and 365"}
	}

	if c.GracePeriodDays < 1 || c.GracePeriodDays > 30 {
		return &ValidationError{Field: "grace_period_days", Message: "must be between 1 and 30"}
	}

	if c.WarningPeriodDays < 1 || c.WarningPeriodDays > c.RotationIntervalDays {
		return &ValidationError{Field: "warning_period_days", Message: "must be between 1 and rotation_interval_days"}
	}

	if c.Audit.Enabled {
		if c.Audit.Directory == "" {
			return &ValidationError{Field: "audit.directory", Message: "must be specified when audit is enabled"}
		}
		if c.Audit.RetentionDays < 30 {
			return &ValidationError{Field: "audit.retention_days", Message: "must be at least 30 days"}
		}
	}

	return nil
}

// ValidationError represents a configuration validation error.
type ValidationError struct {
	Field   string
	Message string
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	return "validation error: " + e.Field + ": " + e.Message
}
