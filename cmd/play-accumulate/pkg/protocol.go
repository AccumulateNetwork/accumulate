package pkg

import (
	"crypto/ed25519"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (s *Session) GenerateKey(values ...interface{}) ed25519.PrivateKey {
	if len(values) == 0 {
		_, priv, err := ed25519.GenerateKey(nil)
		if err != nil {
			s.Abort(err)
		}
		return priv
	}

	hash := storage.MakeKey(values...)
	return ed25519.NewKeyFromSeed(hash[:])
}

func (s *Session) LiteAddress(keyOrEntry interface{}, args ...interface{}) *URL {
	var addr *url.URL
	var err error
	switch len(args) {
	case 0:
		switch v := keyOrEntry.(type) {
		case ed25519.PrivateKey, ed25519.PublicKey, []byte:
			return protocol.LiteAuthorityForKey(s.pubkey(v), protocol.SignatureTypeED25519)
		case protocol.DataEntry:
			addr, err = protocol.LiteDataAddress(protocol.ComputeLiteDataAccountId(v))
		case [][]byte:
			addr, err = protocol.LiteDataAddress(protocol.ComputeLiteDataAccountId(&protocol.AccumulateDataEntry{Data: v}))
		}
	case 1:
		addr, err = protocol.LiteTokenAddress(s.pubkey(keyOrEntry), s.url(args[0]).String(), protocol.SignatureTypeED25519)
	}

	if err != nil {
		s.Abort(err)
	}
	return addr

}
