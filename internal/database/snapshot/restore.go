// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package snapshot

import (
	"time"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
)

// RestoreVisitor is a visitor that restores accounts, transactions, and
// signatures.
type RestoreVisitor struct {
	logger logging.OptionalLogger
	db     database.Beginner
	start  time.Time
	batch  *database.Batch

	DisableWriteBatching bool
	CompressChains       bool
}

func Restore(db database.Beginner, file ioutil2.SectionReader, logger log.Logger) error {
	v := NewRestoreVisitor(db, logger)
	return Visit(file, v)
}

func NewRestoreVisitor(db database.Beginner, logger log.Logger) *RestoreVisitor {
	v := new(RestoreVisitor)
	v.logger.L = logger
	v.db = db
	return v
}

const markPointBatchSize = 10000

func (v *RestoreVisitor) VisitAccount(acct *Account, i int) error {
	// End of section
	if acct == nil {
		return v.end(i, "Restore accounts")
	}

	if v.CompressChains {
		for _, c := range acct.Chains {
			c.MarkPoints = nil
		}
	}

	// If an account's history has been preserved, it must be committed in a
	// separate batch
	var needsOwnBatch bool
	for _, c := range acct.Chains {
		if len(c.MarkPoints) > 0 {
			needsOwnBatch = true
			break
		}
	}

	err := v.visit(i, 10000, "Restore accounts", needsOwnBatch)
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	err = acct.Restore(v.batch)
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "restore %v: %w", acct.Url, err)
	}

	pos := map[string]int{}
	for _, c := range acct.Chains {
		_, err = acct.RestoreChainHead(v.batch, c)
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "restore %s chain head: %w", c.Name, err)
		}

		if len(c.MarkPoints) > 0 {
			pos[c.Name] = 0
		}
	}

	// Add chain entries 10000 at a time
	for len(pos) > 0 {
		record := v.batch.Account(acct.Url)
		for _, c := range acct.Chains {
			start, ok := pos[c.Name]
			if !ok {
				continue
			}

			end := len(c.MarkPoints)
			if end-start > markPointBatchSize {
				end = start + markPointBatchSize
				pos[c.Name] = end
			} else {
				delete(pos, c.Name)
			}

			mgr, err := record.ChainByName(c.Name)
			if err != nil {
				return errors.Format(errors.StatusUnknownError, "get %s chain: %w", c.Name, err)
			}
			err = mgr.Inner().RestoreMarkPointRange(c, start, end)
			if err != nil {
				return errors.Format(errors.StatusUnknownError, "restore %s chain mark points [%d,%d): %w", c.Name, start, end, err)
			}
		}

		err = v.refreshBatch()
		if err != nil {
			return errors.Wrap(errors.StatusUnknownError, err)
		}
	}

	// DO NOT reuse the existing record - it may have changed
	record := v.batch.Account(acct.Url)

	err = record.VerifyHash(acct.Hash[:])
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "restore %v: %w", acct.Url, err)
	}
	return nil
}

func (v *RestoreVisitor) VisitTransaction(txn *Transaction, i int) error {
	// End of section
	if txn == nil {
		return v.end(i, "Restore transactions")
	}

	err := v.visit(i, 10000, "Restore transactions", false)
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	err = txn.Restore(v.batch)
	return errors.Wrap(errors.StatusUnknownError, err)
}

func (v *RestoreVisitor) VisitSignature(sig *Signature, i int) error {
	// End of section
	if sig == nil {
		return v.end(i, "Restore signatures")
	}

	err := v.visit(i, 10000, "Restore signatures", false)
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	err = sig.Restore(v.batch)
	return errors.Wrap(errors.StatusUnknownError, err)
}

func (v *RestoreVisitor) visit(i, threshold int, msg string, force bool) error {
	if i == 0 {
		v.start = time.Now()
	}

	begin := force || v.batch == nil
	if i%threshold == 0 {
		if i > 0 {
			d := time.Since(v.start)
			v.logger.Info(msg, "module", "restore", "count", i, "duration", d, "per-second", float64(i)/d.Seconds())
		}
		if !v.DisableWriteBatching {
			begin = true
		}
	}
	if !begin {
		return nil
	}

	return v.refreshBatch()
}

func (v *RestoreVisitor) refreshBatch() error {
	if v.batch != nil {
		err := v.batch.Commit()
		if err != nil {
			return errors.Wrap(errors.StatusUnknownError, err)
		}
	}
	v.batch = v.db.Begin(true)
	return nil
}

func (v *RestoreVisitor) end(count int, msg string) error {
	if v.batch == nil {
		return nil
	}
	d := time.Since(v.start)
	v.logger.Info(msg, "module", "restore", "count", count, "duration", d, "per-second", float64(count)/d.Seconds())
	err := v.batch.Commit()
	v.batch = nil
	return errors.Wrap(errors.StatusUnknownError, err)
}
