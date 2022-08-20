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
		return errors.BadRequest.Format("cannot reduce lockup period")
	}
	l.LockHeight = v
	return nil
}
