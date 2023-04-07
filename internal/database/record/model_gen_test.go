// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package record_test

// GENERATED BY go run ./tools/cmd/gen-model. DO NOT EDIT.

//lint:file-ignore S1008,U1000 generated code

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type ChangeSet interface {
	record.Record
	Entity(name string) Entity
	ChangeLog() record.Counted[string]
}

type changeSet struct {
	logger logging.OptionalLogger
	store  record.Store

	entity    map[entityKey]*entity
	changeLog record.Counted[string]
}

type entityKey struct {
	Name string
}

func keyForEntity(name string) entityKey {
	return entityKey{name}
}

func (c *changeSet) Entity(name string) Entity {
	return record.FieldGetOrCreateMap(&c.entity, keyForEntity(name), func() *entity {
		v := new(entity)
		v.logger = c.logger
		v.store = c.store
		v.key = (*record.Key)(nil).Append("Entity", name)
		v.parent = c
		v.label = "entity" + " " + name
		return v
	})
}

func (c *changeSet) ChangeLog() record.Counted[string] {
	return record.FieldGetOrCreate(&c.changeLog, func() record.Counted[string] {
		return record.NewCounted(c.logger.L, c.store, (*record.Key)(nil).Append("ChangeLog"), "change log", record.WrappedFactory(record.StringWrapper))
	})
}

func (c *changeSet) Resolve(key *record.Key) (record.Record, *record.Key, error) {
	if key.Len() == 0 {
		return nil, nil, errors.InternalError.With("bad key for change set")
	}

	switch key.Get(0) {
	case "Entity":
		if key.Len() < 2 {
			return nil, nil, errors.InternalError.With("bad key for change set")
		}
		name, okName := key.Get(1).(string)
		if !okName {
			return nil, nil, errors.InternalError.With("bad key for change set")
		}
		v := c.Entity(name)
		return v, key.SliceI(2), nil
	case "ChangeLog":
		return c.ChangeLog(), key.SliceI(1), nil
	default:
		return nil, nil, errors.InternalError.With("bad key for change set")
	}
}

func (c *changeSet) IsDirty() bool {
	if c == nil {
		return false
	}

	for _, v := range c.entity {
		if v.IsDirty() {
			return true
		}
	}
	if record.FieldIsDirty(c.changeLog) {
		return true
	}

	return false
}

func (c *changeSet) WalkChanges(fn record.WalkFunc) error {
	if c == nil {
		return nil
	}

	var err error
	for _, v := range c.entity {
		record.FieldWalkChanges(&err, v, fn)
	}
	record.FieldWalkChanges(&err, c.changeLog, fn)
	return err
}

func (c *changeSet) Commit() error {
	if c == nil {
		return nil
	}

	var err error
	for _, v := range c.entity {
		record.FieldCommit(&err, v)
	}
	record.FieldCommit(&err, c.changeLog)

	return err
}

type Entity interface {
	record.Record
	Union() record.Value[protocol.Account]
	Set() record.Set[*url.TxID]
	CountableRefType() record.Counted[*protocol.Transaction]
	CountableUnion() record.Counted[protocol.Account]
}

type entity struct {
	logger logging.OptionalLogger
	store  record.Store
	key    *record.Key
	label  string
	parent *changeSet

	union            record.Value[protocol.Account]
	set              record.Set[*url.TxID]
	countableRefType record.Counted[*protocol.Transaction]
	countableUnion   record.Counted[protocol.Account]
}

func (c *entity) Union() record.Value[protocol.Account] {
	return record.FieldGetOrCreate(&c.union, func() record.Value[protocol.Account] {
		return record.NewValue(c.logger.L, c.store, c.key.Append("Union"), c.label+" "+"union", false, record.Union(protocol.UnmarshalAccount))
	})
}

func (c *entity) Set() record.Set[*url.TxID] {
	return record.FieldGetOrCreate(&c.set, func() record.Set[*url.TxID] {
		return record.NewSet(c.logger.L, c.store, c.key.Append("Set"), c.label+" "+"set", record.Wrapped(record.TxidWrapper), record.CompareTxid)
	})
}

func (c *entity) CountableRefType() record.Counted[*protocol.Transaction] {
	return record.FieldGetOrCreate(&c.countableRefType, func() record.Counted[*protocol.Transaction] {
		return record.NewCounted(c.logger.L, c.store, c.key.Append("CountableRefType"), c.label+" "+"countable ref type", record.Struct[protocol.Transaction])
	})
}

func (c *entity) CountableUnion() record.Counted[protocol.Account] {
	return record.FieldGetOrCreate(&c.countableUnion, func() record.Counted[protocol.Account] {
		return record.NewCounted(c.logger.L, c.store, c.key.Append("CountableUnion"), c.label+" "+"countable union", record.UnionFactory(protocol.UnmarshalAccount))
	})
}

func (c *entity) Resolve(key *record.Key) (record.Record, *record.Key, error) {
	if key.Len() == 0 {
		return nil, nil, errors.InternalError.With("bad key for entity")
	}

	switch key.Get(0) {
	case "Union":
		return c.Union(), key.SliceI(1), nil
	case "Set":
		return c.Set(), key.SliceI(1), nil
	case "CountableRefType":
		return c.CountableRefType(), key.SliceI(1), nil
	case "CountableUnion":
		return c.CountableUnion(), key.SliceI(1), nil
	default:
		return nil, nil, errors.InternalError.With("bad key for entity")
	}
}

func (c *entity) IsDirty() bool {
	if c == nil {
		return false
	}

	if record.FieldIsDirty(c.union) {
		return true
	}
	if record.FieldIsDirty(c.set) {
		return true
	}
	if record.FieldIsDirty(c.countableRefType) {
		return true
	}
	if record.FieldIsDirty(c.countableUnion) {
		return true
	}

	return false
}

func (c *entity) WalkChanges(fn record.WalkFunc) error {
	if c == nil {
		return nil
	}

	var err error
	record.FieldWalkChanges(&err, c.union, fn)
	record.FieldWalkChanges(&err, c.set, fn)
	record.FieldWalkChanges(&err, c.countableRefType, fn)
	record.FieldWalkChanges(&err, c.countableUnion, fn)
	return err
}

func (c *entity) baseCommit() error {
	if c == nil {
		return nil
	}

	var err error
	record.FieldCommit(&err, c.union)
	record.FieldCommit(&err, c.set)
	record.FieldCommit(&err, c.countableRefType)
	record.FieldCommit(&err, c.countableUnion)

	return err
}

type TemplateTest struct {
	logger logging.OptionalLogger
	store  record.Store
	key    *record.Key
	label  string

	wrapped     record.Value[string]
	structPtr   record.Value[*StructType]
	union       record.Value[UnionType]
	wrappedSet  record.Set[*url.URL]
	structSet   record.Set[*StructType]
	unionSet    record.Set[UnionType]
	wrappedList record.Counted[string]
	structList  record.Counted[*StructType]
	unionList   record.Counted[UnionType]
}

func (c *TemplateTest) Wrapped() record.Value[string] {
	return record.FieldGetOrCreate(&c.wrapped, func() record.Value[string] {
		return record.NewValue(c.logger.L, c.store, c.key.Append("Wrapped"), c.label+" "+"wrapped", false, record.Wrapped(record.StringWrapper))
	})
}

func (c *TemplateTest) StructPtr() record.Value[*StructType] {
	return record.FieldGetOrCreate(&c.structPtr, func() record.Value[*StructType] {
		return record.NewValue(c.logger.L, c.store, c.key.Append("StructPtr"), c.label+" "+"struct ptr", false, record.Struct[StructType]())
	})
}

func (c *TemplateTest) Union() record.Value[UnionType] {
	return record.FieldGetOrCreate(&c.union, func() record.Value[UnionType] {
		return record.NewValue(c.logger.L, c.store, c.key.Append("Union"), c.label+" "+"union", false, record.Union(UnmarshalUnionType))
	})
}

func (c *TemplateTest) WrappedSet() record.Set[*url.URL] {
	return record.FieldGetOrCreate(&c.wrappedSet, func() record.Set[*url.URL] {
		return record.NewSet(c.logger.L, c.store, c.key.Append("WrappedSet"), c.label+" "+"wrapped set", record.Wrapped(record.UrlWrapper), record.CompareUrl)
	})
}

func (c *TemplateTest) StructSet() record.Set[*StructType] {
	return record.FieldGetOrCreate(&c.structSet, func() record.Set[*StructType] {
		return record.NewSet(c.logger.L, c.store, c.key.Append("StructSet"), c.label+" "+"struct set", record.Struct[StructType](), func(u, v *StructType) int { return u.Compare(v) })
	})
}

func (c *TemplateTest) UnionSet() record.Set[UnionType] {
	return record.FieldGetOrCreate(&c.unionSet, func() record.Set[UnionType] {
		return record.NewSet(c.logger.L, c.store, c.key.Append("UnionSet"), c.label+" "+"union set", record.Union(UnmarshalUnionType), func(u, v UnionType) int { return u.Compare(v) })
	})
}

func (c *TemplateTest) WrappedList() record.Counted[string] {
	return record.FieldGetOrCreate(&c.wrappedList, func() record.Counted[string] {
		return record.NewCounted(c.logger.L, c.store, c.key.Append("WrappedList"), c.label+" "+"wrapped list", record.WrappedFactory(record.StringWrapper))
	})
}

func (c *TemplateTest) StructList() record.Counted[*StructType] {
	return record.FieldGetOrCreate(&c.structList, func() record.Counted[*StructType] {
		return record.NewCounted(c.logger.L, c.store, c.key.Append("StructList"), c.label+" "+"struct list", record.Struct[StructType])
	})
}

func (c *TemplateTest) UnionList() record.Counted[UnionType] {
	return record.FieldGetOrCreate(&c.unionList, func() record.Counted[UnionType] {
		return record.NewCounted(c.logger.L, c.store, c.key.Append("UnionList"), c.label+" "+"union list", record.UnionFactory(UnmarshalUnionType))
	})
}

func (c *TemplateTest) Resolve(key *record.Key) (record.Record, *record.Key, error) {
	if key.Len() == 0 {
		return nil, nil, errors.InternalError.With("bad key for template test")
	}

	switch key.Get(0) {
	case "Wrapped":
		return c.Wrapped(), key.SliceI(1), nil
	case "StructPtr":
		return c.StructPtr(), key.SliceI(1), nil
	case "Union":
		return c.Union(), key.SliceI(1), nil
	case "WrappedSet":
		return c.WrappedSet(), key.SliceI(1), nil
	case "StructSet":
		return c.StructSet(), key.SliceI(1), nil
	case "UnionSet":
		return c.UnionSet(), key.SliceI(1), nil
	case "WrappedList":
		return c.WrappedList(), key.SliceI(1), nil
	case "StructList":
		return c.StructList(), key.SliceI(1), nil
	case "UnionList":
		return c.UnionList(), key.SliceI(1), nil
	default:
		return nil, nil, errors.InternalError.With("bad key for template test")
	}
}

func (c *TemplateTest) IsDirty() bool {
	if c == nil {
		return false
	}

	if record.FieldIsDirty(c.wrapped) {
		return true
	}
	if record.FieldIsDirty(c.structPtr) {
		return true
	}
	if record.FieldIsDirty(c.union) {
		return true
	}
	if record.FieldIsDirty(c.wrappedSet) {
		return true
	}
	if record.FieldIsDirty(c.structSet) {
		return true
	}
	if record.FieldIsDirty(c.unionSet) {
		return true
	}
	if record.FieldIsDirty(c.wrappedList) {
		return true
	}
	if record.FieldIsDirty(c.structList) {
		return true
	}
	if record.FieldIsDirty(c.unionList) {
		return true
	}

	return false
}

func (c *TemplateTest) WalkChanges(fn record.WalkFunc) error {
	if c == nil {
		return nil
	}

	var err error
	record.FieldWalkChanges(&err, c.wrapped, fn)
	record.FieldWalkChanges(&err, c.structPtr, fn)
	record.FieldWalkChanges(&err, c.union, fn)
	record.FieldWalkChanges(&err, c.wrappedSet, fn)
	record.FieldWalkChanges(&err, c.structSet, fn)
	record.FieldWalkChanges(&err, c.unionSet, fn)
	record.FieldWalkChanges(&err, c.wrappedList, fn)
	record.FieldWalkChanges(&err, c.structList, fn)
	record.FieldWalkChanges(&err, c.unionList, fn)
	return err
}

func (c *TemplateTest) Commit() error {
	if c == nil {
		return nil
	}

	var err error
	record.FieldCommit(&err, c.wrapped)
	record.FieldCommit(&err, c.structPtr)
	record.FieldCommit(&err, c.union)
	record.FieldCommit(&err, c.wrappedSet)
	record.FieldCommit(&err, c.structSet)
	record.FieldCommit(&err, c.unionSet)
	record.FieldCommit(&err, c.wrappedList)
	record.FieldCommit(&err, c.structList)
	record.FieldCommit(&err, c.unionList)

	return err
}
