package indexing

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

type DirectoryIndexer struct {
	batch      *database.Batch
	account    *url.URL
	indexValue *database.ValueAs[*DirectoryIndex]
}

func Directory(batch *database.Batch, account *url.URL) *DirectoryIndexer {
	v := database.AccountIndex(batch, account, newfn[DirectoryIndex](), "Directory")
	return &DirectoryIndexer{batch, account, v}
}

func loadMainIndex[T encoding.BinaryValue](x *database.ValueAs[T], notFound T) (T, error) {
	md, err := x.Get()
	switch {
	case err == nil:
		return md, nil
	case errors.Is(err, storage.ErrNotFound):
		return notFound, nil
	default:
		var zero T
		return zero, errors.Wrap(errors.StatusUnknownError, err)
	}
}

func (d *DirectoryIndexer) Count() (uint64, error) {
	md, err := loadMainIndex(d.indexValue, new(DirectoryIndex))
	if err != nil {
		return 0, err
	}
	return md.Count, nil
}

func (d *DirectoryIndexer) Get(i uint64) (*url.URL, error) {
	u, err := database.SubValue(d.indexValue, newfn[DirectoryEntry](), i).Get()
	if err != nil {
		return nil, err
	}
	return u.Account, nil
}

func (d *DirectoryIndexer) Put(u *url.URL) error {
	md, err := loadMainIndex(d.indexValue, new(DirectoryIndex))
	if err != nil {
		return err
	}

	err = database.SubValue[*DirectoryEntry](d.indexValue, nil, md.Count).Put(&DirectoryEntry{Account: u})
	if err != nil {
		return err
	}

	md.Count++
	return d.indexValue.Put(md)
}

type DataIndexer struct {
	batch      *database.Batch
	account    *database.Account
	indexValue *database.ValueAs[*DataIndex]
}

func Data(batch *database.Batch, account *url.URL) *DataIndexer {
	v := database.AccountIndex(batch, account, newfn[DataIndex](), "Data", "Metadata")
	return &DataIndexer{batch, batch.Account(account), v}
}

func (d *DataIndexer) Count() (uint64, error) {
	md, err := loadMainIndex(d.indexValue, new(DataIndex))
	if err != nil {
		return 0, err
	}
	return md.Count, nil
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
	v, err := database.SubValue(d.indexValue, newfn[DataEntry](), i).Get()
	if err != nil {
		return nil, err
	}
	return v.Hash[:], nil
}

// Transaction returns the transaction hash for the given entry hash.
func (d *DataIndexer) Transaction(entryHash []byte) ([]byte, error) {
	v, err := database.SubValue(d.indexValue, newfn[DataEntry](), entryHash).Get()
	if err != nil {
		return nil, err
	}
	return v.Hash[:], nil
}

func (d *DataIndexer) Put(entryHash, txnHash []byte) error {
	md, err := loadMainIndex(d.indexValue, new(DataIndex))
	if err != nil {
		return err
	}

	err = database.SubValue[*DataEntry](d.indexValue, nil, md.Count).Put(&DataEntry{Hash: *(*[32]byte)(entryHash)})
	if err != nil {
		return err
	}

	err = database.SubValue[*DataEntry](d.indexValue, nil, entryHash).Put(&DataEntry{Hash: *(*[32]byte)(txnHash)})
	if err != nil {
		return err
	}

	md.Count++
	return d.indexValue.Put(md)
}
