package pkg

import (
	"crypto/ed25519"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
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

func (s *Session) LiteAddress(key interface{}, token Urlish) *URL {
	addr, err := protocol.LiteTokenAddress(s.pubkey(key), s.url(token).String(), protocol.SignatureTypeED25519)
	if err != nil {
		s.Abortf("Invalid token URL: %v", err)
	}
	return addr
}
