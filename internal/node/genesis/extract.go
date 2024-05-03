// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package genesis

import (
	"fmt"
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
	Main   protocol.Account
	Heads  map[string]*merkle.State
	States map[string][]*merkle.State
}

func Extract(db *coredb.Database, snap ioutil2.SectionReader, shouldKeep func(*url.URL) (bool, error)) (map[[32]byte]*AccountData, error) {
	accounts := map[[32]byte]*AccountData{}
	data := func(u *url.URL) *AccountData {
		data, ok := accounts[u.AccountID32()]
		if ok {
			return data
		}

		data = new(AccountData)
		data.Url = u
		data.Heads = map[string]*merkle.State{}
		data.States = map[string][]*merkle.State{}
		accounts[u.AccountID32()] = data
		return data
	}

	// Restore accounts
	seen := map[[32]byte]bool{}
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

				// If we haven't seen this account yet, route it
				want, ok := seen[u.AccountID32()]
				if !ok {
					var err error
					want, err = shouldKeep(u)
					if err != nil {
						return false, errors.UnknownError.Wrap(err)
					}
					seen[u.AccountID32()] = want
				}

				switch e.Key.Get(2) {
				case "Pending":
					// Do not preserve pending transactions
					return false, nil

				case "Main":
					if !want {
						break
					}

					// Record the account main state
					acct, _ := protocol.UnmarshalAccount(e.Value)
					data(u).Main = acct

				case "SignatureChain":
					// Don't preserve the signature chain
					return false, nil

				case "MainChain", "ScratchChain":
					if !want {
						break
					}

					// Track chains
					//
					// TODO This is fragile - it breaks if we add new chain
					// types. Is there a decent way to programmatically detect
					// if `v` is a child of a chain?
					data := data(u)
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

					case "Index":
						// Don't care
					default:
						fmt.Println("What about", e.Key)
					}
				}

				return want, nil

			default:
				// Ignore everything else on this pass
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
