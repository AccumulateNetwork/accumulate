// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build dagbft

package run

import (
	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// Snapshot service is not supported in DAG-BFT mode

func (s *SnapshotService) Verify() error {
	return errors.BadRequest.With("snapshot service is not supported in dagbft mode")
}

func (s *SnapshotService) Requires() []ioc.Requirement {
	return nil
}

func (s *SnapshotService) Provides() []ioc.Provided {
	return nil
}

func (s *SnapshotService) start(inst *Instance) error {
	return errors.BadRequest.With("snapshot service is not supported in dagbft mode")
}
