// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package build

import "gitlab.com/accumulatenetwork/accumulate/protocol"

type keyPageEntryBuilderArg[T any] interface {
	addEntry(protocol.KeySpecParams, []error) T
}

type KeyPageEntryBuilder[T keyPageEntryBuilderArg[T]] struct {
	t T
	parser
	entry protocol.KeySpecParams
}

func (b KeyPageEntryBuilder[T]) Owner(owner any, path ...string) KeyPageEntryBuilder[T] {
	b.entry.Delegate = b.parseUrl(owner, path...)
	return b
}

func (b KeyPageEntryBuilder[T]) Hash(hash any) KeyPageEntryBuilder[T] {
	b.entry.KeyHash = b.parseHash(hash)
	return b
}

func (b KeyPageEntryBuilder[T]) Key(key any, typ protocol.SignatureType) KeyPageEntryBuilder[T] {
	b.entry.KeyHash = b.hashKey(b.parseKey(key, typ, false))
	return b
}

func (b KeyPageEntryBuilder[T]) FinishEntry() T {
	return b.t.addEntry(b.entry, b.errs)
}
