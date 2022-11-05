// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package build

import "gitlab.com/accumulatenetwork/accumulate/protocol"

// Fix lint
var _ any = CreateKeyPageBuilder{}.addEntry
var _ any = AddKeyOperationBuilder{}.addEntry
var _ any = RemoveKeyOperationBuilder{}.addEntry
var _ any = UpdateKeyOperationBuilder{}.addEntry

type AddKeyOperationBuilder struct {
	b  UpdateKeyPageBuilder
	op protocol.AddKeyOperation
}

func (b AddKeyOperationBuilder) Entry() KeyPageEntryBuilder[AddKeyOperationBuilder] {
	return KeyPageEntryBuilder[AddKeyOperationBuilder]{t: b}
}

func (b AddKeyOperationBuilder) addEntry(entry protocol.KeySpecParams, err []error) AddKeyOperationBuilder {
	b.b.t.record(err...)
	b.op.Entry = entry
	return b
}

func (b AddKeyOperationBuilder) FinishOperation() UpdateKeyPageBuilder {
	b.b.body.Operation = append(b.b.body.Operation, &b.op)
	return b.b
}

type UpdateKeyOperationBuilder struct {
	b      UpdateKeyPageBuilder
	op     protocol.UpdateKeyOperation
	didOld bool
}

func (b UpdateKeyOperationBuilder) Entry() KeyPageEntryBuilder[UpdateKeyOperationBuilder] {
	return KeyPageEntryBuilder[UpdateKeyOperationBuilder]{t: b}
}

func (b UpdateKeyOperationBuilder) To() KeyPageEntryBuilder[UpdateKeyOperationBuilder] {
	return KeyPageEntryBuilder[UpdateKeyOperationBuilder]{t: b}
}

func (b UpdateKeyOperationBuilder) addEntry(entry protocol.KeySpecParams, err []error) UpdateKeyOperationBuilder {
	b.b.t.record(err...)
	if !b.didOld {
		b.didOld = true
		b.op.OldEntry = entry
	} else {
		b.op.NewEntry = entry
	}
	return b
}

func (b UpdateKeyOperationBuilder) FinishOperation() UpdateKeyPageBuilder {
	b.b.body.Operation = append(b.b.body.Operation, &b.op)
	return b.b
}

type RemoveKeyOperationBuilder struct {
	b  UpdateKeyPageBuilder
	op protocol.RemoveKeyOperation
}

func (b RemoveKeyOperationBuilder) Entry() KeyPageEntryBuilder[RemoveKeyOperationBuilder] {
	return KeyPageEntryBuilder[RemoveKeyOperationBuilder]{t: b}
}

func (b RemoveKeyOperationBuilder) addEntry(entry protocol.KeySpecParams, err []error) RemoveKeyOperationBuilder {
	b.b.t.record(err...)
	b.op.Entry = entry
	return b
}

func (b RemoveKeyOperationBuilder) FinishOperation() UpdateKeyPageBuilder {
	b.b.body.Operation = append(b.b.body.Operation, &b.op)
	return b.b
}

type UpdateAllowedKeyPageOperationBuilder struct {
	b  UpdateKeyPageBuilder
	op protocol.UpdateAllowedKeyPageOperation
}

func (b UpdateAllowedKeyPageOperationBuilder) Allow(typ protocol.TransactionType) UpdateAllowedKeyPageOperationBuilder {
	b.op.Allow = append(b.op.Allow, typ)
	return b
}

func (b UpdateAllowedKeyPageOperationBuilder) Deny(typ protocol.TransactionType) UpdateAllowedKeyPageOperationBuilder {
	b.op.Deny = append(b.op.Deny, typ)
	return b
}

func (b UpdateAllowedKeyPageOperationBuilder) FinishOperation() UpdateKeyPageBuilder {
	b.b.body.Operation = append(b.b.body.Operation, &b.op)
	return b.b
}
