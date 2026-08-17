// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"path/filepath"

	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type partOpts struct {
	*CoreValidatorConfiguration
	ID               string
	Type             protocol.PartitionType
	Genesis          string
	Dir              string
	BootstrapPeers   []multiaddr.Multiaddr
	MetricsNamespace string
	NumWorkers       *int64
}

func (p partOpts) apply(cfg *Config) error {
	// Consensus (DAG-BFT)
	addService(cfg,
		&DAGBFTService{
			NodeDir:      p.Dir,
			ValidatorKey: p.ValidatorKey,
			Genesis:      p.Genesis,
			Partition: &protocol.PartitionInfo{
				ID:   p.ID,
				Type: p.Type,
			},
			EnableHealing:        p.EnableHealing,
			EnableDirectDispatch: p.EnableDirectDispatch,
			MaxEnvelopesPerBlock: p.MaxEnvelopesPerBlock,
			NumWorkers:           p.NumWorkers,
			DAGGCDepth:           p.DagGcDepth,
		},
		func(s *DAGBFTService) string { return s.Partition.ID })

	// Storage
	if !haveService2(cfg, p.ID, func(s *StorageService) string { return s.Name }, nil) {
		storage, err := sStorage.New(*p.StorageType)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}

		storage.setPath(filepath.Join(p.Dir, "data", "accumulate.db"))
		cfg.Services = append(cfg.Services, &StorageService{Name: p.ID, Storage: storage})
	}

	// Services
	addService(cfg, &Querier{Partition: p.ID}, func(s *Querier) string { return s.Partition })
	addService(cfg, &NetworkService{Partition: p.ID}, func(s *NetworkService) string { return s.Partition })
	addService(cfg, &MetricsService{Partition: p.ID}, func(s *MetricsService) string { return s.Partition })
	addService(cfg, &EventsService{Partition: p.ID}, func(s *EventsService) string { return s.Partition })

	// Key rotation service (with default config)
	addService(cfg, &KeyRotationService{
		Partition:    p.ID,
		ValidatorKey: p.ValidatorKey,
		Config: &KeyRotationConfig{
			Enabled:              true,
			RotationIntervalDays: 90,
			GracePeriodDays:      7,
			WarningPeriodDays:    7,
			AuditDirectory:       filepath.Join(p.Dir, "key-rotation-audit"),
			AuditRetentionDays:   365,
		},
	}, func(s *KeyRotationService) string { return s.Partition })

	return nil
}
