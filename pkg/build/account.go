// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package build

import (
	"crypto/ed25519"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type DbResolver = func(*url.URL) database.Updater
type accountBuildStep func(dbr DbResolver) error
type accountBuildSteps []accountBuildStep

func (a *accountBuildSteps) add(step accountBuildStep) {
	*a = append(*a, step)
}

func (a *accountBuildSteps) execute(dbr DbResolver) error {
	for _, step := range *a {
		err := step(dbr)
		if err != nil {
			return err
		}
	}
	*a = nil
	return nil
}

type baseAccountBuilder struct {
	parser
	prev  *baseAccountBuilder
	url   *url.URL
	steps *accountBuildSteps
}

func (b *baseAccountBuilder) Url() *url.URL { return b.url }

// AccountID returns Url().AccountID(). AccountID exists solely to allow
// builders to be passed to [record.NewKey] and friends.
func (b *baseAccountBuilder) AccountID() []byte {
	return b.url.AccountID()
}

func (b *baseAccountBuilder) ensureSteps() *accountBuildSteps {
	if b.steps != nil {
		return b.steps
	}
	if b.prev != nil {
		b.steps = b.prev.ensureSteps()
	} else {
		b.steps = new(accountBuildSteps)
	}
	return b.steps
}

func (b *baseAccountBuilder) addStep(step accountBuildStep) {
	b.ensureSteps().add(func(dbr DbResolver) error {
		if len(b.errs) > 0 {
			return b.err()
		}
		return step(dbr)
	})
}

func (b *baseAccountBuilder) parent() baseAccountBuilder {
	parent, ok := b.url.Parent()
	if !ok {
		panic(fmt.Errorf("%v is a root account", b.url))
	}
	if b.prev != nil && b.prev.url.Equal(parent) {
		b.prev.errs = append(b.prev.errs, b.errs...)
		return *b.prev
	}
	return baseAccountBuilder{parser: b.parser, prev: b, url: parent}
}

func (b *baseAccountBuilder) child(path ...string) baseAccountBuilder {
	return baseAccountBuilder{prev: b, url: b.parseUrl(b.url, path...)}
}

type AccountBuilder[T protocol.Account] struct {
	baseAccountBuilder
}

func (b *AccountBuilder[T]) addAcctStep(fn func(r *database.Account) error) {
	b.baseAccountBuilder.addStep(func(dbr DbResolver) error {
		return dbr(b.url).Update(func(batch *database.Batch) error {
			return fn(batch.Account(b.url))
		})
	})
}

func (b *AccountBuilder[T]) Create(account T) *AccountBuilder[T] {
	b.addStep(func(dbr DbResolver) error {
		if account.GetUrl() == nil {
			switch a := any(account).(type) {
			case *protocol.ADI:
				a.Url = b.url
			case *protocol.TokenAccount:
				a.Url = b.url
			case *protocol.KeyBook:
				a.Url = b.url
			case *protocol.KeyPage:
				a.Url = b.url
			case *protocol.DataAccount:
				a.Url = b.url
			case *protocol.TokenIssuer:
				a.Url = b.url
			default:
				return fmt.Errorf("missing account URL (%T)", account)
			}

		} else if !account.GetUrl().Equal(b.url) {
			return fmt.Errorf("wrong URL: want %v, got %v", b.url, account.GetUrl())
		}

		switch acct := any(account).(type) {
		case *protocol.KeyBook:
			// If the key book's authority is not set explicitly, set it to
			// itself
			if len(acct.Authorities) == 0 {
				acct.AddAuthority(acct.Url)
			}

		case *protocol.KeyPage:
			// Update the book's page count if necessary
			bookUrl, index, ok := protocol.ParseKeyPageUrl(b.url)
			if !ok {
				return fmt.Errorf("invalid key page URL %v", b.url)
			}

			book, err := loadAccount[*protocol.KeyBook](dbr, bookUrl)
			if err != nil {
				return err
			}
			if book.PageCount < index+1 {
				book.PageCount = index + 1
				err = dbr(bookUrl).Update(func(batch *database.Batch) error {
					return batch.Account(bookUrl).Main().Put(book)
				})
				if err != nil {
					return err
				}
			}

		case protocol.FullAccount:
			// Inherit the identity's authorities if the account defines
			// authorities, hash none, and we're not on Baikonur
			ledger, err := loadAccount[*protocol.SystemLedger](dbr, protocol.DnUrl().JoinPath(protocol.Ledger))
			if err != nil {
				return err
			}

			inheritAuth := true &&
				!ledger.ExecutorVersion.V2BaikonurEnabled() &&
				!b.url.IsRootIdentity()

			if inheritAuth && len(acct.GetAuth().Authorities) == 0 {
				adi, err := loadAccount[*protocol.ADI](dbr, b.url.Identity())
				if err != nil {
					return err
				}
				acct.GetAuth().Authorities = adi.Authorities
			}
		}

		return dbr(b.url).Update(func(batch *database.Batch) error {
			return batch.Account(b.url).Main().Put(account)
		})
	})
	return b
}

func (b *AccountBuilder[T]) Update(fn func(T)) *AccountBuilder[T] {
	if b.url == nil {
		b.errorf(errors.BadRequest, "no URL specified")
		return b
	}
	b.addAcctStep(func(r *database.Account) error {
		var account T
		err := r.Main().GetAs(&account)
		if err != nil {
			return err
		}
		fn(account)
		return r.Main().Put(account)
	})
	return b
}

func (b *AccountBuilder[T]) Build(dbr DbResolver) error {
	if len(b.errs) > 0 {
		return b.err()
	}
	return b.steps.execute(dbr)
}

func (b *AccountBuilder[T]) Load(dbr DbResolver) (T, error) {
	return loadAccount[T](dbr, b.url)
}

func loadAccount[T protocol.Account](dbr DbResolver, u *url.URL) (T, error) {
	var account T
	return account, dbr(u).View(func(batch *database.Batch) error {
		return batch.Account(u).Main().GetAs(&account)
	})
}

type IdentityBuilder struct{ AccountBuilder[*protocol.ADI] }

func Identity(base any, path ...string) *IdentityBuilder {
	if s, ok := base.(string); ok {
		base = protocol.AccountUrl(s)
	}
	b := &IdentityBuilder{}
	b.url = b.parseUrl(base, path...)
	return b
}

func (b *IdentityBuilder) Identity() *IdentityBuilder {
	return &IdentityBuilder{AccountBuilder[*protocol.ADI]{b.parent()}}
}

func (b *IdentityBuilder) Create(bookPath string) *IdentityBuilder {
	c := b.Book(bookPath).Create()
	b.AccountBuilder.Create(&protocol.ADI{
		AccountAuth: protocol.AccountAuth{
			Authorities: []protocol.AuthorityEntry{
				{Url: c.url},
			},
		},
	})
	return b
}

func (b *IdentityBuilder) Update(fn func(adi *protocol.ADI)) *IdentityBuilder {
	b.AccountBuilder.Update(fn)
	return b
}

type KeyBookBuilder struct {
	AccountBuilder[*protocol.KeyBook]
}

func (b *IdentityBuilder) Book(path string) *KeyBookBuilder {
	return &KeyBookBuilder{AccountBuilder[*protocol.KeyBook]{b.child(path)}}
}

func (b *KeyBookBuilder) Identity() *IdentityBuilder {
	return &IdentityBuilder{AccountBuilder[*protocol.ADI]{b.parent()}}
}

func (b *KeyBookBuilder) Create() *KeyBookBuilder {
	b.AccountBuilder.Create(&protocol.KeyBook{})
	return b
}

func (b *KeyBookBuilder) Update(fn func(book *protocol.KeyBook)) *KeyBookBuilder {
	b.AccountBuilder.Update(fn)
	return b
}

func (b *KeyBookBuilder) Page(page int) *KeyPageBuilder {
	return &KeyPageBuilder{AccountBuilder[*protocol.KeyPage]{b.child(fmt.Sprint(page))}}
}

type KeyPageBuilder struct {
	AccountBuilder[*protocol.KeyPage]
}

func (b *KeyPageBuilder) Book() *KeyBookBuilder {
	return &KeyBookBuilder{AccountBuilder[*protocol.KeyBook]{b.parent()}}
}

func (b *KeyPageBuilder) Create() *KeyPageBuilder {
	b.AccountBuilder.Create(&protocol.KeyPage{
		Version: 1,
	})
	return b
}

func (b *KeyPageBuilder) Update(fn func(page *protocol.KeyPage)) *KeyPageBuilder {
	b.AccountBuilder.Update(fn)
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
	default:
		b.errorf(errors.BadRequest, "generating %v keys is not supported", typ)
		return nil
	}
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
	AccountBuilder[*protocol.TokenAccount]
}

func (b *IdentityBuilder) Tokens(path string) *TokenAccountBuilder {
	return &TokenAccountBuilder{AccountBuilder[*protocol.TokenAccount]{b.child(path)}}
}

func (b *TokenAccountBuilder) Identity() *IdentityBuilder {
	return &IdentityBuilder{AccountBuilder[*protocol.ADI]{b.parent()}}
}

func (b *TokenAccountBuilder) Create(issuer any, path ...string) *TokenAccountBuilder {
	b.AccountBuilder.Create(&protocol.TokenAccount{
		TokenUrl: b.parseUrl(issuer, path...),
	})
	return b
}

func (b *TokenAccountBuilder) Update(fn func(acct *protocol.TokenAccount)) *TokenAccountBuilder {
	b.AccountBuilder.Update(fn)
	return b
}

func (b *TokenAccountBuilder) Add(amount any) *TokenAccountBuilder {
	b.addStep(func(dbr DbResolver) error {
		acct, err := loadAccount[*protocol.TokenAccount](dbr, b.url)
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

type DataAccountBuilder struct {
	AccountBuilder[*protocol.DataAccount]
}

func (b *IdentityBuilder) Data(path string) *DataAccountBuilder {
	return &DataAccountBuilder{AccountBuilder[*protocol.DataAccount]{b.child(path)}}
}

func (b *DataAccountBuilder) Identity() *IdentityBuilder {
	return &IdentityBuilder{AccountBuilder[*protocol.ADI]{b.parent()}}
}

func (b *DataAccountBuilder) Create() *DataAccountBuilder {
	b.AccountBuilder.Create(&protocol.DataAccount{})
	return b
}

func (b *DataAccountBuilder) Update(fn func(acct *protocol.DataAccount)) *DataAccountBuilder {
	b.AccountBuilder.Update(fn)
	return b
}
