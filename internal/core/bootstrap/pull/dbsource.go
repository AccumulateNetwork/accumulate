// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// DBSource adapts a *database.Batch into a Source by reading directly
// from the local database. Used in tests as a stand-in for the
// network: populate a reference DB, point Pull at a DBSource over it,
// and verify the target DB's leaf hashes match. The production
// adapter (api.Querier2-backed) lives in apisource.go and shares the
// same Source contract.
type DBSource struct {
	batch *database.Batch
}

// NewDBSource returns a Source that reads from batch. The batch must
// remain valid for the lifetime of the source — DBSource does not
// take ownership.
func NewDBSource(batch *database.Batch) *DBSource {
	return &DBSource{batch: batch}
}

func (s *DBSource) Main(_ context.Context, u *url.URL) (protocol.Account, error) {
	var acct protocol.Account
	if err := s.batch.Account(u).Main().GetAs(&acct); err != nil {
		return nil, err
	}
	return acct, nil
}

func (s *DBSource) DirectoryUrls(_ context.Context, u *url.URL) ([]*url.URL, error) {
	return s.batch.Account(u).Directory().Get()
}

func (s *DBSource) PendingIDs(_ context.Context, u *url.URL) ([]*url.TxID, error) {
	return s.batch.Account(u).Pending().Get()
}

func (s *DBSource) ChainNames(_ context.Context, u *url.URL) ([]string, error) {
	chains, err := s.batch.Account(u).Chains().Get()
	if err != nil {
		return nil, err
	}
	out := make([]string, len(chains))
	for i, c := range chains {
		out[i] = c.Name
	}
	return out, nil
}

func (s *DBSource) ChainEntries(_ context.Context, u *url.URL, chainName string) ([][]byte, error) {
	c, err := s.batch.Account(u).ChainByName(chainName)
	if err != nil {
		return nil, err
	}
	head, err := c.Head().Get()
	if err != nil {
		return nil, err
	}
	out := make([][]byte, head.Count)
	for i := int64(0); i < head.Count; i++ {
		e, err := c.Entry(i)
		if err != nil {
			return nil, err
		}
		out[i] = e
	}
	return out, nil
}
