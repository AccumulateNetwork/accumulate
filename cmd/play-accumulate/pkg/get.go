package pkg

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
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

func (s *Session) GetAccount(query interface{}) protocol.Account {
	url := s.url(query)
	acct, err := s.Engine.GetAccount(url)
	if err != nil {
		s.Abortf("Get %v: %v", url, err)
	}
	return acct
}

func (s *Session) GetAccountAs(query interface{}, target interface{}) {
	account := s.GetAccount(query)
	err := encoding.SetPtr(account, target)
	if err != nil {
		s.Abortf("Get %v: %v", account.GetUrl(), err)
	}
}
