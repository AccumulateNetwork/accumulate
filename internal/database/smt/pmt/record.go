// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pmt

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

var _ record.Record = (*Manager)(nil)

func (m *Manager) Resolve(key record.Key) (record.Record, record.Key, error) {
	// Finish any pending writes prior to resolving a block
	err := m.Bpt.Update()
	if err != nil {
		return nil, nil, errors.UnknownError.Wrap(err)
	}

	return m.model.Resolve(key)
}

func (m *Manager) IsDirty() bool {
	return len(m.Bpt.DirtyMap) > 0 || m.model.IsDirty()
}

func (m *Manager) Commit() error {
	err := m.Bpt.Update()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	err = m.model.Commit()
	return errors.UnknownError.Wrap(err)
}
