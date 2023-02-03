// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pkg

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"math/big"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type URL = url.URL

// Urlish indicates that the parameter must be convertable to a URL.
type Urlish = interface{}

// Keyish indicates that the parameter must be convertable to a key.
type Keyish = interface{}

// Numish indicates that the parameter must be convertable to a number.
type Numish = interface{}

type Output struct {
	Mime  string
	Value []byte
}

type Session struct {
	Filename string
	Output   func(...Output)
	Engine   Engine

	timestamp uint64
	submitted []*submittedTxn
}

type Engine interface {
	GetAccount(*URL) (protocol.Account, error)
	GetDirectory(*URL) ([]*url.URL, error)
	GetTransaction([32]byte) (*protocol.Transaction, error)
	Submit(*messaging.Envelope) (*protocol.TransactionStatus, error)
	WaitFor(txn [32]byte, delivered bool) ([]*protocol.TransactionStatus, []*protocol.Transaction, error)
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

func (s *Session) Assert(condition bool, format string, args ...interface{}) {
	if !condition {
		s.Abortf(format, args...)
	}
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

func intexp(x *big.Int, precision int) *big.Int {
	y := big.NewInt(10)
	y.Exp(y, big.NewInt(int64(precision)), nil)
	return x.Mul(x, y)
}

func bigfloat(v float64, precision int) *big.Int {
	e := intexp(big.NewInt(1), precision)
	x := new(big.Float)
	x.SetInt(e)               // We have to consider rounding errors with floating point math
	x.Mul(x, big.NewFloat(v)) // This scales v to the fixed point precision
	round := big.NewFloat(.9) // To adjust for lost precision, round to the nearest int
	if x.Sign() < 0 {         // Just to be safe, account for negative numbers
		round = big.NewFloat(-.9)
	}
	x.Add(x, round)
	x.Int(e)
	return e
}

func (s *Session) bigint(v Numish, precision int) *big.Int {
	switch v := v.(type) {
	case int:
		return intexp(big.NewInt(int64(v)), precision)
	case int8:
		return intexp(big.NewInt(int64(v)), precision)
	case int16:
		return intexp(big.NewInt(int64(v)), precision)
	case int32:
		return intexp(big.NewInt(int64(v)), precision)
	case int64:
		return intexp(big.NewInt(int64(v)), precision) //nolint:unconvert
	case uint:
		return intexp(big.NewInt(int64(v)), precision)
	case uint8:
		return intexp(big.NewInt(int64(v)), precision)
	case uint16:
		return intexp(big.NewInt(int64(v)), precision)
	case uint32:
		return intexp(big.NewInt(int64(v)), precision)
	case uint64:
		return intexp(big.NewInt(int64(v)), precision)

	case float32:
		return bigfloat(float64(v), precision)
	case float64:
		return bigfloat(float64(v), precision) //nolint:unconvert

	case string:
		parts := strings.Split(v, ".")
		if len(parts) > 2 {
			s.Abortf("Invalid number: %q\n", v)
		}
		x := new(big.Int)
		if _, ok := x.SetString(parts[0], 10); !ok {
			s.Abortf("Invalid number: %q\n", v)
		}
		intexp(x, precision)
		if len(parts) == 1 {
			return x
		}

		fpart := parts[1]
		if len(fpart) > precision {
			fpart = fpart[:precision]
		} else {
			fpart += strings.Repeat("0", precision-len(fpart))
		}
		y := new(big.Int)
		if _, ok := y.SetString(fpart, 10); !ok {
			s.Abortf("Invalid number: %q\n", v)
		}
		return x.Add(x, y)

	default:
		s.Abortf("Cannot convert %T to an integer", v)
		panic("unreachable")
	}
}
