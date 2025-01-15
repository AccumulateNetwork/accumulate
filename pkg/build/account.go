// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package build

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type AccountBuilder interface {
	Url() *url.URL
	Build(dbr DbResolver) error

	ensureSteps() *accountBuildSteps
	record(err ...error)
}

type accountBuilder[T protocol.Account] struct {
	parser
	parent AccountBuilder
	url    *url.URL
	steps  *accountBuildSteps
}

func (b *accountBuilder[T]) Url() *url.URL { return b.url }

// AccountID returns Url().AccountID(). AccountID exists solely to allow
// builders to be passed to [record.NewKey] and friends.
func (b *accountBuilder[T]) AccountID() []byte {
	return b.url.AccountID()
}

func (b *accountBuilder[T]) ensureSteps() *accountBuildSteps {
	if b.steps != nil {
		return b.steps
	}
	if b.parent != nil {
		b.steps = b.parent.ensureSteps()
	} else {
		b.steps = new(accountBuildSteps)
	}
	return b.steps
}

func (b *accountBuilder[T]) addStep(step accountBuildStep) {
	b.ensureSteps().add(func(dbr DbResolver) error {
		if len(b.errs) > 0 {
			return b.err()
		}
		return step(dbr)
	})
}

func (b *accountBuilder[T]) ensureParent() AccountBuilder {
	_, ok := b.url.Parent()
	if !ok {
		panic(errors.BadRequest.WithFormat("%v is a root account", b.url))
	}
	if b.parent == nil {
		panic(errors.BadRequest.WithFormat("builder has no parent"))
	}
	b.parent.record(b.errs...)
	return b.parent
}

func child[T protocol.Account](b AccountBuilder, path ...string) accountBuilder[T] {
	var c accountBuilder[T]
	c.parent = b
	c.url = c.parseUrl(b.Url(), path...)
	return c
}

func (b *accountBuilder[T]) addAcctStep(fn func(r *database.Account) error) {
	b.addStep(func(dbr DbResolver) error {
		return dbr(b.url).Update(func(batch *database.Batch) error {
			return fn(batch.Account(b.url))
		})
	})
}

func (b *accountBuilder[T]) create(account T) {
	b.addStep(func(dbr DbResolver) error {
		if b.url == nil {
			return errors.BadRequest.With("no URL specified")
		}
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
			case *protocol.LiteIdentity:
				a.Url = b.url
			case *protocol.LiteTokenAccount:
				a.Url = b.url
			default:
				return errors.BadRequest.WithFormat("missing account URL (%T)", account)
			}

		} else if !account.GetUrl().Equal(b.url) {
			return errors.BadRequest.WithFormat("wrong URL: want %v, got %v", b.url, account.GetUrl())
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
				return errors.BadRequest.WithFormat("invalid key page URL %v", b.url)
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
}

func (b *accountBuilder[T]) update(fn func(T)) {
	b.addAcctStep(func(r *database.Account) error {
		if b.url == nil {
			return errors.BadRequest.With("no URL specified")
		}
		var account T
		err := r.Main().GetAs(&account)
		if err != nil {
			return err
		}
		fn(account)
		return r.Main().Put(account)
	})
}

func (b *accountBuilder[T]) Build(dbr DbResolver) error {
	if len(b.errs) > 0 {
		return b.err()
	}
	return b.steps.execute(dbr)
}

func (b *accountBuilder[T]) Load(dbr DbResolver) (T, error) {
	return loadAccount[T](dbr, b.url)
}

func loadAccount[T protocol.Account](dbr DbResolver, u *url.URL) (T, error) {
	var account T
	return account, dbr(u).View(func(batch *database.Batch) error {
		return batch.Account(u).Main().GetAs(&account)
	})
}

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
