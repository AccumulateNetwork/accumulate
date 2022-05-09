package indexing

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type DirectoryIndex struct {
	account *database.Account
}

func Directory(batch *database.Batch, account *url.URL) *DirectoryIndex {
	return &DirectoryIndex{batch.Account(account)}
}

func (d *DirectoryIndex) Count() (uint64, error) {
	md := new(protocol.DirectoryIndexMetadata)
	err := d.account.Index("Directory", "Metadata").GetAs(md)
	if err != nil {
		return 0, err
	}
	return md.Count, nil
}

func (d *DirectoryIndex) Get(i uint64) (*url.URL, error) {
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

func (d *DirectoryIndex) Put(u *url.URL) error {
	md := new(protocol.DirectoryIndexMetadata)
	err := d.account.Index("Directory", "Metadata").GetAs(md)
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
