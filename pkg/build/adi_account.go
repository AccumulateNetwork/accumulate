// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package build

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	badrand "math/rand"

	btc "github.com/btcsuite/btcd/btcec"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type IdentityBuilder struct {
	accountBuilder[*protocol.ADI]
}

func Identity(base any, path ...string) *IdentityBuilder {
	if s, ok := base.(string); ok {
		base = protocol.AccountUrl(s)
	}
	b := &IdentityBuilder{}
	b.url = b.parseUrl(base, path...)
	return b
}

func (b *IdentityBuilder) Identity() *IdentityBuilder {
	return b.ensureParent().(*IdentityBuilder)
}

func (b *IdentityBuilder) Create(bookPath string) *IdentityBuilder {
	c := b.Book(bookPath).Create()
	b.accountBuilder.create(&protocol.ADI{
		AccountAuth: protocol.AccountAuth{
			Authorities: []protocol.AuthorityEntry{
				{Url: c.url},
			},
		},
	})
	return b
}

func (b *IdentityBuilder) Update(fn func(adi *protocol.ADI)) *IdentityBuilder {
	b.accountBuilder.update(fn)
	return b
}

type KeyBookBuilder struct {
	accountBuilder[*protocol.KeyBook]
}

func (b *IdentityBuilder) Book(path string) *KeyBookBuilder {
	return &KeyBookBuilder{child[*protocol.KeyBook](b, path)}
}

func (b *KeyBookBuilder) Identity() *IdentityBuilder {
	return b.ensureParent().(*IdentityBuilder)
}

func (b *KeyBookBuilder) Create() *KeyBookBuilder {
	b.accountBuilder.create(&protocol.KeyBook{})
	return b
}

func (b *KeyBookBuilder) Update(fn func(book *protocol.KeyBook)) *KeyBookBuilder {
	b.accountBuilder.update(fn)
	return b
}

func (b *KeyBookBuilder) Page(page int) *KeyPageBuilder {
	return &KeyPageBuilder{child[*protocol.KeyPage](b, fmt.Sprint(page))}
}

type KeyPageBuilder struct {
	accountBuilder[*protocol.KeyPage]
}

func (b *KeyPageBuilder) Book() *KeyBookBuilder {
	return b.ensureParent().(*KeyBookBuilder)
}

func (b *KeyPageBuilder) Create() *KeyPageBuilder {
	b.accountBuilder.create(&protocol.KeyPage{
		Version: 1,
	})
	return b
}

func (b *KeyPageBuilder) Update(fn func(page *protocol.KeyPage)) *KeyPageBuilder {
	b.accountBuilder.update(fn)
	return b
}

func (b *KeyPageBuilder) AddEntry(entry protocol.KeySpec) *KeyPageBuilder {
	return b.Update(func(page *protocol.KeyPage) {
		page.AddKeySpec(&entry)
	})
}

func (b *KeyPageBuilder) AddKey(key any) *KeyPageBuilder {
	hash := b.hashKey(b.parseKey(key, 0, false))
	return b.AddEntry(protocol.KeySpec{PublicKeyHash: hash})
}

func (b *KeyPageBuilder) AddDelegate(base any, path ...string) *KeyPageBuilder {
	u := b.parseUrl(base, path...)
	return b.AddEntry(protocol.KeySpec{Delegate: u})
}

func (b *KeyPageBuilder) AddCredits(amount any) *KeyPageBuilder {
	a := b.parseAmount(amount, 2)
	return b.Update(func(page *protocol.KeyPage) {
		page.CreditCredits(a.Uint64())
	})
}

func (b *KeyPageBuilder) GenerateKey(typ protocol.SignatureType, seedParts ...any) address.Address {
	seed := record.NewKey(b.url).
		Append(seedParts...).
		Hash()

	switch typ {
	case protocol.SignatureTypeED25519,
		protocol.SignatureTypeLegacyED25519,
		protocol.SignatureTypeRCD1:
		sk := ed25519.NewKeyFromSeed(seed[:])
		addr := address.FromED25519PrivateKey(sk)
		addr.Type = typ
		b.AddKey(addr)
		return addr

	case protocol.SignatureTypeETH,
		protocol.SignatureTypeBTC,
		protocol.SignatureTypeBTCLegacy:
		sk := ecdsaFromSeed(btc.S256(), seed)
		addr := address.FromETHPrivateKey(sk)
		addr.Type = typ
		b.AddKey(addr)
		return addr

	default:
		b.errorf(errors.BadRequest, "generating %v keys is not supported", typ)
		return nil
	}
}

func ecdsaFromSeed(c elliptic.Curve, seed [32]byte) *ecdsa.PrivateKey {
	rand := badrand.New(badrand.NewSource(int64(binary.BigEndian.Uint64(seed[:]))))

	var k *big.Int
	for {
		N := c.Params().N
		b := make([]byte, (N.BitLen()+7)/8)
		if _, err := io.ReadFull(rand, b); err != nil {
			panic(err)
		}
		if excess := len(b)*8 - N.BitLen(); excess > 0 {
			b[0] >>= excess
		}
		k = new(big.Int).SetBytes(b)
		if k.Sign() != 0 && k.Cmp(N) < 0 {
			break
		}
	}

	priv := new(ecdsa.PrivateKey)
	priv.PublicKey.Curve = c
	priv.D = k
	priv.PublicKey.X, priv.PublicKey.Y = c.ScalarBaseMult(k.Bytes())
	return priv
}

func (b *KeyPageBuilder) SetAcceptThreshold(v uint64) *KeyPageBuilder {
	return b.Update(func(page *protocol.KeyPage) {
		page.AcceptThreshold = v
	})
}

func (b *KeyPageBuilder) SetRejectThreshold(v uint64) *KeyPageBuilder {
	return b.Update(func(page *protocol.KeyPage) {
		page.RejectThreshold = v
	})
}

func (b *KeyPageBuilder) SetResponseThreshold(v uint64) *KeyPageBuilder {
	return b.Update(func(page *protocol.KeyPage) {
		page.ResponseThreshold = v
	})
}

type TokenAccountBuilder struct {
	accountBuilder[*protocol.TokenAccount]
}

func (b *IdentityBuilder) Tokens(path string) *TokenAccountBuilder {
	return &TokenAccountBuilder{child[*protocol.TokenAccount](b, path)}
}

func (b *TokenAccountBuilder) Identity() *IdentityBuilder {
	return b.ensureParent().(*IdentityBuilder)
}

func (b *TokenAccountBuilder) Create(issuer any, path ...string) *TokenAccountBuilder {
	b.accountBuilder.create(&protocol.TokenAccount{
		TokenUrl: b.parseUrl(issuer, path...),
	})
	return b
}

func (b *TokenAccountBuilder) Update(fn func(acct *protocol.TokenAccount)) *TokenAccountBuilder {
	b.accountBuilder.update(fn)
	return b
}

func (b *TokenAccountBuilder) Add(amount any) *TokenAccountBuilder {
	b.addStep(func(dbr DbResolver) error {
		acct, err := loadAccount[*protocol.TokenAccount](dbr, b.url)
		if err != nil {
			return err
		}
		if acct.TokenUrl == nil {
			return errors.BadRequest.WithFormat("%v has no TokenUrl", b.url)
		}

		var precision uint64
		if protocol.AcmeUrl().Equal(acct.TokenUrl) {
			// acc://ACME may not exist yet if this is called before genesis
			precision = protocol.AcmePrecisionPower

		} else {
			issuer, err := loadAccount[*protocol.TokenIssuer](dbr, acct.TokenUrl)
			if err != nil {
				return err
			}
			precision = issuer.Precision
		}

		var p parser
		amt := p.parseAmount(amount, precision)
		if len(p.errs) > 0 {
			return p.err()
		}

		if !acct.CreditTokens(amt) {
			return errors.BadRequest.WithFormat("invalid amount %v", amount)
		}

		return dbr(b.url).Update(func(batch *database.Batch) error {
			return batch.Account(b.url).Main().Put(acct)
		})
	})
	return b
}

type DataAccountBuilder struct {
	accountBuilder[*protocol.DataAccount]
}

func (b *IdentityBuilder) Data(path string) *DataAccountBuilder {
	return &DataAccountBuilder{child[*protocol.DataAccount](b, path)}
}

func (b *DataAccountBuilder) Identity() *IdentityBuilder {
	return b.ensureParent().(*IdentityBuilder)
}

func (b *DataAccountBuilder) Create() *DataAccountBuilder {
	b.accountBuilder.create(&protocol.DataAccount{})
	return b
}

func (b *DataAccountBuilder) Update(fn func(acct *protocol.DataAccount)) *DataAccountBuilder {
	b.accountBuilder.update(fn)
	return b
}
