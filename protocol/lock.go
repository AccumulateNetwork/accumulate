// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import "gitlab.com/accumulatenetwork/accumulate/pkg/errors"

type LockableAccount interface {
	Account
	GetLockHeight() uint64
	SetLockHeight(uint64) error
}

func (l *LiteTokenAccount) GetLockHeight() uint64 {
	return l.LockHeight
}

func (l *LiteTokenAccount) SetLockHeight(v uint64) error {
	if v < l.LockHeight {
		return errors.BadRequest.WithFormat("cannot reduce lockup period")
	}
	l.LockHeight = v
	return nil
}
