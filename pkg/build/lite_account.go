// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package build

import (
	"crypto/ed25519"
	"fmt"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type LiteIdentityBuilder struct {
	Key address.Address
	accountBuilder[*protocol.LiteIdentity]
}

func LiteIdentity() *LiteIdentityBuilder {
	return &LiteIdentityBuilder{}
}

func (b *LiteIdentityBuilder) For(key any) *LiteIdentityBuilder {
	b.accountBuilder.create(&protocol.LiteIdentity{})
	addr := b.parseKey(key, protocol.SignatureTypeUnknown, false)
	if addr == nil {
		return b
	}
	hash, ok := addr.GetPublicKeyHash()
	if !ok {
		return b
	}
	b.url = protocol.LiteAuthorityForHash(hash)
	return b
}

func (b *LiteIdentityBuilder) Generate(seedParts ...any) *LiteIdentityBuilder {
	var seed record.Key
	typ := protocol.SignatureTypeED25519
	for _, part := range seedParts {
		if x, ok := part.(protocol.SignatureType); ok {
			typ = x
			continue
		}

		switch x := part.(type) {
		case interface{ Name() string }:
			part = x.Name()
		case interface{ String() string }:
			part = x.String()
		}

		seed = *seed.Append(part)
	}

	switch typ {
	case protocol.SignatureTypeED25519,
		protocol.SignatureTypeLegacyED25519,
		protocol.SignatureTypeRCD1:
		h := seed.Hash()
		sk := ed25519.NewKeyFromSeed(h[:])
		addr := address.FromED25519PrivateKey(sk)
		addr.Type = typ
		b.Key = addr
		return b.For(addr)
	default:
		b.errorf(errors.BadRequest, "generating %v keys is not supported", typ)
		return b
	}
}

func (b *LiteIdentityBuilder) Update(fn func(lid *protocol.LiteIdentity)) *LiteIdentityBuilder {
	b.accountBuilder.update(fn)
	return b
}

func (b *LiteIdentityBuilder) AddCredits(amount any) *LiteIdentityBuilder {
	a := b.parseAmount(amount, 2)
	return b.Update(func(page *protocol.LiteIdentity) {
		page.CreditCredits(a.Uint64())
	})
}

type LiteTokenAccountBuilder struct {
	accountBuilder[*protocol.LiteTokenAccount]
}

func (b *LiteIdentityBuilder) Tokens(path string) *LiteTokenAccountBuilder {
	return &LiteTokenAccountBuilder{child[*protocol.LiteTokenAccount](b, path)}
}

func (b *LiteTokenAccountBuilder) Identity() *LiteIdentityBuilder {
	return b.ensureParent().(*LiteIdentityBuilder)
}

func (b *LiteTokenAccountBuilder) Create() *LiteTokenAccountBuilder {
	b.accountBuilder.create(&protocol.LiteTokenAccount{
		TokenUrl: b.parseUrl(strings.TrimPrefix(b.url.Path, "/")),
	})
	return b
}

func (b *LiteTokenAccountBuilder) Update(fn func(acct *protocol.LiteTokenAccount)) *LiteTokenAccountBuilder {
	b.accountBuilder.update(fn)
	return b
}

func (b *LiteTokenAccountBuilder) Add(amount any) *LiteTokenAccountBuilder {
	b.addStep(func(dbr DbResolver) error {
		acct, err := loadAccount[*protocol.LiteTokenAccount](dbr, b.url)
		if err != nil {
			return err
		}
		if acct.TokenUrl == nil {
			return fmt.Errorf("%v has no TokenUrl", b.url)
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
			return fmt.Errorf("invalid amount %v", amount)
		}

		return dbr(b.url).Update(func(batch *database.Batch) error {
			return batch.Account(b.url).Main().Put(acct)
		})
	})
	return b
}
