package snapshot

import (
	"time"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
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

const chainEntryBatchSize = 10000

func (v *RestoreVisitor) VisitAccount(acct *Account, i int) error {
	// End of section
	if acct == nil {
		return v.end(i, "Restore accounts")
	}

	if v.CompressChains {
		for _, c := range acct.Chains {
			ms := new(managed.MerkleState)
			ms.Count = int64(c.Count)
			ms.Pending = c.Pending
			for _, v := range c.Entries {
				ms.AddToMerkleTree(v)
			}
			c.Count = uint64(ms.Count)
			c.Pending = ms.Pending
			c.Entries = nil
		}
	}

	// If an account's history has been preserved, it must be committed in a
	// separate batch
	var needsOwnBatch bool
	for _, c := range acct.Chains {
		if len(c.Entries) > 0 {
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
		return errors.Format(errors.StatusUnknownError, "restore %v: %w", acct.Main.GetUrl(), err)
	}

	record := v.batch.Account(acct.Main.GetUrl())
	chains := map[string][][]byte{}
	for _, c := range acct.Chains {
		mgr, err := record.GetChainByName(c.Name)
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "store %s chain head: %w", c.Name, err)
		}
		err = mgr.RestoreHead(&managed.MerkleState{Count: int64(c.Count), Pending: c.Pending})
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "store %s chain head: %w", c.Name, err)
		}

		if len(c.Entries) > 0 {
			chains[c.Name] = c.Entries
		}
	}

	// Add chain entries 10000 at a time
	for len(chains) > 0 {
		next := map[string][][]byte{}
		record := v.batch.Account(acct.Main.GetUrl())
		for name, entries := range chains {
			if len(entries) > chainEntryBatchSize {
				entries, next[name] = entries[:chainEntryBatchSize], entries[chainEntryBatchSize:]
			}
			mgr, err := record.GetChainByName(name)
			if err != nil {
				return errors.Format(errors.StatusUnknownError, "store %s chain head: %w", name, err)
			}
			for _, entry := range entries {
				err := mgr.AddEntry(entry, false)
				if err != nil {
					return errors.Format(errors.StatusUnknownError, "store %s chain entry: %w", name, err)
				}
			}
		}
		chains = next

		err = v.refreshBatch()
		if err != nil {
			return errors.Wrap(errors.StatusUnknownError, err)
		}
	}

	// DO NOT reuse the existing record - it may have changed
	record = v.batch.Account(acct.Main.GetUrl())

	err = record.VerifyHash(acct.Hash[:])
	if err != nil {
		return errors.Format(errors.StatusUnknownError, "restore %v: %w", acct.Main.GetUrl(), err)
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
		d := time.Since(v.start)
		v.logger.Info(msg, "module", "restore", "count", i, "duration", d, "per-second", float64(i)/d.Seconds())
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
