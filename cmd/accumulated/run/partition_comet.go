// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build !dagbft

package run

import (
	"path/filepath"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
)

func (p partOpts) addSnapshotService(cfg *Config) {
	// Snapshots; capture on every major block
	if *p.EnableSnapshots {
		addService(cfg,
			&SnapshotService{
				Partition: p.ID,
				Directory: filepath.Join(p.Dir, "snapshots"),
				Schedule:  network.MustParseCron("* * * * *")},
			func(s *SnapshotService) string { return s.Partition })
	}
}
