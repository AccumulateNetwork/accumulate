package snapshot

import (
	"io"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// FullCollect collects a snapshot including additional records required for a
// fully-functioning node.
func FullCollect(batch *database.Batch, file io.WriteSeeker, network *config.Describe) error {
	w, err := Collect(batch, file, func(account *database.Account) (bool, error) {
		// Preserve history for DN/BVN ADIs
		_, ok := protocol.ParsePartitionUrl(account.Url())
		return ok, nil
	})
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	// Collect anchors
	txnHashes := new(HashSet)
	record := batch.Account(network.AnchorPool())
	err = txnHashes.CollectFromChain(record.AnchorSequenceChain())
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	err = w.CollectTransactions(batch, txnHashes.Hashes, nil)
	return errors.Wrap(errors.StatusUnknownError, err)
}

// FullRestore restores the snapshot and rebuilds indices.
func FullRestore(db database.Beginner, file ioutil2.SectionReader, logger log.Logger, network *config.Describe) error {
	err := Restore(db, file, logger)
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	batch := db.Begin(true)
	defer batch.Discard()

	// Rebuild the synthetic transaction index index
	record := batch.Account(network.Synthetic())
	synthIndexChain, err := record.MainChain().Index().Get()
	if err != nil {
		return errors.Format(errors.StatusInternalError, "load synthetic index chain: %w", err)
	}

	entries, err := synthIndexChain.Entries(0, synthIndexChain.Height())
	if err != nil {
		return errors.Format(errors.StatusInternalError, "load synthetic index chain entries: %w", err)
	}

	for i, data := range entries {
		entry := new(protocol.IndexEntry)
		err = entry.UnmarshalBinary(data)
		if err != nil {
			return errors.Format(errors.StatusInternalError, "unmarshal synthetic index chain entry %d: %w", i, err)
		}

		err = batch.SystemData(network.PartitionId).SyntheticIndexIndex(entry.BlockIndex).Put(uint64(i))
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "store synthetic transaction index index %d for block: %w", i, err)
		}
	}

	err = batch.Commit()
	return errors.Wrap(errors.StatusUnknownError, err)
}
