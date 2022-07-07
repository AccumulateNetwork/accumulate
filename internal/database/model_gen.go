package database

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

type Account struct {
	logger  logging.OptionalLogger
	store   record.Store
	key     record.Key
	batch   *Batch
	label   string
	chains2 map[storage.Key]*managed.Chain

	main               *record.Value[protocol.Account]
	pending            *record.Set[*url.TxID]
	syntheticForAnchor map[storage.Key]*record.Set[*url.TxID]
	chains             *record.Set[*protocol.ChainMetadata]
	syntheticAnchors   *record.Set[[32]byte]
	directory          *record.Counted[*url.URL]
	data               *AccountData
}

func (c *Account) Main() *record.Value[protocol.Account] {
	return getOrCreateField(&c.main, func() *record.Value[protocol.Account] {
		return record.NewValue(c.logger.L, c.store, c.key.Append("Main"), c.label+" main", false,
			record.Union(protocol.UnmarshalAccount))
	})
}

func (c *Account) Pending() *record.Set[*url.TxID] {
	return getOrCreateField(&c.pending, func() *record.Set[*url.TxID] {
		return record.NewSet(c.logger.L, c.store, c.key.Append("Pending"), c.label+" pending",
			record.Wrapped(record.TxidWrapper), record.CompareTxid)
	})
}

func (c *Account) SyntheticForAnchor(anchor [32]byte) *record.Set[*url.TxID] {
	return getOrCreateMap(&c.syntheticForAnchor, c.key.Append("SyntheticForAnchor", anchor), func() *record.Set[*url.TxID] {
		return record.NewSet(c.logger.L, c.store, c.key.Append("SyntheticForAnchor", anchor), c.label+" synthetic for anchor",
			record.Wrapped(record.TxidWrapper), record.CompareTxid)
	})
}

func (c *Account) Chains() *record.Set[*protocol.ChainMetadata] {
	return getOrCreateField(&c.chains, func() *record.Set[*protocol.ChainMetadata] {
		return record.NewSet(c.logger.L, c.store, c.key.Append("Chains"), c.label+" chains",
			record.Struct[protocol.ChainMetadata](), func(u, v *protocol.ChainMetadata) int { return u.Compare(v) })
	})
}

func (c *Account) SyntheticAnchors() *record.Set[[32]byte] {
	return getOrCreateField(&c.syntheticAnchors, func() *record.Set[[32]byte] {
		return record.NewSet(c.logger.L, c.store, c.key.Append("SyntheticAnchors"), c.label+" synthetic anchors",
			record.Wrapped(record.HashWrapper), record.CompareHash)
	})
}

func (c *Account) Directory() *record.Counted[*url.URL] {
	return getOrCreateField(&c.directory, func() *record.Counted[*url.URL] {
		return record.NewCounted(c.logger.L, c.store, c.key.Append("Directory"), c.label+" directory", record.WrappedFactory(record.UrlWrapper))
	})
}

func (c *Account) Data() *AccountData {
	return getOrCreateField(&c.data, func() *AccountData {
		v := new(AccountData)
		v.logger = c.logger
		v.store = c.store
		v.key = c.key.Append("Data")
		v.parent = c
		return v
	})
}

func (c *Account) baseResolve(key record.Key) (record.Record, record.Key, error) {
	switch key[0] {
	case "Main":
		return c.Main(), key[1:], nil
	case "Pending":
		return c.Pending(), key[1:], nil
	case "SyntheticForAnchor":
		if len(key) < 2 {
			return nil, nil, errors.New(errors.StatusInternalError, "bad key for account")
		}
		anchor, okAnchor := key[1].([32]byte)
		if !okAnchor {
			return nil, nil, errors.New(errors.StatusInternalError, "bad key for account")
		}
		v := c.SyntheticForAnchor(anchor)
		return v, key[2:], nil
	case "Chains":
		return c.Chains(), key[1:], nil
	case "SyntheticAnchors":
		return c.SyntheticAnchors(), key[1:], nil
	case "Directory":
		return c.Directory(), key[1:], nil
	case "Data":
		return c.Data(), key[1:], nil
	default:
		return nil, nil, errors.New(errors.StatusInternalError, "bad key for account")
	}
}

func (c *Account) IsDirty() bool {
	if c == nil {
		return false
	}

	if fieldIsDirty(c.main) {
		return true
	}
	if fieldIsDirty(c.pending) {
		return true
	}
	for _, v := range c.syntheticForAnchor {
		if v.IsDirty() {
			return true
		}
	}
	if fieldIsDirty(c.chains) {
		return true
	}
	if fieldIsDirty(c.syntheticAnchors) {
		return true
	}
	if fieldIsDirty(c.directory) {
		return true
	}
	if fieldIsDirty(c.data) {
		return true
	}

	return false
}

func (c *Account) baseCommit() error {
	if c == nil {
		return nil
	}

	var err error
	commitField(&err, c.main)
	commitField(&err, c.pending)
	for _, v := range c.syntheticForAnchor {
		commitField(&err, v)
	}
	commitField(&err, c.chains)
	commitField(&err, c.syntheticAnchors)
	commitField(&err, c.directory)
	commitField(&err, c.data)

	return nil
}

type AccountData struct {
	logger logging.OptionalLogger
	store  record.Store
	key    record.Key
	parent *Account

	entry       *record.Counted[[32]byte]
	transaction map[storage.Key]*record.Value[[32]byte]
}

func (c *AccountData) Entry() *record.Counted[[32]byte] {
	return getOrCreateField(&c.entry, func() *record.Counted[[32]byte] {
		return record.NewCounted(c.logger.L, c.store, c.key.Append("Entry"), "data entry", record.WrappedFactory(record.HashWrapper))
	})
}

func (c *AccountData) Transaction(entryHash [32]byte) *record.Value[[32]byte] {
	return getOrCreateMap(&c.transaction, c.key.Append("Transaction", entryHash), func() *record.Value[[32]byte] {
		return record.NewValue(c.logger.L, c.store, c.key.Append("Transaction", entryHash), "data transaction %[3]x", false,
			record.Wrapped(record.HashWrapper))
	})
}

func (c *AccountData) Resolve(key record.Key) (record.Record, record.Key, error) {
	switch key[0] {
	case "Entry":
		return c.Entry(), key[1:], nil
	case "Transaction":
		if len(key) < 2 {
			return nil, nil, errors.New(errors.StatusInternalError, "bad key for data")
		}
		entryHash, okEntryHash := key[1].([32]byte)
		if !okEntryHash {
			return nil, nil, errors.New(errors.StatusInternalError, "bad key for data")
		}
		v := c.Transaction(entryHash)
		return v, key[2:], nil
	default:
		return nil, nil, errors.New(errors.StatusInternalError, "bad key for data")
	}
}

func (c *AccountData) IsDirty() bool {
	if c == nil {
		return false
	}

	if fieldIsDirty(c.entry) {
		return true
	}
	for _, v := range c.transaction {
		if v.IsDirty() {
			return true
		}
	}

	return false
}

func (c *AccountData) Commit() error {
	if c == nil {
		return nil
	}

	var err error
	commitField(&err, c.entry)
	for _, v := range c.transaction {
		commitField(&err, v)
	}

	return nil
}

type Transaction struct {
	logger logging.OptionalLogger
	store  record.Store
	key    record.Key
	batch  *Batch
	label  string

	main       *record.Value[*SigOrTxn]
	status     *record.Value[*protocol.TransactionStatus]
	produced   *record.Set[*url.TxID]
	signatures map[storage.Key]*record.Value[*sigSetData]
	chains     *record.Set[*TransactionChainEntry]
}

func (c *Transaction) Main() *record.Value[*SigOrTxn] {
	return getOrCreateField(&c.main, func() *record.Value[*SigOrTxn] {
		return record.NewValue(c.logger.L, c.store, c.key.Append("Main"), c.label+" main", false,
			record.Struct[SigOrTxn]())
	})
}

func (c *Transaction) Status() *record.Value[*protocol.TransactionStatus] {
	return getOrCreateField(&c.status, func() *record.Value[*protocol.TransactionStatus] {
		return record.NewValue(c.logger.L, c.store, c.key.Append("Status"), c.label+" status", true,
			record.Struct[protocol.TransactionStatus]())
	})
}

func (c *Transaction) Produced() *record.Set[*url.TxID] {
	return getOrCreateField(&c.produced, func() *record.Set[*url.TxID] {
		return record.NewSet(c.logger.L, c.store, c.key.Append("Produced"), c.label+" produced",
			record.Wrapped(record.TxidWrapper), record.CompareTxid)
	})
}

func (c *Transaction) getSignatures(signer *url.URL) *record.Value[*sigSetData] {
	return getOrCreateMap(&c.signatures, c.key.Append("Signatures", signer), func() *record.Value[*sigSetData] {
		return record.NewValue(c.logger.L, c.store, c.key.Append("Signatures", signer), c.label+" signatures", true,
			record.Struct[sigSetData]())
	})
}

func (c *Transaction) Chains() *record.Set[*TransactionChainEntry] {
	return getOrCreateField(&c.chains, func() *record.Set[*TransactionChainEntry] {
		return record.NewSet(c.logger.L, c.store, c.key.Append("Chains"), c.label+" chains",
			record.Struct[TransactionChainEntry](), func(u, v *TransactionChainEntry) int { return u.Compare(v) })
	})
}

func (c *Transaction) Resolve(key record.Key) (record.Record, record.Key, error) {
	switch key[0] {
	case "Main":
		return c.Main(), key[1:], nil
	case "Status":
		return c.Status(), key[1:], nil
	case "Produced":
		return c.Produced(), key[1:], nil
	case "Signatures":
		if len(key) < 2 {
			return nil, nil, errors.New(errors.StatusInternalError, "bad key for transaction")
		}
		signer, okSigner := key[1].(*url.URL)
		if !okSigner {
			return nil, nil, errors.New(errors.StatusInternalError, "bad key for transaction")
		}
		v := c.getSignatures(signer)
		return v, key[2:], nil
	case "Chains":
		return c.Chains(), key[1:], nil
	default:
		return nil, nil, errors.New(errors.StatusInternalError, "bad key for transaction")
	}
}

func (c *Transaction) IsDirty() bool {
	if c == nil {
		return false
	}

	if fieldIsDirty(c.main) {
		return true
	}
	if fieldIsDirty(c.status) {
		return true
	}
	if fieldIsDirty(c.produced) {
		return true
	}
	for _, v := range c.signatures {
		if v.IsDirty() {
			return true
		}
	}
	if fieldIsDirty(c.chains) {
		return true
	}

	return false
}

func (c *Transaction) Commit() error {
	if c == nil {
		return nil
	}

	var err error
	commitField(&err, c.main)
	commitField(&err, c.status)
	commitField(&err, c.produced)
	for _, v := range c.signatures {
		commitField(&err, v)
	}
	commitField(&err, c.chains)

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
