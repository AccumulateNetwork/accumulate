// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package genesis

import (
	"io"

	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type AccountData struct {
	Url    *url.URL
	Keep   bool
	Main   protocol.Account
	States map[string][]*merkle.State
}

func Extract(db *coredb.Database, snap ioutil2.SectionReader, shouldKeep func(*url.URL) bool) (map[[32]byte]*AccountData, error) {
	accounts := map[[32]byte]*AccountData{}
	data := func(u *url.URL) *AccountData {
		data, ok := accounts[u.AccountID32()]
		if ok {
			return data
		}

		data = new(AccountData)
		data.Keep = shouldKeep(u)
		data.Url = u
		data.States = map[string][]*merkle.State{}
		accounts[u.AccountID32()] = data
		return data
	}

	// Restore accounts
	err := coredb.Restore(db, snap, &coredb.RestoreOptions{
		BatchRecordLimit: 50_000,
		SkipHashCheck:    true,
		Predicate: func(e *snapshot.RecordEntry) (bool, error) {
			switch e.Key.Get(0) {
			case "Account":
				// Skip the faucet
				u := e.Key.Get(1).(*url.URL)
				if protocol.FaucetUrl.Equal(u) {
					return false, nil
				}

				// Skip ACME
				if protocol.AcmeUrl().Equal(u) {
					return false, nil
				}

				// Skip system accounts
				if _, ok := protocol.ParsePartitionUrl(u); ok {
					return false, nil
				}

				switch e.Key.Get(2) {
				case "Pending":
					// Do not preserve pending transactions
					return false, nil

				case "Main":
					// Record the account main state
					acct, _ := protocol.UnmarshalAccount(e.Value)
					data(u).Main = acct

				case "SignatureChain":
					// Don't preserve the signature chain
					return false, nil

				case "MainChain", "ScratchChain":
					data := data(u)
					if !data.Keep {
						break
					}

					// Track chains
					//
					// TODO This is fragile - it breaks if we add new chain
					// types. Is there a decent way to programmatically detect
					// if `v` is a child of a chain?
					switch e.Key.Get(3) {
					case "States":
						// Discard mark points unless it's a data account
						switch data.Main.(type) {
						case *protocol.DataAccount,
							*protocol.LiteDataAccount:
							// Ok
						default:
							return false, nil
						}

						fallthrough
					case "Head":
						s := new(merkle.State)
						err := s.UnmarshalBinary(e.Value)
						if err != nil {
							return false, errors.InternalError.WithFormat("decode %v: %w", e.Key, err)
						}

						name := e.Key.Get(2).(string)
						data.States[name] = append(data.States[name], s)
					}
				}

				return data(u).Keep, nil

			default:
				// Ignore everything that isn't an Account on this pass. So,
				// transactions/signatures/etc aka messages. And other system
				// data.
				return false, nil
			}
		},
	})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// Restore messages
	hashes := map[[32]byte]bool{}
	for _, cd := range accounts {
		for _, states := range cd.States {
			for _, s := range states {
				for _, h := range s.HashList {
					hashes[*(*[32]byte)(h)] = true
				}
			}
		}
	}

	_, err = snap.Seek(0, io.SeekStart)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	err = coredb.Restore(db, snap, &coredb.RestoreOptions{
		BatchRecordLimit: 50_000,
		SkipHashCheck:    true,
		Predicate: func(e *snapshot.RecordEntry) (bool, error) {
			switch e.Key.Get(0) {
			case "Transaction", "Message":
				h := e.Key.Get(1).([32]byte)
				return hashes[h], nil

			default:
				// Ignore everything else on this pass
				return false, nil
			}
		},
	})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return accounts, nil
}
