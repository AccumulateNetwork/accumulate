package pkg

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"io"
	"math/big"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type URL = url.URL

// Urlish indicates that the parameter must be convertable to a URL.
type Urlish = interface{}

// Keyish indicates that the parameter must be convertable to a key.
type Keyish = interface{}

// Intish indicates that the parameter must be convertable to an integer.
type Intish = interface{}

type Session struct {
	Filename string
	Stdout   io.Writer
	Stderr   io.Writer
	Engine   Engine

	timestamp uint64
	submitted []*submittedTxn
}

type Engine interface {
	GetAccount(*URL) (protocol.Account, error)
	GetTransaction([32]byte) (*protocol.Transaction, error)
	Submit(*protocol.Envelope) (*protocol.TransactionStatus, error)
	WaitFor([32]byte) (*protocol.TransactionStatus, *protocol.Transaction, error)
}

type Abort struct {
	Value interface{}
}

func (s *Session) Abort(value interface{}) {
	panic(Abort{value})
}

func (s *Session) Abortf(format string, args ...interface{}) {
	s.Abort(fmt.Errorf(format, args...))
}

func (s *Session) url(v Urlish) *URL {
	var str string
	switch v := v.(type) {
	case URL:
		return &v
	case *URL:
		return v
	case string:
		str = v
	case fmt.Stringer:
		str = v.String()
	default:
		s.Abortf("Cannot convert %T into a URL", v)
	}

	u, err := url.Parse(str)
	if err != nil {
		s.Abortf("Invalid URL %q: %v", str, err)
	}
	return u
}

func (s *Session) pubkey(key Keyish) []byte {
	switch key := key.(type) {
	case ed25519.PrivateKey:
		return key[32:]
	case ed25519.PublicKey:
		return key
	case []byte:
		return key
	default:
		s.Abortf("Unknown key type: %T", key)
		panic("unreachable")
	}
}

func (s *Session) pubkeyhash(key Keyish) []byte {
	hash := sha256.Sum256(s.pubkey(key))
	return hash[:]
}

func (s *Session) privkey(key Keyish) []byte {
	switch key := key.(type) {
	case ed25519.PrivateKey:
		return key
	case []byte:
		return key
	default:
		s.Abortf("Unknown private key type: %T", key)
		panic("unreachable")
	}
}

func (s *Session) bigint(v Intish) *big.Int {
	switch v := v.(type) {
	case int:
		return big.NewInt(int64(v))
	case int8:
		return big.NewInt(int64(v))
	case int16:
		return big.NewInt(int64(v))
	case int32:
		return big.NewInt(int64(v))
	case int64:
		return big.NewInt(int64(v))
	case uint:
		return big.NewInt(int64(v))
	case uint8:
		return big.NewInt(int64(v))
	case uint16:
		return big.NewInt(int64(v))
	case uint32:
		return big.NewInt(int64(v))
	case uint64:
		return big.NewInt(int64(v))
	case string:
		u := new(big.Int)
		_, ok := u.SetString(v, 10)
		if !ok {
			s.Abortf("Invalid number: %q", v)
		}
		return u
	default:
		s.Abortf("Cannot convert %T to an integer", v)
		panic("unreachable")
	}
}
