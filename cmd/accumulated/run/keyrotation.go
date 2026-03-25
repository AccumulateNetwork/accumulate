// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log/slog"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/keyrotation"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// KeyRotation service IOC providers
var (
	keyRotationProvidesManager = ioc.Provides[*keyrotation.Manager](func(s *KeyRotationService) string { return s.Partition })
)

// Requires returns the IOC requirements for key rotation.
func (s *KeyRotationService) Requires() []ioc.Requirement {
	return nil
}

// Provides returns the IOC provisions for key rotation.
func (s *KeyRotationService) Provides() []ioc.Provided {
	return []ioc.Provided{
		keyRotationProvidesManager.Provided(s),
	}
}

// Prometheus metrics
var (
	krMeter               = otel.Meter("gitlab.com/accumulatenetwork/accumulate/pkg/consensus/keyrotation")
	krRotationEvents      = must(krMeter.Int64Counter("key_rotation_events"))
	krCurrentKeyAge       = must(krMeter.Int64ObservableGauge("key_rotation_current_age_days"))
	krDaysUntilExpiration = must(krMeter.Int64ObservableGauge("key_rotation_days_until_expiration"))
)

// start initializes and starts the key rotation service.
func (s *KeyRotationService) start(inst *Instance) error {
	// Get the validator key
	validatorKeyAddr, err := s.ValidatorKey.get(inst)
	if err != nil {
		return errors.UnknownError.WithFormat("get validator key: %w", err)
	}
	validatorKey, ok := validatorKeyAddr.GetPrivateKey()
	if !ok {
		return errors.BadRequest.With("validator key is not a private key")
	}
	if len(validatorKey) != ed25519.PrivateKeySize {
		return errors.BadRequest.WithFormat("validator key has wrong size: %d", len(validatorKey))
	}

	// Convert config
	config := &keyrotation.Config{
		Enabled:              s.Config.Enabled,
		RotationIntervalDays: int(s.Config.RotationIntervalDays),
		GracePeriodDays:      int(s.Config.GracePeriodDays),
		WarningPeriodDays:    int(s.Config.WarningPeriodDays),
	}

	// Set audit config if provided
	if s.Config.AuditDirectory != "" {
		config.Audit.Enabled = true
		config.Audit.Directory = inst.path(s.Config.AuditDirectory)
		if s.Config.AuditRetentionDays > 0 {
			config.Audit.RetentionDays = int(s.Config.AuditRetentionDays)
		} else {
			config.Audit.RetentionDays = 365 // Default 1 year
		}
	}

	// Create manager
	validatorID := fmt.Sprintf("%s-validator", s.Partition)
	manager, err := keyrotation.NewManager(config, slog.Default(), validatorID)
	if err != nil {
		return errors.UnknownError.WithFormat("create key rotation manager: %w", err)
	}

	// Register the manager
	s.manager = manager
	err = keyRotationProvidesManager.Register(inst.services, s, manager)
	if err != nil {
		return errors.UnknownError.WithFormat("register key rotation manager: %w", err)
	}

	// Start the manager
	err = manager.Start()
	if err != nil {
		return errors.UnknownError.WithFormat("start key rotation manager: %w", err)
	}

	// Register Prometheus metrics
	partitionAttr := attribute.String("partition", s.Partition)

	// Track rotation events
	inst.cleanup("key rotation metrics", func(ctx context.Context) error {
		return nil
	})

	// Register observable gauges for key age and expiration
	_, err = krMeter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		if activeKey := manager.GetActiveKey(); activeKey != nil {
			ageInDays := int64(time.Since(activeKey.ActivatedAt).Hours() / 24)
			o.ObserveInt64(krCurrentKeyAge, ageInDays, metric.WithAttributes(partitionAttr))

			daysUntilExp := int64(time.Until(activeKey.ExpiresAt).Hours() / 24)
			o.ObserveInt64(krDaysUntilExpiration, daysUntilExp, metric.WithAttributes(partitionAttr))
		}
		return nil
	}, krCurrentKeyAge, krDaysUntilExpiration)
	if err != nil {
		slog.Warn("Failed to register key rotation metrics", "error", err)
	}

	// Cleanup on shutdown
	inst.cleanup("key rotation manager", func(ctx context.Context) error {
		return manager.Stop()
	})

	slog.Info("Key rotation service started",
		"partition", s.Partition,
		"enabled", config.Enabled,
		"rotation_interval_days", config.RotationIntervalDays)

	return nil
}
