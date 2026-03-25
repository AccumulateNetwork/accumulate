// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package config

// KeyRotationConfig contains configuration for validator key rotation.
type KeyRotationConfig struct {
	// Enabled controls whether automatic key rotation is enabled.
	Enabled bool `toml:"enabled"`

	// RotationIntervalDays is the interval between key rotations (90-180 recommended).
	RotationIntervalDays int `toml:"rotation_interval_days"`

	// GracePeriodDays is the overlap period where both old and new keys are valid.
	GracePeriodDays int `toml:"grace_period_days"`

	// WarningPeriodDays is when to generate a new key before rotation.
	WarningPeriodDays int `toml:"warning_period_days"`

	// Audit contains audit logging configuration.
	Audit KeyRotationAuditConfig `toml:"audit"`

	// HSM contains HSM/KMS provider configuration.
	HSM KeyRotationHSMConfig `toml:"hsm"`

	// Alert contains alerting configuration.
	Alert KeyRotationAlertConfig `toml:"alert"`
}

// KeyRotationAuditConfig contains audit logging configuration for key rotation.
type KeyRotationAuditConfig struct {
	// Enabled controls whether audit logging is enabled.
	Enabled bool `toml:"enabled"`

	// Directory where audit logs are stored.
	Directory string `toml:"directory"`

	// RetentionDays is how long to retain audit logs.
	RetentionDays int `toml:"retention_days"`

	// LogSigningOperations controls whether individual signing operations are logged.
	LogSigningOperations bool `toml:"log_signing_operations"`
}

// KeyRotationHSMConfig contains HSM/KMS provider configuration for key rotation.
type KeyRotationHSMConfig struct {
	// Provider: "aws-cloudhsm", "google-cloud-kms", "vault", or "none"
	Provider string `toml:"provider"`

	// AWS CloudHSM configuration
	AWSClusterID string `toml:"aws_cluster_id"`
	AWSRegion    string `toml:"aws_region"`
	AWSKeyLabel  string `toml:"aws_key_label"`

	// Google Cloud KMS configuration
	GCPProjectID string `toml:"gcp_project_id"`
	GCPLocation  string `toml:"gcp_location"`
	GCPKeyRing   string `toml:"gcp_key_ring"`
	GCPKeyName   string `toml:"gcp_key_name"`

	// HashiCorp Vault configuration
	VaultAddress   string `toml:"vault_address"`
	VaultTokenFile string `toml:"vault_token_file"`
	VaultMountPath string `toml:"vault_mount_path"`
	VaultKeyName   string `toml:"vault_key_name"`
}

// KeyRotationAlertConfig contains alerting configuration for key rotation.
type KeyRotationAlertConfig struct {
	// Email addresses to notify on key events.
	Email []string `toml:"email"`

	// PagerDutyIntegrationKey for critical alerts.
	PagerDutyIntegrationKey string `toml:"pagerduty_integration_key"`

	// SlackWebhook for notifications.
	SlackWebhook string `toml:"slack_webhook"`
}

// DefaultKeyRotationConfig returns default key rotation configuration.
func DefaultKeyRotationConfig() KeyRotationConfig {
	return KeyRotationConfig{
		Enabled:              false, // Disabled by default
		RotationIntervalDays: 90,
		GracePeriodDays:      7,
		WarningPeriodDays:    7,
		Audit: KeyRotationAuditConfig{
			Enabled:              true,
			Directory:            "/var/log/accumulate/key-rotation",
			RetentionDays:        730, // 2 years
			LogSigningOperations: false,
		},
		HSM: KeyRotationHSMConfig{
			Provider: "none",
		},
		Alert: KeyRotationAlertConfig{
			Email: []string{},
		},
	}
}
