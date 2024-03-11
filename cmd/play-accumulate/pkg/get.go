// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pkg

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (s *Session) Get(query interface{}, subquery ...interface{}) interface{} {
	switch hash := query.(type) {
	case []byte:
		txn, err := s.Engine.GetTransaction(*(*[32]byte)(hash))
		if err != nil {
			s.Abortf("Get %X: %v", hash, err)
		}
		return txn
	case [32]byte:
		txn, err := s.Engine.GetTransaction(hash)
		if err != nil {
			s.Abortf("Get %X: %v", hash, err)
		}
		return txn
	case *[32]byte:
		txn, err := s.Engine.GetTransaction(*hash)
		if err != nil {
			s.Abortf("Get %X: %v", hash, err)
		}
		return txn
	}

	return s.GetAccount(query)
}

func (s *Session) GetAs(u interface{}, query interface{}, subquery ...interface{}) {
	v := s.Get(query, subquery...)
	err := encoding.SetPtr(v, &u)
	if err != nil {
		s.Abortf("Get %v(%v): %v", query, subquery, err)
	}
}

func (s *Session) GetAccount(query Urlish) protocol.Account {
	url := s.url(query)
	acct, err := s.Engine.GetAccount(url)
	if err != nil {
		s.Abortf("Get %v: %v", url, err)
	}
	return acct
}

func (s *Session) GetDirectory(query Urlish) []*URL {
	url := s.url(query)
	urls, err := s.Engine.GetDirectory(url)
	if err != nil {
		s.Abortf("Get directory %v: %v", url, err)
	}
	return urls
}

func (s *Session) GetAccountAs(query Urlish, target interface{}) {
	account := s.GetAccount(query)
	err := encoding.SetPtr(account, target)
	if err != nil {
		s.Abortf("Get %v: %v", account.GetUrl(), err)
	}
}

func (s *Session) TryGetAccount(query Urlish) (protocol.Account, bool) {
	url := s.url(query)
	acct, err := s.Engine.GetAccount(url)
	if err != nil {
		return nil, false
	}
	return acct, true
}

func (s *Session) TryGetDirectory(query Urlish) ([]*URL, bool) {
	url := s.url(query)
	urls, err := s.Engine.GetDirectory(url)
	if err != nil {
		return nil, false
	}
	return urls, true
}

func (s *Session) TryGetAccountAs(query Urlish, target interface{}) bool {
	account, ok := s.TryGetAccount(query)
	return ok && encoding.SetPtr(account, target) == nil
}
