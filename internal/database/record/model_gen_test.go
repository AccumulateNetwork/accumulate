// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package record_test

// GENERATED BY go run ./tools/cmd/gen-model. DO NOT EDIT.

//lint:file-ignore S1008,U1000 generated code

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	record "gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type ChangeSet interface {
	record.Record
	Entity(name string) Entity
	ChangeLog() values.Counted[string]
}

type changeSet struct {
	logger logging.OptionalLogger
	store  record.Store

	entity    map[entityKey]*entity
	changeLog values.Counted[string]
}

func (c *changeSet) Key() *record.Key { return nil }

type entityKey struct {
	Name string
}

func keyForEntity(name string) entityKey {
	return entityKey{name}
}

func (c *changeSet) Entity(name string) Entity {
	return values.GetOrCreateMap1(&c.entity, keyForEntity(name), (*changeSet).newEntity, c, name)
}

func (c *changeSet) newEntity(name string) *entity {
	v := new(entity)
	v.logger = c.logger
	v.store = c.store
	v.key = (*record.Key)(nil).Append("Entity", name)
	v.parent = c
	v.label = "entity" + " " + name
	return v
}

func (c *changeSet) ChangeLog() values.Counted[string] {
	return values.GetOrCreate(&c.changeLog, (*changeSet).newChangeLog, c)
}

func (c *changeSet) newChangeLog() values.Counted[string] {
	return values.NewCounted(c.logger.L, c.store, (*record.Key)(nil).Append("ChangeLog"), "change log", values.WrappedFactory(values.StringWrapper))
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
	if values.IsDirty(c.changeLog) {
		return true
	}

	return false
}

func (c *changeSet) Walk(opts record.WalkOptions, fn record.WalkFunc) error {
	if c == nil {
		return nil
	}

	skip, err := values.WalkComposite(c, opts, fn)
	if skip || err != nil {
		return errors.UnknownError.Wrap(err)
	}
	for _, v := range c.entity {
		values.Walk(&err, v, opts, fn)
	}
	values.WalkField(&err, c.changeLog, c.ChangeLog, opts, fn)
	return err
}

func (c *changeSet) Commit() error {
	if c == nil {
		return nil
	}

	var err error
	for _, v := range c.entity {
		values.Commit(&err, v)
	}
	values.Commit(&err, c.changeLog)

	return err
}

type Entity interface {
	record.Record
	Union() values.Value[protocol.Account]
	Set() values.Set[*url.TxID]
	CountableRefType() values.Counted[*protocol.Transaction]
	CountableUnion() values.Counted[protocol.Account]
}

type entity struct {
	logger logging.OptionalLogger
	store  record.Store
	key    *record.Key
	label  string
	parent *changeSet

	union            values.Value[protocol.Account]
	set              values.Set[*url.TxID]
	countableRefType values.Counted[*protocol.Transaction]
	countableUnion   values.Counted[protocol.Account]
}

func (c *entity) Key() *record.Key { return c.key }

func (c *entity) Union() values.Value[protocol.Account] {
	return values.GetOrCreate(&c.union, (*entity).newUnion, c)
}

func (c *entity) newUnion() values.Value[protocol.Account] {
	return values.NewValue(c.logger.L, c.store, c.key.Append("Union"), c.label+" "+"union", false, values.Union(protocol.UnmarshalAccount))
}

func (c *entity) Set() values.Set[*url.TxID] {
	return values.GetOrCreate(&c.set, (*entity).newSet, c)
}

func (c *entity) newSet() values.Set[*url.TxID] {
	return values.NewSet(c.logger.L, c.store, c.key.Append("Set"), c.label+" "+"set", values.Wrapped(values.TxidWrapper), values.CompareTxid)
}

func (c *entity) CountableRefType() values.Counted[*protocol.Transaction] {
	return values.GetOrCreate(&c.countableRefType, (*entity).newCountableRefType, c)
}

func (c *entity) newCountableRefType() values.Counted[*protocol.Transaction] {
	return values.NewCounted(c.logger.L, c.store, c.key.Append("CountableRefType"), c.label+" "+"countable ref type", values.Struct[protocol.Transaction])
}

func (c *entity) CountableUnion() values.Counted[protocol.Account] {
	return values.GetOrCreate(&c.countableUnion, (*entity).newCountableUnion, c)
}

func (c *entity) newCountableUnion() values.Counted[protocol.Account] {
	return values.NewCounted(c.logger.L, c.store, c.key.Append("CountableUnion"), c.label+" "+"countable union", values.UnionFactory(protocol.UnmarshalAccount))
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

	if values.IsDirty(c.union) {
		return true
	}
	if values.IsDirty(c.set) {
		return true
	}
	if values.IsDirty(c.countableRefType) {
		return true
	}
	if values.IsDirty(c.countableUnion) {
		return true
	}

	return false
}

func (c *entity) Walk(opts record.WalkOptions, fn record.WalkFunc) error {
	if c == nil {
		return nil
	}

	skip, err := values.WalkComposite(c, opts, fn)
	if skip || err != nil {
		return errors.UnknownError.Wrap(err)
	}
	values.WalkField(&err, c.union, c.Union, opts, fn)
	values.WalkField(&err, c.set, c.Set, opts, fn)
	values.WalkField(&err, c.countableRefType, c.CountableRefType, opts, fn)
	values.WalkField(&err, c.countableUnion, c.CountableUnion, opts, fn)
	return err
}

func (c *entity) baseCommit() error {
	if c == nil {
		return nil
	}

	var err error
	values.Commit(&err, c.union)
	values.Commit(&err, c.set)
	values.Commit(&err, c.countableRefType)
	values.Commit(&err, c.countableUnion)

	return err
}

type TemplateTest struct {
	logger logging.OptionalLogger
	store  record.Store
	key    *record.Key
	label  string

	wrapped     values.Value[string]
	structPtr   values.Value[*StructType]
	union       values.Value[UnionType]
	wrappedSet  values.Set[*url.URL]
	structSet   values.Set[*StructType]
	unionSet    values.Set[UnionType]
	wrappedList values.Counted[string]
	structList  values.Counted[*StructType]
	unionList   values.Counted[UnionType]
}

func (c *TemplateTest) Key() *record.Key { return c.key }

func (c *TemplateTest) Wrapped() values.Value[string] {
	return values.GetOrCreate(&c.wrapped, (*TemplateTest).newWrapped, c)
}

func (c *TemplateTest) newWrapped() values.Value[string] {
	return values.NewValue(c.logger.L, c.store, c.key.Append("Wrapped"), c.label+" "+"wrapped", false, values.Wrapped(values.StringWrapper))
}

func (c *TemplateTest) StructPtr() values.Value[*StructType] {
	return values.GetOrCreate(&c.structPtr, (*TemplateTest).newStructPtr, c)
}

func (c *TemplateTest) newStructPtr() values.Value[*StructType] {
	return values.NewValue(c.logger.L, c.store, c.key.Append("StructPtr"), c.label+" "+"struct ptr", false, values.Struct[StructType]())
}

func (c *TemplateTest) Union() values.Value[UnionType] {
	return values.GetOrCreate(&c.union, (*TemplateTest).newUnion, c)
}

func (c *TemplateTest) newUnion() values.Value[UnionType] {
	return values.NewValue(c.logger.L, c.store, c.key.Append("Union"), c.label+" "+"union", false, values.Union(UnmarshalUnionType))
}

func (c *TemplateTest) WrappedSet() values.Set[*url.URL] {
	return values.GetOrCreate(&c.wrappedSet, (*TemplateTest).newWrappedSet, c)
}

func (c *TemplateTest) newWrappedSet() values.Set[*url.URL] {
	return values.NewSet(c.logger.L, c.store, c.key.Append("WrappedSet"), c.label+" "+"wrapped set", values.Wrapped(values.UrlWrapper), values.CompareUrl)
}

func (c *TemplateTest) StructSet() values.Set[*StructType] {
	return values.GetOrCreate(&c.structSet, (*TemplateTest).newStructSet, c)
}

func (c *TemplateTest) newStructSet() values.Set[*StructType] {
	return values.NewSet(c.logger.L, c.store, c.key.Append("StructSet"), c.label+" "+"struct set", values.Struct[StructType](), func(u, v *StructType) int { return u.Compare(v) })
}

func (c *TemplateTest) UnionSet() values.Set[UnionType] {
	return values.GetOrCreate(&c.unionSet, (*TemplateTest).newUnionSet, c)
}

func (c *TemplateTest) newUnionSet() values.Set[UnionType] {
	return values.NewSet(c.logger.L, c.store, c.key.Append("UnionSet"), c.label+" "+"union set", values.Union(UnmarshalUnionType), func(u, v UnionType) int { return u.Compare(v) })
}

func (c *TemplateTest) WrappedList() values.Counted[string] {
	return values.GetOrCreate(&c.wrappedList, (*TemplateTest).newWrappedList, c)
}

func (c *TemplateTest) newWrappedList() values.Counted[string] {
	return values.NewCounted(c.logger.L, c.store, c.key.Append("WrappedList"), c.label+" "+"wrapped list", values.WrappedFactory(values.StringWrapper))
}

func (c *TemplateTest) StructList() values.Counted[*StructType] {
	return values.GetOrCreate(&c.structList, (*TemplateTest).newStructList, c)
}

func (c *TemplateTest) newStructList() values.Counted[*StructType] {
	return values.NewCounted(c.logger.L, c.store, c.key.Append("StructList"), c.label+" "+"struct list", values.Struct[StructType])
}

func (c *TemplateTest) UnionList() values.Counted[UnionType] {
	return values.GetOrCreate(&c.unionList, (*TemplateTest).newUnionList, c)
}

func (c *TemplateTest) newUnionList() values.Counted[UnionType] {
	return values.NewCounted(c.logger.L, c.store, c.key.Append("UnionList"), c.label+" "+"union list", values.UnionFactory(UnmarshalUnionType))
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

	if values.IsDirty(c.wrapped) {
		return true
	}
	if values.IsDirty(c.structPtr) {
		return true
	}
	if values.IsDirty(c.union) {
		return true
	}
	if values.IsDirty(c.wrappedSet) {
		return true
	}
	if values.IsDirty(c.structSet) {
		return true
	}
	if values.IsDirty(c.unionSet) {
		return true
	}
	if values.IsDirty(c.wrappedList) {
		return true
	}
	if values.IsDirty(c.structList) {
		return true
	}
	if values.IsDirty(c.unionList) {
		return true
	}

	return false
}

func (c *TemplateTest) Walk(opts record.WalkOptions, fn record.WalkFunc) error {
	if c == nil {
		return nil
	}

	skip, err := values.WalkComposite(c, opts, fn)
	if skip || err != nil {
		return errors.UnknownError.Wrap(err)
	}
	values.WalkField(&err, c.wrapped, c.Wrapped, opts, fn)
	values.WalkField(&err, c.structPtr, c.StructPtr, opts, fn)
	values.WalkField(&err, c.union, c.Union, opts, fn)
	values.WalkField(&err, c.wrappedSet, c.WrappedSet, opts, fn)
	values.WalkField(&err, c.structSet, c.StructSet, opts, fn)
	values.WalkField(&err, c.unionSet, c.UnionSet, opts, fn)
	values.WalkField(&err, c.wrappedList, c.WrappedList, opts, fn)
	values.WalkField(&err, c.structList, c.StructList, opts, fn)
	values.WalkField(&err, c.unionList, c.UnionList, opts, fn)
	return err
}

func (c *TemplateTest) Commit() error {
	if c == nil {
		return nil
	}

	var err error
	values.Commit(&err, c.wrapped)
	values.Commit(&err, c.structPtr)
	values.Commit(&err, c.union)
	values.Commit(&err, c.wrappedSet)
	values.Commit(&err, c.structSet)
	values.Commit(&err, c.unionSet)
	values.Commit(&err, c.wrappedList)
	values.Commit(&err, c.structList)
	values.Commit(&err, c.unionList)

	return err
}
