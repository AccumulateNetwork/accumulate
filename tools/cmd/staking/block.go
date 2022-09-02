package main

import (
	"net/url"
	"time"
)

type Block struct {
	MajorHeight int64
	MinorHeight int64
	Timestamp   time.Time
	Accounts    []*Account
}

// GetAccount
// Look through the accounts modified in this block, and return the
// account if it has been modified.
func (b *Block) GetAccount(AccountURL *url.URL)*Account {
	for _,acc := range b.Accounts{
		if acc.URL.String()==AccountURL.String(){
			return acc
		}
	}
	return nil
}
