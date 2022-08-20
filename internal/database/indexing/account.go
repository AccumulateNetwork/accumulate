package indexing

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type DirectoryIndexer struct {
	*record.Set[*url.URL]
}

func Directory(batch *database.Batch, account *url.URL) *DirectoryIndexer {
	return &DirectoryIndexer{batch.Account(account).Directory()}
}

func (d *DirectoryIndexer) Get(i uint64) (*url.URL, error) {
	value, err := d.Set.Get()
	return value[i], err
}

func (d *DirectoryIndexer) Count() (uint64, error) {
	uarr, err := d.Set.Get()
	return uint64(len(uarr)), err
}

type DataIndexer struct {
	batch *database.Batch
	*database.AccountData
}

func Data(batch *database.Batch, account *url.URL) *DataIndexer {
	return &DataIndexer{batch, batch.Account(account).Data()}
}

func (d *DataIndexer) Count() (uint64, error) {
	v, err := d.AccountData.Entry().Count()
	return uint64(v), err
}

// GetLatest returns the last entry.
func (d *DataIndexer) GetLatest() (index uint64, entryHash, txnHash []byte, err error) {
	count, err := d.Count()
	if err != nil {
		return 0, nil, nil, err
	}

	if count == 0 {
		return 0, nil, nil, errors.NotFound("empty")
	}

	entryHash, err = d.Entry(count - 1)
	if err != nil {
		return 0, nil, nil, err
	}

	txnHash, err = d.Transaction(entryHash)
	if err != nil {
		return 0, nil, nil, err
	}

	return count - 1, entryHash, txnHash, nil
}

func GetDataEntry(batch *database.Batch, txnHash []byte) (protocol.DataEntry, error) {
	state, err := batch.Transaction(txnHash).GetState()
	if err != nil {
		return nil, err
	}

	switch txn := state.Transaction.Body.(type) {
	case *protocol.WriteData:
		return txn.Entry, nil
	case *protocol.WriteDataTo:
		return txn.Entry, nil
	case *protocol.SyntheticWriteData:
		return txn.Entry, nil
	case *protocol.SystemWriteData:
		return txn.Entry, nil
	default:
		return nil, errors.Format(errors.StatusInternalError, "invalid data transaction: expected %v or %v, got %v", protocol.TransactionTypeWriteData, protocol.TransactionTypeWriteDataTo, state.Transaction.Body.Type())
	}
}

func (d *DataIndexer) GetLatestEntry() (protocol.DataEntry, error) {
	_, _, txnHash, err := d.GetLatest()
	if err != nil {
		return nil, err
	}

	return GetDataEntry(d.batch, txnHash)
}

// Entry returns the entry hash for the given index.
func (d *DataIndexer) Entry(i uint64) ([]byte, error) {
	v, err := d.AccountData.Entry().Get(int(i))
	if err != nil {
		return nil, err
	}
	return v[:], nil
}

// Transaction returns the transaction hash for the given entry hash.
func (d *DataIndexer) Transaction(entryHash []byte) ([]byte, error) {
	v, err := d.AccountData.Transaction(*(*[32]byte)(entryHash)).Get()
	if err != nil {
		return nil, err
	}
	return v[:], nil
}

func (d *DataIndexer) Put(entryHash, txnHash []byte) error {
	err := d.AccountData.Entry().Put(*(*[32]byte)(entryHash))
	if err != nil {
		return errors.Wrap(errors.StatusUnknownError, err)
	}

	err = d.AccountData.Transaction(*(*[32]byte)(entryHash)).Put(*(*[32]byte)(txnHash))
	return errors.Wrap(errors.StatusUnknownError, err)
}
