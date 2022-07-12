package record_test

// GENERATED BY go run ./tools/cmd/gen-model. DO NOT EDIT.

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

type ChangeSet struct {
	logger logging.OptionalLogger
	store  record.Store

	entity    map[storage.Key]*Entity
	changeLog *record.Counted[string]
}

func (c *ChangeSet) Entity(name string) *Entity {
	return getOrCreateMap(&c.entity, record.Key{}.Append("Entity", name), func() *Entity {
		v := new(Entity)
		v.logger = c.logger
		v.store = c.store
		v.key = record.Key{}.Append("Entity", name)
		v.parent = c
		v.label = "entity %[2]v"
		return v
	})
}

func (c *ChangeSet) ChangeLog() *record.Counted[string] {
	return getOrCreateField(&c.changeLog, func() *record.Counted[string] {
		return record.NewCounted(c.logger.L, c.store, record.Key{}.Append("ChangeLog"), "change log", record.WrappedFactory(record.StringWrapper))
	})
}

func (c *ChangeSet) Resolve(key record.Key) (record.Record, record.Key, error) {
	switch key[0] {
	case "Entity":
		if len(key) < 2 {
			return nil, nil, errors.New(errors.StatusInternalError, "bad key for change set")
		}
		name, okName := key[1].(string)
		if !okName {
			return nil, nil, errors.New(errors.StatusInternalError, "bad key for change set")
		}
		v := c.Entity(name)
		return v, key[2:], nil
	case "ChangeLog":
		return c.ChangeLog(), key[1:], nil
	default:
		return nil, nil, errors.New(errors.StatusInternalError, "bad key for change set")
	}
}

func (c *ChangeSet) IsDirty() bool {
	if c == nil {
		return false
	}

	for _, v := range c.entity {
		if v.IsDirty() {
			return true
		}
	}
	if fieldIsDirty(c.changeLog) {
		return true
	}

	return false
}

func (c *ChangeSet) dirtyChains() []*managed.Chain {
	if c == nil {
		return nil
	}

	var chains []*managed.Chain

	for _, v := range c.entity {
		chains = append(chains, v.dirtyChains()...)
	}

	return chains
}

func (c *ChangeSet) Commit() error {
	if c == nil {
		return nil
	}

	var err error
	for _, v := range c.entity {
		commitField(&err, v)
	}
	commitField(&err, c.changeLog)

	return nil
}

type Entity struct {
	logger logging.OptionalLogger
	store  record.Store
	key    record.Key
	label  string
	parent *ChangeSet

	union            *record.Value[protocol.Account]
	set              *record.Set[*url.TxID]
	chain            *managed.Chain
	countableRefType *record.Counted[*protocol.Transaction]
	countableUnion   *record.Counted[protocol.Account]
}

func (c *Entity) Union() *record.Value[protocol.Account] {
	return getOrCreateField(&c.union, func() *record.Value[protocol.Account] {
		return record.NewValue(c.logger.L, c.store, c.key.Append("Union"), c.label+" union", false, record.Union(protocol.UnmarshalAccount))
	})
}

func (c *Entity) Set() *record.Set[*url.TxID] {
	return getOrCreateField(&c.set, func() *record.Set[*url.TxID] {
		return record.NewSet(c.logger.L, c.store, c.key.Append("Set"), c.label+" set", record.Wrapped(record.TxidWrapper), record.CompareTxid)
	})
}

func (c *Entity) Chain() *managed.Chain {
	return getOrCreateField(&c.chain, func() *managed.Chain {
		return managed.NewChain(c.logger.L, c.store, c.key.Append("Chain"), markPower, managed.ChainTypeTransaction, "entity(%[2]v)-", c.label+" chain")
	})
}

func (c *Entity) CountableRefType() *record.Counted[*protocol.Transaction] {
	return getOrCreateField(&c.countableRefType, func() *record.Counted[*protocol.Transaction] {
		return record.NewCounted(c.logger.L, c.store, c.key.Append("CountableRefType"), c.label+" countable ref type", record.Struct[protocol.Transaction])
	})
}

func (c *Entity) CountableUnion() *record.Counted[protocol.Account] {
	return getOrCreateField(&c.countableUnion, func() *record.Counted[protocol.Account] {
		return record.NewCounted(c.logger.L, c.store, c.key.Append("CountableUnion"), c.label+" countable union", record.UnionFactory(protocol.UnmarshalAccount))
	})
}

func (c *Entity) Resolve(key record.Key) (record.Record, record.Key, error) {
	switch key[0] {
	case "Union":
		return c.Union(), key[1:], nil
	case "Set":
		return c.Set(), key[1:], nil
	case "Chain":
		return c.Chain(), key[1:], nil
	case "CountableRefType":
		return c.CountableRefType(), key[1:], nil
	case "CountableUnion":
		return c.CountableUnion(), key[1:], nil
	default:
		return nil, nil, errors.New(errors.StatusInternalError, "bad key for entity")
	}
}

func (c *Entity) IsDirty() bool {
	if c == nil {
		return false
	}

	if fieldIsDirty(c.union) {
		return true
	}
	if fieldIsDirty(c.set) {
		return true
	}
	if fieldIsDirty(c.chain) {
		return true
	}
	if fieldIsDirty(c.countableRefType) {
		return true
	}
	if fieldIsDirty(c.countableUnion) {
		return true
	}

	return false
}

func (c *Entity) resolveChain(name string) (chain *managed.Chain, ok bool) {
	if name == "" {
		return c.Chain(), true
	}
	return
}

func (c *Entity) dirtyChains() []*managed.Chain {
	if c == nil {
		return nil
	}

	var chains []*managed.Chain

	if fieldIsDirty(c.chain) {
		chains = append(chains, c.chain)
	}

	return chains
}

func (c *Entity) baseCommit() error {
	if c == nil {
		return nil
	}

	var err error
	commitField(&err, c.union)
	commitField(&err, c.set)
	commitField(&err, c.chain)
	commitField(&err, c.countableRefType)
	commitField(&err, c.countableUnion)

	return nil
}

type TemplateTest struct {
	logger logging.OptionalLogger
	store  record.Store
	key    record.Key
	label  string

	wrapped     *record.Value[string]
	structPtr   *record.Value[*StructType]
	union       *record.Value[UnionType]
	wrappedSet  *record.Set[*url.URL]
	structSet   *record.Set[*StructType]
	unionSet    *record.Set[UnionType]
	wrappedList *record.Counted[string]
	structList  *record.Counted[*StructType]
	unionList   *record.Counted[UnionType]
}

func (c *TemplateTest) Wrapped() *record.Value[string] {
	return getOrCreateField(&c.wrapped, func() *record.Value[string] {
		return record.NewValue(c.logger.L, c.store, c.key.Append("Wrapped"), c.label+" wrapped", false, record.Wrapped(record.StringWrapper))
	})
}

func (c *TemplateTest) StructPtr() *record.Value[*StructType] {
	return getOrCreateField(&c.structPtr, func() *record.Value[*StructType] {
		return record.NewValue(c.logger.L, c.store, c.key.Append("StructPtr"), c.label+" struct ptr", false, record.Struct[StructType]())
	})
}

func (c *TemplateTest) Union() *record.Value[UnionType] {
	return getOrCreateField(&c.union, func() *record.Value[UnionType] {
		return record.NewValue(c.logger.L, c.store, c.key.Append("Union"), c.label+" union", false, record.Union(UnmarshalUnionType))
	})
}

func (c *TemplateTest) WrappedSet() *record.Set[*url.URL] {
	return getOrCreateField(&c.wrappedSet, func() *record.Set[*url.URL] {
		return record.NewSet(c.logger.L, c.store, c.key.Append("WrappedSet"), c.label+" wrapped set", record.Wrapped(record.UrlWrapper), record.CompareUrl)
	})
}

func (c *TemplateTest) StructSet() *record.Set[*StructType] {
	return getOrCreateField(&c.structSet, func() *record.Set[*StructType] {
		return record.NewSet(c.logger.L, c.store, c.key.Append("StructSet"), c.label+" struct set", record.Struct[StructType](), func(u, v *StructType) int { return u.Compare(v) })
	})
}

func (c *TemplateTest) UnionSet() *record.Set[UnionType] {
	return getOrCreateField(&c.unionSet, func() *record.Set[UnionType] {
		return record.NewSet(c.logger.L, c.store, c.key.Append("UnionSet"), c.label+" union set", record.Union(UnmarshalUnionType), func(u, v UnionType) int { return u.Compare(v) })
	})
}

func (c *TemplateTest) WrappedList() *record.Counted[string] {
	return getOrCreateField(&c.wrappedList, func() *record.Counted[string] {
		return record.NewCounted(c.logger.L, c.store, c.key.Append("WrappedList"), c.label+" wrapped list", record.WrappedFactory(record.StringWrapper))
	})
}

func (c *TemplateTest) StructList() *record.Counted[*StructType] {
	return getOrCreateField(&c.structList, func() *record.Counted[*StructType] {
		return record.NewCounted(c.logger.L, c.store, c.key.Append("StructList"), c.label+" struct list", record.Struct[StructType])
	})
}

func (c *TemplateTest) UnionList() *record.Counted[UnionType] {
	return getOrCreateField(&c.unionList, func() *record.Counted[UnionType] {
		return record.NewCounted(c.logger.L, c.store, c.key.Append("UnionList"), c.label+" union list", record.UnionFactory(UnmarshalUnionType))
	})
}

func (c *TemplateTest) Resolve(key record.Key) (record.Record, record.Key, error) {
	switch key[0] {
	case "Wrapped":
		return c.Wrapped(), key[1:], nil
	case "StructPtr":
		return c.StructPtr(), key[1:], nil
	case "Union":
		return c.Union(), key[1:], nil
	case "WrappedSet":
		return c.WrappedSet(), key[1:], nil
	case "StructSet":
		return c.StructSet(), key[1:], nil
	case "UnionSet":
		return c.UnionSet(), key[1:], nil
	case "WrappedList":
		return c.WrappedList(), key[1:], nil
	case "StructList":
		return c.StructList(), key[1:], nil
	case "UnionList":
		return c.UnionList(), key[1:], nil
	default:
		return nil, nil, errors.New(errors.StatusInternalError, "bad key for template test")
	}
}

func (c *TemplateTest) IsDirty() bool {
	if c == nil {
		return false
	}

	if fieldIsDirty(c.wrapped) {
		return true
	}
	if fieldIsDirty(c.structPtr) {
		return true
	}
	if fieldIsDirty(c.union) {
		return true
	}
	if fieldIsDirty(c.wrappedSet) {
		return true
	}
	if fieldIsDirty(c.structSet) {
		return true
	}
	if fieldIsDirty(c.unionSet) {
		return true
	}
	if fieldIsDirty(c.wrappedList) {
		return true
	}
	if fieldIsDirty(c.structList) {
		return true
	}
	if fieldIsDirty(c.unionList) {
		return true
	}

	return false
}

func (c *TemplateTest) Commit() error {
	if c == nil {
		return nil
	}

	var err error
	commitField(&err, c.wrapped)
	commitField(&err, c.structPtr)
	commitField(&err, c.union)
	commitField(&err, c.wrappedSet)
	commitField(&err, c.structSet)
	commitField(&err, c.unionSet)
	commitField(&err, c.wrappedList)
	commitField(&err, c.structList)
	commitField(&err, c.unionList)

	return nil
}

func getOrCreateField[T any](ptr **T, create func() *T) *T {
	if *ptr != nil {
		return *ptr
	}

	*ptr = create()
	return *ptr
}

func getOrCreateMap[T any](ptr *map[storage.Key]T, key record.Key, create func() T) T {
	if *ptr == nil {
		*ptr = map[storage.Key]T{}
	}

	k := key.Hash()
	if v, ok := (*ptr)[k]; ok {
		return v
	}

	v := create()
	(*ptr)[k] = v
	return v
}

func commitField[T any, PT record.RecordPtr[T]](lastErr *error, field PT) {
	if *lastErr != nil || field == nil {
		return
	}

	*lastErr = field.Commit()
}

func fieldIsDirty[T any, PT record.RecordPtr[T]](field PT) bool {
	return field != nil && field.IsDirty()
}
