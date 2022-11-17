// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package build

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
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
	p.errs = append(p.errs, err...)
}

func (p *parser) errorf(code errors.Status, format string, args ...interface{}) {
	p.record(code.WithFormat(format, args...))
}

func (p *parser) parseUrl(v any, path ...string) *url.URL {
	var str string
	switch v := v.(type) {
	case url.URL:
		return v.JoinPath(path...)
	case *url.URL:
		return v.JoinPath(path...)
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
		if len(v) != 32 {
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

func (p *parser) parsePublicKey(key any) []byte {
	switch key := key.(type) {
	case ed25519.PrivateKey:
		return key[32:]
	case ed25519.PublicKey:
		return key
	case tmed25519.PrivKey:
		return key[32:]
	case tmed25519.PubKey:
		return key
	case []byte:
		return key
	case string:
		b, err := hex.DecodeString(key)
		if err != nil {
			p.errorf(errors.BadRequest, "parse key as hex: %w", err)
			return nil
		}
		return b
	default:
		p.errorf(errors.BadRequest, "unsupported key type %T", key)
		return nil
	}
}

func (p *parser) hashKey(key []byte, typ protocol.SignatureType) []byte {
	switch typ {
	case protocol.SignatureTypeLegacyED25519,
		protocol.SignatureTypeED25519:
		return hash(key)
	case protocol.SignatureTypeRCD1:
		return protocol.GetRCDHashFromPublicKey(key, 1)
	case protocol.SignatureTypeBTC,
		protocol.SignatureTypeBTCLegacy:
		return protocol.BTCHash(key)
	case protocol.SignatureTypeETH:
		return protocol.ETHhash(key)
	default:
		p.errorf(errors.BadRequest, "unsupported key type %v", typ)
		return nil
	}
}

func hash(b []byte) []byte {
	h := sha256.Sum256(b)
	return h[:]
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
		return intexp(big.NewInt(int64(v)), precision)
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
		return bigfloat(float64(v), precision)

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
		return uint64(v)

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
