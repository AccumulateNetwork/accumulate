// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func (c *AccountTransaction) Vote(authority *url.URL) record.Value[[32]byte] {
	return &accountTransactionVote{c.getVote(authority), c, authority}
}

// accountTransactionVote is a wrapper for the vote [record.Value] that records
// the authority in Voters.
type accountTransactionVote struct {
	record.Value[[32]byte]
	parent    *AccountTransaction
	authority *url.URL
}

// Put updates the vote and adds the authority to Voters.
func (v *accountTransactionVote) Put(hash [32]byte) error {
	err := v.Value.Put(hash)
	if err != nil {
		return err
	}

	err = v.parent.Voters().Add(v.authority)
	return errors.UnknownError.Wrap(err)
}
