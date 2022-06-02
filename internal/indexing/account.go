package indexing

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

type DirectoryIndexer struct {
	account *database.Account
}

func Directory(batch *database.Batch, account *url.URL) *DirectoryIndexer {
	return &DirectoryIndexer{batch.Account(account)}
}

func (d *DirectoryIndexer) index() (*DirectoryIndex, error) {
	md := new(DirectoryIndex)
	err := d.account.Index("Directory", "Metadata").GetAs(md)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, err
	}
	return md, nil
}

func (d *DirectoryIndexer) Count() (uint64, error) {
	md, err := d.index()
	if err != nil {
		return 0, err
	}
	return md.Count, nil
}

func (d *DirectoryIndexer) Get(i uint64) (*url.URL, error) {
	b, err := d.account.Index("Directory", i).Get()
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(string(b))
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (d *DirectoryIndexer) Put(u *url.URL) error {
	md, err := d.index()
	if err != nil {
		return err
	}

	err = d.account.Index("Directory", md.Count).Put([]byte(u.String()))
	if err != nil {
		return err
	}

	md.Count++
	return d.account.Index("Directory", "Metadata").PutAs(md)
}

type DataIndexer struct {
	batch   *database.Batch
	account *database.Account
}

func Data(batch *database.Batch, account *url.URL) *DataIndexer {
	return &DataIndexer{batch, batch.Account(account)}
}

func (d *DataIndexer) index() (*DataIndex, error) {
	md := new(DataIndex)
	err := d.account.Index("Data", "Metadata").GetAs(md)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, err
	}
	return md, nil
}

func (d *DataIndexer) Count() (uint64, error) {
	md, err := d.index()
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
	entryHash, err := d.account.Index("Data", i).Get()
	if err != nil {
		return nil, err
	}
	return entryHash, nil
}

// Transaction returns the transaction hash for the given entry hash.
func (d *DataIndexer) Transaction(entryHash []byte) ([]byte, error) {
	txnHash, err := d.account.Index("Data", entryHash).Get()
	if err != nil {
		return nil, err
	}
	return txnHash, nil
}

func (d *DataIndexer) Put(entryHash, txnHash []byte) error {
	md, err := d.index()
	if err != nil {
		return err
	}

	err = d.account.Index("Data", md.Count).Put(entryHash)
	if err != nil {
		return err
	}

	err = d.account.Index("Data", entryHash).Put(txnHash)
	if err != nil {
		return err
	}

	md.Count++
	return d.account.Index("Data", "Metadata").PutAs(md)
}
