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

func (b KeyPageEntryBuilder[T]) Owner(owner any) KeyPageEntryBuilder[T] {
	b.entryWithOwner(&b.entry, owner)
	return b
}

func (b KeyPageEntryBuilder[T]) Hash(hash []byte) KeyPageEntryBuilder[T] {
	b.entryWithHash(&b.entry, hash)
	return b
}

func (b KeyPageEntryBuilder[T]) Key(key any, typ protocol.SignatureType) KeyPageEntryBuilder[T] {
	b.entryWithKey(&b.entry, key, typ)
	return b
}

func (b KeyPageEntryBuilder[T]) FinishEntry() T {
	return b.t.addEntry(b.entry, b.errs)
}

func (p *parser) entryWithOwner(e *protocol.KeySpecParams, owner any) {
	e.Delegate = p.parseUrl(owner)
}

func (p *parser) entryWithHash(e *protocol.KeySpecParams, hash []byte) {
	e.KeyHash = hash
}

func (p *parser) entryWithKey(e *protocol.KeySpecParams, key any, typ protocol.SignatureType) {
	e.KeyHash = p.hashKey(p.parsePublicKey(key), typ)
}
