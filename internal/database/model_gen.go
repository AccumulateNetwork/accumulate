// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

// GENERATED BY go run ./tools/cmd/gen-model. DO NOT EDIT.

//lint:file-ignore S1008,U1000 generated code

import (
	"encoding/hex"
	"strconv"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Batch struct {
	logger      logging.OptionalLogger
	store       record.Store
	done        bool
	writable    bool
	id          string
	nextChildId int64
	parent      *Batch
	kvstore     storage.KeyValueTxn
	bptEntries  map[storage.Key][32]byte

	account     map[accountKey]*Account
	transaction map[transactionKey]*Transaction
	systemData  map[systemDataKey]*SystemData
}

type accountKey struct {
	Url [32]byte
}

func keyForAccount(url *url.URL) accountKey {
	return accountKey{record.MapKeyUrl(url)}
}

type transactionKey struct {
	Hash [32]byte
}

func keyForTransaction(hash [32]byte) transactionKey {
	return transactionKey{hash}
}

type systemDataKey struct {
	Partition string
}

func keyForSystemData(partition string) systemDataKey {
	return systemDataKey{partition}
}

func (c *Batch) getAccount(url *url.URL) *Account {
	return getOrCreateMap(&c.account, keyForAccount(url), func() *Account {
		v := new(Account)
		v.logger = c.logger
		v.store = c.store
		v.key = record.Key{}.Append("Account", url)
		v.parent = c
		v.label = "account" + " " + url.RawString()
		return v
	})
}

func (c *Batch) getTransaction(hash [32]byte) *Transaction {
	return getOrCreateMap(&c.transaction, keyForTransaction(hash), func() *Transaction {
		v := new(Transaction)
		v.logger = c.logger
		v.store = c.store
		v.key = record.Key{}.Append("Transaction", hash)
		v.parent = c
		v.label = "transaction" + " " + hex.EncodeToString(hash[:])
		return v
	})
}

func (c *Batch) SystemData(partition string) *SystemData {
	return getOrCreateMap(&c.systemData, keyForSystemData(partition), func() *SystemData {
		v := new(SystemData)
		v.logger = c.logger
		v.store = c.store
		v.key = record.Key{}.Append("SystemData", partition)
		v.parent = c
		v.label = "system data" + " " + partition
		return v
	})
}

func (c *Batch) Resolve(key record.Key) (record.Record, record.Key, error) {
	if len(key) == 0 {
		return nil, nil, errors.InternalError.With("bad key for batch")
	}

	switch key[0] {
	case "Account":
		if len(key) < 2 {
			return nil, nil, errors.InternalError.With("bad key for batch")
		}
		url, okUrl := key[1].(*url.URL)
		if !okUrl {
			return nil, nil, errors.InternalError.With("bad key for batch")
		}
		v := c.getAccount(url)
		return v, key[2:], nil
	case "Transaction":
		if len(key) < 2 {
			return nil, nil, errors.InternalError.With("bad key for batch")
		}
		hash, okHash := key[1].([32]byte)
		if !okHash {
			return nil, nil, errors.InternalError.With("bad key for batch")
		}
		v := c.getTransaction(hash)
		return v, key[2:], nil
	case "SystemData":
		if len(key) < 2 {
			return nil, nil, errors.InternalError.With("bad key for batch")
		}
		partition, okPartition := key[1].(string)
		if !okPartition {
			return nil, nil, errors.InternalError.With("bad key for batch")
		}
		v := c.SystemData(partition)
		return v, key[2:], nil
	default:
		return nil, nil, errors.InternalError.With("bad key for batch")
	}
}

func (c *Batch) IsDirty() bool {
	if c == nil {
		return false
	}

	for _, v := range c.account {
		if v.IsDirty() {
			return true
		}
	}
	for _, v := range c.transaction {
		if v.IsDirty() {
			return true
		}
	}
	for _, v := range c.systemData {
		if v.IsDirty() {
			return true
		}
	}

	return false
}

func (c *Batch) dirtyChains() []*MerkleManager {
	if c == nil {
		return nil
	}

	var chains []*MerkleManager

	for _, v := range c.account {
		chains = append(chains, v.dirtyChains()...)
	}

	return chains
}

func (c *Batch) baseCommit() error {
	if c == nil {
		return nil
	}

	var err error
	for _, v := range c.account {
		commitField(&err, v)
	}
	for _, v := range c.transaction {
		commitField(&err, v)
	}
	for _, v := range c.systemData {
		commitField(&err, v)
	}

	return err
}

type Account struct {
	logger logging.OptionalLogger
	store  record.Store
	key    record.Key
	label  string
	parent *Batch

	url                    record.Value[*url.URL]
	main                   record.Value[protocol.Account]
	pending                record.Set[*url.TxID]
	syntheticForAnchor     map[accountSyntheticForAnchorKey]record.Set[*url.TxID]
	directory              record.Set[*url.URL]
	mainChain              *Chain2
	scratchChain           *Chain2
	signatureChain         *Chain2
	rootChain              *Chain2
	anchorSequenceChain    *Chain2
	majorBlockChain        *Chain2
	syntheticSequenceChain map[accountSyntheticSequenceChainKey]*Chain2
	anchorChain            map[accountAnchorChainKey]*AccountAnchorChain
	chains                 record.Set[*protocol.ChainMetadata]
	syntheticAnchors       record.Set[[32]byte]
	data                   *AccountData
}

type accountSyntheticForAnchorKey struct {
	Anchor [32]byte
}

func keyForAccountSyntheticForAnchor(anchor [32]byte) accountSyntheticForAnchorKey {
	return accountSyntheticForAnchorKey{anchor}
}

type accountSyntheticSequenceChainKey struct {
	Partition string
}

func keyForAccountSyntheticSequenceChain(partition string) accountSyntheticSequenceChainKey {
	return accountSyntheticSequenceChainKey{partition}
}

type accountAnchorChainKey struct {
	Partition string
}

func keyForAccountAnchorChain(partition string) accountAnchorChainKey {
	return accountAnchorChainKey{partition}
}

func (c *Account) getUrl() record.Value[*url.URL] {
	return getOrCreateField(&c.url, func() record.Value[*url.URL] {
		return record.NewValue(c.logger.L, c.store, c.key.Append("Url"), c.label+" "+"url", false, record.Wrapped(record.UrlWrapper))
	})
}

func (c *Account) Main() record.Value[protocol.Account] {
	return getOrCreateField(&c.main, func() record.Value[protocol.Account] {
		return record.NewValue(c.logger.L, c.store, c.key.Append("Main"), c.label+" "+"main", false, record.Union(protocol.UnmarshalAccount))
	})
}

func (c *Account) Pending() record.Set[*url.TxID] {
	return getOrCreateField(&c.pending, func() record.Set[*url.TxID] {
		return record.NewSet(c.logger.L, c.store, c.key.Append("Pending"), c.label+" "+"pending", record.Wrapped(record.TxidWrapper), record.CompareTxid)
	})
}

func (c *Account) SyntheticForAnchor(anchor [32]byte) record.Set[*url.TxID] {
	return getOrCreateMap(&c.syntheticForAnchor, keyForAccountSyntheticForAnchor(anchor), func() record.Set[*url.TxID] {
		return record.NewSet(c.logger.L, c.store, c.key.Append("SyntheticForAnchor", anchor), c.label+" "+"synthetic for anchor"+" "+hex.EncodeToString(anchor[:]), record.Wrapped(record.TxidWrapper), record.CompareTxid)
	})
}

func (c *Account) Directory() record.Set[*url.URL] {
	return getOrCreateField(&c.directory, func() record.Set[*url.URL] {
		return record.NewSet(c.logger.L, c.store, c.key.Append("Directory"), c.label+" "+"directory", record.Wrapped(record.UrlWrapper), record.CompareUrl)
	})
}

func (c *Account) MainChain() *Chain2 {
	return getOrCreateField(&c.mainChain, func() *Chain2 {
		return newChain2(c, c.logger.L, c.store, c.key.Append("MainChain"), "main", c.label+" "+"main chain")
	})
}

func (c *Account) ScratchChain() *Chain2 {
	return getOrCreateField(&c.scratchChain, func() *Chain2 {
		return newChain2(c, c.logger.L, c.store, c.key.Append("ScratchChain"), "scratch", c.label+" "+"scratch chain")
	})
}

func (c *Account) SignatureChain() *Chain2 {
	return getOrCreateField(&c.signatureChain, func() *Chain2 {
		return newChain2(c, c.logger.L, c.store, c.key.Append("SignatureChain"), "signature", c.label+" "+"signature chain")
	})
}

func (c *Account) RootChain() *Chain2 {
	return getOrCreateField(&c.rootChain, func() *Chain2 {
		return newChain2(c, c.logger.L, c.store, c.key.Append("RootChain"), "root", c.label+" "+"root chain")
	})
}

func (c *Account) AnchorSequenceChain() *Chain2 {
	return getOrCreateField(&c.anchorSequenceChain, func() *Chain2 {
		return newChain2(c, c.logger.L, c.store, c.key.Append("AnchorSequenceChain"), "anchor-sequence", c.label+" "+"anchor sequence chain")
	})
}

func (c *Account) MajorBlockChain() *Chain2 {
	return getOrCreateField(&c.majorBlockChain, func() *Chain2 {
		return newChain2(c, c.logger.L, c.store, c.key.Append("MajorBlockChain"), "major-block", c.label+" "+"major block chain")
	})
}

func (c *Account) getSyntheticSequenceChain(partition string) *Chain2 {
	return getOrCreateMap(&c.syntheticSequenceChain, keyForAccountSyntheticSequenceChain(partition), func() *Chain2 {
		return newChain2(c, c.logger.L, c.store, c.key.Append("SyntheticSequenceChain", partition), "synthetic-sequence(%[4]v)", c.label+" "+"synthetic sequence chain"+" "+partition)
	})
}

func (c *Account) getAnchorChain(partition string) *AccountAnchorChain {
	return getOrCreateMap(&c.anchorChain, keyForAccountAnchorChain(partition), func() *AccountAnchorChain {
		v := new(AccountAnchorChain)
		v.logger = c.logger
		v.store = c.store
		v.key = c.key.Append("AnchorChain", partition)
		v.parent = c
		v.label = c.label + " " + "anchor chain" + " " + partition
		return v
	})
}

func (c *Account) Chains() record.Set[*protocol.ChainMetadata] {
	return getOrCreateField(&c.chains, func() record.Set[*protocol.ChainMetadata] {
		return record.NewSet(c.logger.L, c.store, c.key.Append("Chains"), c.label+" "+"chains", record.Struct[protocol.ChainMetadata](), func(u, v *protocol.ChainMetadata) int { return u.Compare(v) })
	})
}

func (c *Account) SyntheticAnchors() record.Set[[32]byte] {
	return getOrCreateField(&c.syntheticAnchors, func() record.Set[[32]byte] {
		return record.NewSet(c.logger.L, c.store, c.key.Append("SyntheticAnchors"), c.label+" "+"synthetic anchors", record.Wrapped(record.HashWrapper), record.CompareHash)
	})
}

func (c *Account) Data() *AccountData {
	return getOrCreateField(&c.data, func() *AccountData {
		v := new(AccountData)
		v.logger = c.logger
		v.store = c.store
		v.key = c.key.Append("Data")
		v.parent = c
		v.label = c.label + " " + "data"
		return v
	})
}

func (c *Account) Resolve(key record.Key) (record.Record, record.Key, error) {
	if len(key) == 0 {
		return nil, nil, errors.InternalError.With("bad key for account")
	}

	switch key[0] {
	case "Url":
		return c.getUrl(), key[1:], nil
	case "Main":
		return c.Main(), key[1:], nil
	case "Pending":
		return c.Pending(), key[1:], nil
	case "SyntheticForAnchor":
		if len(key) < 2 {
			return nil, nil, errors.InternalError.With("bad key for account")
		}
		anchor, okAnchor := key[1].([32]byte)
		if !okAnchor {
			return nil, nil, errors.InternalError.With("bad key for account")
		}
		v := c.SyntheticForAnchor(anchor)
		return v, key[2:], nil
	case "Directory":
		return c.Directory(), key[1:], nil
	case "MainChain":
		return c.MainChain(), key[1:], nil
	case "ScratchChain":
		return c.ScratchChain(), key[1:], nil
	case "SignatureChain":
		return c.SignatureChain(), key[1:], nil
	case "RootChain":
		return c.RootChain(), key[1:], nil
	case "AnchorSequenceChain":
		return c.AnchorSequenceChain(), key[1:], nil
	case "MajorBlockChain":
		return c.MajorBlockChain(), key[1:], nil
	case "SyntheticSequenceChain":
		if len(key) < 2 {
			return nil, nil, errors.InternalError.With("bad key for account")
		}
		partition, okPartition := key[1].(string)
		if !okPartition {
			return nil, nil, errors.InternalError.With("bad key for account")
		}
		v := c.getSyntheticSequenceChain(partition)
		return v, key[2:], nil
	case "AnchorChain":
		if len(key) < 2 {
			return nil, nil, errors.InternalError.With("bad key for account")
		}
		partition, okPartition := key[1].(string)
		if !okPartition {
			return nil, nil, errors.InternalError.With("bad key for account")
		}
		v := c.getAnchorChain(partition)
		return v, key[2:], nil
	case "Chains":
		return c.Chains(), key[1:], nil
	case "SyntheticAnchors":
		return c.SyntheticAnchors(), key[1:], nil
	case "Data":
		return c.Data(), key[1:], nil
	default:
		return nil, nil, errors.InternalError.With("bad key for account")
	}
}

func (c *Account) IsDirty() bool {
	if c == nil {
		return false
	}

	if fieldIsDirty(c.url) {
		return true
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
	if fieldIsDirty(c.directory) {
		return true
	}
	if fieldIsDirty(c.mainChain) {
		return true
	}
	if fieldIsDirty(c.scratchChain) {
		return true
	}
	if fieldIsDirty(c.signatureChain) {
		return true
	}
	if fieldIsDirty(c.rootChain) {
		return true
	}
	if fieldIsDirty(c.anchorSequenceChain) {
		return true
	}
	if fieldIsDirty(c.majorBlockChain) {
		return true
	}
	for _, v := range c.syntheticSequenceChain {
		if v.IsDirty() {
			return true
		}
	}
	for _, v := range c.anchorChain {
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
	if fieldIsDirty(c.data) {
		return true
	}

	return false
}

func (c *Account) dirtyChains() []*MerkleManager {
	if c == nil {
		return nil
	}

	var chains []*MerkleManager

	chains = append(chains, c.mainChain.dirtyChains()...)
	chains = append(chains, c.scratchChain.dirtyChains()...)
	chains = append(chains, c.signatureChain.dirtyChains()...)
	chains = append(chains, c.rootChain.dirtyChains()...)
	chains = append(chains, c.anchorSequenceChain.dirtyChains()...)
	chains = append(chains, c.majorBlockChain.dirtyChains()...)
	for _, v := range c.syntheticSequenceChain {
		chains = append(chains, v.dirtyChains()...)
	}
	for _, v := range c.anchorChain {
		chains = append(chains, v.dirtyChains()...)
	}

	return chains
}

func (c *Account) baseCommit() error {
	if c == nil {
		return nil
	}

	var err error
	commitField(&err, c.url)
	commitField(&err, c.main)
	commitField(&err, c.pending)
	for _, v := range c.syntheticForAnchor {
		commitField(&err, v)
	}
	commitField(&err, c.directory)
	commitField(&err, c.mainChain)
	commitField(&err, c.scratchChain)
	commitField(&err, c.signatureChain)
	commitField(&err, c.rootChain)
	commitField(&err, c.anchorSequenceChain)
	commitField(&err, c.majorBlockChain)
	for _, v := range c.syntheticSequenceChain {
		commitField(&err, v)
	}
	for _, v := range c.anchorChain {
		commitField(&err, v)
	}
	commitField(&err, c.chains)
	commitField(&err, c.syntheticAnchors)
	commitField(&err, c.data)

	return err
}

type AccountAnchorChain struct {
	logger logging.OptionalLogger
	store  record.Store
	key    record.Key
	label  string
	parent *Account

	root *Chain2
	bpt  *Chain2
}

func (c *AccountAnchorChain) Root() *Chain2 {
	return getOrCreateField(&c.root, func() *Chain2 {
		return newChain2(c, c.logger.L, c.store, c.key.Append("Root"), "anchor(%[4]v)-root", c.label+" "+"root")
	})
}

func (c *AccountAnchorChain) BPT() *Chain2 {
	return getOrCreateField(&c.bpt, func() *Chain2 {
		return newChain2(c, c.logger.L, c.store, c.key.Append("BPT"), "anchor(%[4]v)-bpt", c.label+" "+"bpt")
	})
}

func (c *AccountAnchorChain) Resolve(key record.Key) (record.Record, record.Key, error) {
	if len(key) == 0 {
		return nil, nil, errors.InternalError.With("bad key for anchor chain")
	}

	switch key[0] {
	case "Root":
		return c.Root(), key[1:], nil
	case "BPT":
		return c.BPT(), key[1:], nil
	default:
		return nil, nil, errors.InternalError.With("bad key for anchor chain")
	}
}

func (c *AccountAnchorChain) IsDirty() bool {
	if c == nil {
		return false
	}

	if fieldIsDirty(c.root) {
		return true
	}
	if fieldIsDirty(c.bpt) {
		return true
	}

	return false
}

func (c *AccountAnchorChain) dirtyChains() []*MerkleManager {
	if c == nil {
		return nil
	}

	var chains []*MerkleManager

	chains = append(chains, c.root.dirtyChains()...)
	chains = append(chains, c.bpt.dirtyChains()...)

	return chains
}

func (c *AccountAnchorChain) Commit() error {
	if c == nil {
		return nil
	}

	var err error
	commitField(&err, c.root)
	commitField(&err, c.bpt)

	return err
}

type AccountData struct {
	logger logging.OptionalLogger
	store  record.Store
	key    record.Key
	label  string
	parent *Account

	entry       record.Counted[[32]byte]
	transaction map[accountDataTransactionKey]record.Value[[32]byte]
}

type accountDataTransactionKey struct {
	EntryHash [32]byte
}

func keyForAccountDataTransaction(entryHash [32]byte) accountDataTransactionKey {
	return accountDataTransactionKey{entryHash}
}

func (c *AccountData) Entry() record.Counted[[32]byte] {
	return getOrCreateField(&c.entry, func() record.Counted[[32]byte] {
		return record.NewCounted(c.logger.L, c.store, c.key.Append("Entry"), c.label+" "+"entry", record.WrappedFactory(record.HashWrapper))
	})
}

func (c *AccountData) Transaction(entryHash [32]byte) record.Value[[32]byte] {
	return getOrCreateMap(&c.transaction, keyForAccountDataTransaction(entryHash), func() record.Value[[32]byte] {
		return record.NewValue(c.logger.L, c.store, c.key.Append("Transaction", entryHash), c.label+" "+"transaction"+" "+hex.EncodeToString(entryHash[:]), false, record.Wrapped(record.HashWrapper))
	})
}

func (c *AccountData) Resolve(key record.Key) (record.Record, record.Key, error) {
	if len(key) == 0 {
		return nil, nil, errors.InternalError.With("bad key for data")
	}

	switch key[0] {
	case "Entry":
		return c.Entry(), key[1:], nil
	case "Transaction":
		if len(key) < 2 {
			return nil, nil, errors.InternalError.With("bad key for data")
		}
		entryHash, okEntryHash := key[1].([32]byte)
		if !okEntryHash {
			return nil, nil, errors.InternalError.With("bad key for data")
		}
		v := c.Transaction(entryHash)
		return v, key[2:], nil
	default:
		return nil, nil, errors.InternalError.With("bad key for data")
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

	return err
}

type Transaction struct {
	logger logging.OptionalLogger
	store  record.Store
	key    record.Key
	label  string
	parent *Batch

	main       record.Value[*SigOrTxn]
	status     record.Value[*protocol.TransactionStatus]
	produced   record.Set[*url.TxID]
	signatures map[transactionSignaturesKey]record.Value[*sigSetData]
	chains     record.Set[*TransactionChainEntry]
}

type transactionSignaturesKey struct {
	Signer [32]byte
}

func keyForTransactionSignatures(signer *url.URL) transactionSignaturesKey {
	return transactionSignaturesKey{record.MapKeyUrl(signer)}
}

func (c *Transaction) Main() record.Value[*SigOrTxn] {
	return getOrCreateField(&c.main, func() record.Value[*SigOrTxn] {
		return record.NewValue(c.logger.L, c.store, c.key.Append("Main"), c.label+" "+"main", false, record.Struct[SigOrTxn]())
	})
}

func (c *Transaction) Status() record.Value[*protocol.TransactionStatus] {
	return getOrCreateField(&c.status, func() record.Value[*protocol.TransactionStatus] {
		return record.NewValue(c.logger.L, c.store, c.key.Append("Status"), c.label+" "+"status", true, record.Struct[protocol.TransactionStatus]())
	})
}

func (c *Transaction) Produced() record.Set[*url.TxID] {
	return getOrCreateField(&c.produced, func() record.Set[*url.TxID] {
		return record.NewSet(c.logger.L, c.store, c.key.Append("Produced"), c.label+" "+"produced", record.Wrapped(record.TxidWrapper), record.CompareTxid)
	})
}

func (c *Transaction) getSignatures(signer *url.URL) record.Value[*sigSetData] {
	return getOrCreateMap(&c.signatures, keyForTransactionSignatures(signer), func() record.Value[*sigSetData] {
		return record.NewValue(c.logger.L, c.store, c.key.Append("Signatures", signer), c.label+" "+"signatures"+" "+signer.RawString(), true, record.Struct[sigSetData]())
	})
}

func (c *Transaction) Chains() record.Set[*TransactionChainEntry] {
	return getOrCreateField(&c.chains, func() record.Set[*TransactionChainEntry] {
		return record.NewSet(c.logger.L, c.store, c.key.Append("Chains"), c.label+" "+"chains", record.Struct[TransactionChainEntry](), func(u, v *TransactionChainEntry) int { return u.Compare(v) })
	})
}

func (c *Transaction) Resolve(key record.Key) (record.Record, record.Key, error) {
	if len(key) == 0 {
		return nil, nil, errors.InternalError.With("bad key for transaction")
	}

	switch key[0] {
	case "Main":
		return c.Main(), key[1:], nil
	case "Status":
		return c.Status(), key[1:], nil
	case "Produced":
		return c.Produced(), key[1:], nil
	case "Signatures":
		if len(key) < 2 {
			return nil, nil, errors.InternalError.With("bad key for transaction")
		}
		signer, okSigner := key[1].(*url.URL)
		if !okSigner {
			return nil, nil, errors.InternalError.With("bad key for transaction")
		}
		v := c.getSignatures(signer)
		return v, key[2:], nil
	case "Chains":
		return c.Chains(), key[1:], nil
	default:
		return nil, nil, errors.InternalError.With("bad key for transaction")
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

	return err
}

type SystemData struct {
	logger logging.OptionalLogger
	store  record.Store
	key    record.Key
	label  string
	parent *Batch

	syntheticIndexIndex map[systemDataSyntheticIndexIndexKey]record.Value[uint64]
}

type systemDataSyntheticIndexIndexKey struct {
	Block uint64
}

func keyForSystemDataSyntheticIndexIndex(block uint64) systemDataSyntheticIndexIndexKey {
	return systemDataSyntheticIndexIndexKey{block}
}

func (c *SystemData) SyntheticIndexIndex(block uint64) record.Value[uint64] {
	return getOrCreateMap(&c.syntheticIndexIndex, keyForSystemDataSyntheticIndexIndex(block), func() record.Value[uint64] {
		return record.NewValue(c.logger.L, c.store, c.key.Append("SyntheticIndexIndex", block), c.label+" "+"synthetic index index"+" "+strconv.FormatUint(block, 10), false, record.Wrapped(record.UintWrapper))
	})
}

func (c *SystemData) Resolve(key record.Key) (record.Record, record.Key, error) {
	if len(key) == 0 {
		return nil, nil, errors.InternalError.With("bad key for system data")
	}

	switch key[0] {
	case "SyntheticIndexIndex":
		if len(key) < 2 {
			return nil, nil, errors.InternalError.With("bad key for system data")
		}
		block, okBlock := key[1].(uint64)
		if !okBlock {
			return nil, nil, errors.InternalError.With("bad key for system data")
		}
		v := c.SyntheticIndexIndex(block)
		return v, key[2:], nil
	default:
		return nil, nil, errors.InternalError.With("bad key for system data")
	}
}

func (c *SystemData) IsDirty() bool {
	if c == nil {
		return false
	}

	for _, v := range c.syntheticIndexIndex {
		if v.IsDirty() {
			return true
		}
	}

	return false
}

func (c *SystemData) Commit() error {
	if c == nil {
		return nil
	}

	var err error
	for _, v := range c.syntheticIndexIndex {
		commitField(&err, v)
	}

	return err
}

type MerkleManager struct {
	logger        logging.OptionalLogger
	store         record.Store
	key           record.Key
	label         string
	typ           merkle.ChainType
	name          string
	markPower     int64
	markFreq      int64
	markMask      int64
	initialHeight int64

	head         record.Value[*MerkleState]
	states       map[merkleManagerStatesKey]record.Value[*MerkleState]
	elementIndex map[merkleManagerElementIndexKey]record.Value[uint64]
	element      map[merkleManagerElementKey]record.Value[[]byte]
}

type merkleManagerStatesKey struct {
	Index uint64
}

func keyForMerkleManagerStates(index uint64) merkleManagerStatesKey {
	return merkleManagerStatesKey{index}
}

type merkleManagerElementIndexKey struct {
	Hash [32]byte
}

func keyForMerkleManagerElementIndex(hash []byte) merkleManagerElementIndexKey {
	return merkleManagerElementIndexKey{record.MapKeyBytes(hash)}
}

type merkleManagerElementKey struct {
	Index uint64
}

func keyForMerkleManagerElement(index uint64) merkleManagerElementKey {
	return merkleManagerElementKey{index}
}

func (c *MerkleManager) Head() record.Value[*MerkleState] {
	return getOrCreateField(&c.head, func() record.Value[*MerkleState] {
		return record.NewValue(c.logger.L, c.store, c.key.Append("Head"), c.label+" "+"head", true, record.Struct[MerkleState]())
	})
}

func (c *MerkleManager) States(index uint64) record.Value[*MerkleState] {
	return getOrCreateMap(&c.states, keyForMerkleManagerStates(index), func() record.Value[*MerkleState] {
		return record.NewValue(c.logger.L, c.store, c.key.Append("States", index), c.label+" "+"states"+" "+strconv.FormatUint(index, 10), false, record.Struct[MerkleState]())
	})
}

func (c *MerkleManager) ElementIndex(hash []byte) record.Value[uint64] {
	return getOrCreateMap(&c.elementIndex, keyForMerkleManagerElementIndex(hash), func() record.Value[uint64] {
		return record.NewValue(c.logger.L, c.store, c.key.Append("ElementIndex", hash), c.label+" "+"element index"+" "+hex.EncodeToString(hash), false, record.Wrapped(record.UintWrapper))
	})
}

func (c *MerkleManager) Element(index uint64) record.Value[[]byte] {
	return getOrCreateMap(&c.element, keyForMerkleManagerElement(index), func() record.Value[[]byte] {
		return record.NewValue(c.logger.L, c.store, c.key.Append("Element", index), c.label+" "+"element"+" "+strconv.FormatUint(index, 10), false, record.Wrapped(record.BytesWrapper))
	})
}

func (c *MerkleManager) Resolve(key record.Key) (record.Record, record.Key, error) {
	if len(key) == 0 {
		return nil, nil, errors.InternalError.With("bad key for merkle manager")
	}

	switch key[0] {
	case "Head":
		return c.Head(), key[1:], nil
	case "States":
		if len(key) < 2 {
			return nil, nil, errors.InternalError.With("bad key for merkle manager")
		}
		index, okIndex := key[1].(uint64)
		if !okIndex {
			return nil, nil, errors.InternalError.With("bad key for merkle manager")
		}
		v := c.States(index)
		return v, key[2:], nil
	case "ElementIndex":
		if len(key) < 2 {
			return nil, nil, errors.InternalError.With("bad key for merkle manager")
		}
		hash, okHash := key[1].([]byte)
		if !okHash {
			return nil, nil, errors.InternalError.With("bad key for merkle manager")
		}
		v := c.ElementIndex(hash)
		return v, key[2:], nil
	case "Element":
		if len(key) < 2 {
			return nil, nil, errors.InternalError.With("bad key for merkle manager")
		}
		index, okIndex := key[1].(uint64)
		if !okIndex {
			return nil, nil, errors.InternalError.With("bad key for merkle manager")
		}
		v := c.Element(index)
		return v, key[2:], nil
	default:
		return nil, nil, errors.InternalError.With("bad key for merkle manager")
	}
}

func (c *MerkleManager) IsDirty() bool {
	if c == nil {
		return false
	}

	if fieldIsDirty(c.head) {
		return true
	}
	for _, v := range c.states {
		if v.IsDirty() {
			return true
		}
	}
	for _, v := range c.elementIndex {
		if v.IsDirty() {
			return true
		}
	}
	for _, v := range c.element {
		if v.IsDirty() {
			return true
		}
	}

	return false
}

func (c *MerkleManager) Commit() error {
	if c == nil {
		return nil
	}

	var err error
	commitField(&err, c.head)
	for _, v := range c.states {
		commitField(&err, v)
	}
	for _, v := range c.elementIndex {
		commitField(&err, v)
	}
	for _, v := range c.element {
		commitField(&err, v)
	}

	return err
}

func getOrCreateField[T any](ptr *T, create func() T) T {
	var z T
	if any(*ptr) != any(z) {
		return *ptr
	}

	*ptr = create()
	return *ptr
}

func getOrCreateMap[T any, K comparable](ptr *map[K]T, key K, create func() T) T {
	if *ptr == nil {
		*ptr = map[K]T{}
	}

	if v, ok := (*ptr)[key]; ok {
		return v
	}

	v := create()
	(*ptr)[key] = v
	return v
}

func commitField[T record.Record](lastErr *error, field T) {
	var z T
	if *lastErr != nil || any(field) == any(z) {
		return
	}

	*lastErr = field.Commit()
}

func fieldIsDirty[T record.Record](field T) bool {
	var z T
	return any(field) != any(z) && field.IsDirty()
}
