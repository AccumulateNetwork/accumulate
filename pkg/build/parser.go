// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package build

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	tmed25519 "github.com/cometbft/cometbft/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Errors []error

func (e Errors) Error() string {
	if len(e) == 0 {
		return e[0].Error()
	}
	var errs []string
	for _, e := range e {
		errs = append(errs, e.Error())
	}
	return strings.Join(errs, "; ")
}

type parser struct {
	errs []error
}

func (p *parser) ok() bool {
	return len(p.errs) == 0
}

func (p *parser) err() error {
	switch len(p.errs) {
	case 0:
		return nil
	case 1:
		return p.errs[0]
	default:
		return Errors(p.errs)
	}
}

func (p *parser) record(err ...error) {
	errs := make([]error, 0, len(p.errs)+len(err))
	errs = append(errs, p.errs...)
	errs = append(errs, err...)
	p.errs = errs
}

func (p *parser) errorf(code errors.Status, format string, args ...interface{}) {
	p.record(code.Skip(1).WithFormat(format, args...))
}

func (p *parser) parseDataEntry(v ...any) protocol.DataEntry {
	if len(v) == 1 {
		if e, ok := v[0].(protocol.DataEntry); ok {
			return e
		}
	}

	return &protocol.AccumulateDataEntry{Data: p.parseDataEntryParts(v...)}
}

func (p *parser) parseBytes(v any) []byte {
	switch v := v.(type) {
	case string:
		return []byte(v)
	case []byte:
		return v
	case [32]byte:
		return v[:]
	default:
		p.errorf(errors.BadRequest, "cannot convert %T to bytes", v)
	}
	return nil
}

func (p *parser) parseDataEntryParts(v ...any) [][]byte {
	var parts [][]byte
	for _, v := range v {
		switch v := v.(type) {
		case []string:
			for _, v := range v {
				parts = append(parts, []byte(v))
			}
		case [][]byte:
			parts = append(parts, v...)
		case [][32]byte:
			for _, v := range v {
				v := v
				parts = append(parts, v[:])
			}
		default:
			parts = append(parts, p.parseBytes(v))
		}
	}
	return parts
}

func (p *parser) parseUrl(v any, path ...string) *url.URL {
	var str string
	switch v := v.(type) {
	case url.URL:
		return v.JoinPath(path...)
	case *url.URL:
		return v.JoinPath(path...)
	case interface{ Url() *url.URL }:
		return v.Url().JoinPath(path...)
	case string:
		str = v
	case fmt.Stringer:
		str = v.String()
	default:
		p.errorf(errors.BadRequest, "cannot convert %T to a URL", v)
		return nil
	}

	u, err := url.Parse(str)
	if err != nil {
		p.errorf(errors.BadRequest, "invalid url %q: %w", v, err)
		return nil
	}
	return u.JoinPath(path...)
}

func (p *parser) parseHash32(v any) [32]byte {
	switch v := v.(type) {
	case []byte:
		if len(v) != 32 {
			p.errorf(errors.BadRequest, "invalid hash length: want 32, got %d", len(v))
			return [32]byte{}
		}
		return *(*[32]byte)(v)
	case [32]byte:
		return v
	case *[32]byte:
		return *v
	default:
		p.errorf(errors.BadRequest, "cannot convert %T to a hash", v)
		return [32]byte{}
	}
}

func (p *parser) parseHash(v any) []byte {
	switch v := v.(type) {
	case []byte:
		if len(v) != 32 && len(v) != 20 {
			p.errorf(errors.BadRequest, "invalid hash length: want 32, got %d", len(v))
			return nil
		}
		return v
	case [32]byte:
		return v[:]
	case *[32]byte:
		return (*v)[:]
	default:
		p.errorf(errors.BadRequest, "cannot convert %T to a hash", v)
		return nil
	}
}

func (p *parser) parseKey(key any, typ protocol.SignatureType, private bool) address.Address {
	switch key := key.(type) {
	case address.Address:
		return key
	case ed25519.PrivateKey:
		return address.FromED25519PrivateKey(key)
	case ed25519.PublicKey:
		return address.FromED25519PublicKey(key)
	case tmed25519.PrivKey:
		return address.FromED25519PrivateKey(key)
	case tmed25519.PubKey:
		return address.FromED25519PublicKey(key)
	case *rsa.PrivateKey:
		return address.FromRSAPrivateKey(key)
	case *rsa.PublicKey:
		return address.FromRSAPublicKey(key)
	case *ecdsa.PrivateKey:
		return address.FromEcdsaPrivateKey(key)
	case *ecdsa.PublicKey:
		return address.FromEcdsaPublicKeyAsPKIX(key)
	case []byte:
		if typ == protocol.SignatureTypeUnknown {
			if len(key) == 32 || len(key) == 64 {
				typ = protocol.SignatureTypeED25519
			} else {
				return &address.Unknown{Value: key}
			}
		}

		// If the key should be assumed to be private or the key is 64 bytes and
		// the type is ED25519, process it as a private key
		if private || len(key) == ed25519.PrivateKeySize &&
			(typ == protocol.SignatureTypeED25519 ||
				typ == protocol.SignatureTypeLegacyED25519 ||
				typ == protocol.SignatureTypeRCD1) {
			return address.FromPrivateKeyBytes(key, typ)
		}

		return &address.PublicKey{
			Type: typ,
			Key:  key,
		}

	case string:
		addr, err := address.Parse(key)
		if err != nil {
			p.record(err)
			return nil
		}
		if uk, ok := addr.(*address.Unknown); ok && typ != protocol.SignatureTypeUnknown {
			return &address.PublicKeyHash{
				Type: typ,
				Hash: uk.Value,
			}
		}
		return addr
	default:
		p.errorf(errors.BadRequest, "unsupported key type %T", key)
		return &address.Unknown{}
	}
}

func (p *parser) hashKey(addr address.Address) []byte {
	hash, ok := addr.GetPublicKeyHash()
	if ok {
		return hash
	}
	p.errorf(errors.BadRequest, "cannot determine hash for %v", addr)
	return nil
}

func (p *parser) parseAmount(v any, precision uint64) *big.Int {
	switch v := v.(type) {
	case *big.Int:
		return v // Assume the caller has already handled precision
	case big.Int:
		return &v

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
			p.errorf(errors.BadRequest, "invalid number: %q", v)
			return nil
		}
		x := new(big.Int)
		if _, ok := x.SetString(parts[0], 10); !ok {
			p.errorf(errors.BadRequest, "invalid number: %q", v)
			return nil
		}
		intexp(x, precision)
		if len(parts) == 1 {
			return x
		}

		fpart := parts[1]
		if len(fpart) > int(precision) {
			fpart = fpart[:precision]
		} else {
			fpart += strings.Repeat("0", int(precision)-len(fpart))
		}
		y := new(big.Int)
		if _, ok := y.SetString(fpart, 10); !ok {
			p.errorf(errors.BadRequest, "invalid number: %q", v)
			return nil
		}
		return x.Add(x, y)

	default:
		p.errorf(errors.BadRequest, "cannot convert %T to a number", v)
		return nil
	}
}

func intexp(x *big.Int, precision uint64) *big.Int {
	y := big.NewInt(10)
	y.Exp(y, big.NewInt(int64(precision)), nil)
	return x.Mul(x, y)
}

func bigfloat(v float64, precision uint64) *big.Int {
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

func (p *parser) parseTimestamp(v any) signing.Timestamp {
	switch v := v.(type) {
	case signing.Timestamp:
		return v
	case time.Time:
		return signing.TimestampFromValue(v.UTC().UnixMilli())
	case *uint64:
		return (*signing.TimestampFromVariable)(v)
	default:
		return signing.TimestampFromValue(p.parseUint(v))
	}
}

func (p *parser) parseUint(v any) uint64 {
	switch v := v.(type) {
	case *big.Int:
		return v.Uint64()
	case big.Int:
		return v.Uint64()

	case int:
		return uint64(v)
	case int8:
		return uint64(v)
	case int16:
		return uint64(v)
	case int32:
		return uint64(v)
	case int64:
		return uint64(v)
	case uint:
		return uint64(v)
	case uint8:
		return uint64(v)
	case uint16:
		return uint64(v)
	case uint32:
		return uint64(v)
	case uint64:
		return uint64(v) //nolint:unconvert

	case float32:
		return uint64(v)
	case float64:
		return uint64(v)

	case string:
		u, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			p.errorf(errors.BadRequest, "not a number: %w", err)
		}
		return u

	default:
		p.errorf(errors.BadRequest, "cannot convert %T to a number", v)
		return 0
	}
}
