package main

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SigOrTxn struct {
	fieldsSet   []bool
	Transaction *protocol.Transaction `json:"transaction,omitempty" form:"transaction" query:"transaction" validate:"required"`
	Signature   protocol.Signature    `json:"signature,omitempty" form:"signature" query:"signature" validate:"required"`
	Txid        *url.TxID             `json:"txid,omitempty" form:"txid" query:"txid" validate:"required"`
	extraData   []byte
}

type SigSetEntry struct {
	fieldsSet     []bool
	System        bool                   `json:"system,omitempty" form:"system" query:"system" validate:"required"`
	Type          protocol.SignatureType `json:"type,omitempty" form:"type" query:"type" validate:"required"`
	KeyEntryIndex uint64                 `json:"keyEntryIndex,omitempty" form:"keyEntryIndex" query:"keyEntryIndex" validate:"required"`
	SignatureHash [32]byte               `json:"signatureHash,omitempty" form:"signatureHash" query:"signatureHash" validate:"required"`
	extraData     []byte
}

type accountState struct {
	fieldsSet []bool
	// Main is the main state of the account.
	Main protocol.Account `json:"main,omitempty" form:"main" query:"main" validate:"required"`
	// Chains is the state of the account's chains.
	Chains []*merkleState `json:"chains,omitempty" form:"chains" query:"chains" validate:"required"`
	// Pending is the state of the account's pending transactions.
	Pending []*transactionState `json:"pending,omitempty" form:"pending" query:"pending" validate:"required"`
	// Transactions is the state of other transactions related to the account.
	Transactions []*transactionState `json:"transactions,omitempty" form:"transactions" query:"transactions" validate:"required"`
	extraData    []byte
}

type hashValue struct {
	fieldsSet []bool
	Value     []byte `json:"value,omitempty" form:"value" query:"value" validate:"required"`
	extraData []byte
}

type intValue struct {
	fieldsSet []bool
	Value     int64 `json:"value,omitempty" form:"value" query:"value" validate:"required"`
	extraData []byte
}

type merkleState struct {
	fieldsSet []bool
	Name      string             `json:"name,omitempty" form:"name" query:"name" validate:"required"`
	Type      protocol.ChainType `json:"type,omitempty" form:"type" query:"type" validate:"required"`
	Count     uint64             `json:"count,omitempty" form:"count" query:"count" validate:"required"`
	Pending   [][]byte           `json:"pending,omitempty" form:"pending" query:"pending" validate:"required"`
	Entries   [][]byte           `json:"entries,omitempty" form:"entries" query:"entries" validate:"required"`
	extraData []byte
}

type sigSetData struct {
	fieldsSet []bool
	Version   uint64        `json:"version,omitempty" form:"version" query:"version" validate:"required"`
	Entries   []SigSetEntry `json:"entries,omitempty" form:"entries" query:"entries" validate:"required"`
	extraData []byte
}

type transactionState struct {
	fieldsSet   []bool
	Transaction *protocol.Transaction       `json:"transaction,omitempty" form:"transaction" query:"transaction" validate:"required"`
	State       *protocol.TransactionStatus `json:"state,omitempty" form:"state" query:"state" validate:"required"`
	Signatures  []*sigSetData               `json:"signatures,omitempty" form:"signatures" query:"signatures" validate:"required"`
	extraData   []byte
}

type txSyntheticTxns struct {
	fieldsSet []bool
	Txids     []*url.TxID `json:"txids,omitempty" form:"txids" query:"txids" validate:"required"`
	extraData []byte
}

func (v *SigOrTxn) Copy() *SigOrTxn {
	u := new(SigOrTxn)

	if v.Transaction != nil {
		u.Transaction = (v.Transaction).Copy()
	}
	if v.Signature != nil {
		u.Signature = (v.Signature).CopyAsInterface().(protocol.Signature)
	}
	if v.Txid != nil {
		u.Txid = (v.Txid).Copy()
	}

	return u
}

func (v *SigOrTxn) CopyAsInterface() interface{} { return v.Copy() }

func (v *SigSetEntry) Copy() *SigSetEntry {
	u := new(SigSetEntry)

	u.System = v.System
	u.Type = v.Type
	u.KeyEntryIndex = v.KeyEntryIndex
	u.SignatureHash = v.SignatureHash

	return u
}

func (v *SigSetEntry) CopyAsInterface() interface{} { return v.Copy() }

func (v *accountState) Copy() *accountState {
	u := new(accountState)

	if v.Main != nil {
		u.Main = (v.Main).CopyAsInterface().(protocol.Account)
	}
	u.Chains = make([]*merkleState, len(v.Chains))
	for i, v := range v.Chains {
		if v != nil {
			u.Chains[i] = (v).Copy()
		}
	}
	u.Pending = make([]*transactionState, len(v.Pending))
	for i, v := range v.Pending {
		if v != nil {
			u.Pending[i] = (v).Copy()
		}
	}
	u.Transactions = make([]*transactionState, len(v.Transactions))
	for i, v := range v.Transactions {
		if v != nil {
			u.Transactions[i] = (v).Copy()
		}
	}

	return u
}

func (v *accountState) CopyAsInterface() interface{} { return v.Copy() }

func (v *hashValue) Copy() *hashValue {
	u := new(hashValue)

	u.Value = encoding.BytesCopy(v.Value)

	return u
}

func (v *hashValue) CopyAsInterface() interface{} { return v.Copy() }

func (v *intValue) Copy() *intValue {
	u := new(intValue)

	u.Value = v.Value

	return u
}

func (v *intValue) CopyAsInterface() interface{} { return v.Copy() }

func (v *merkleState) Copy() *merkleState {
	u := new(merkleState)

	u.Name = v.Name
	u.Type = v.Type
	u.Count = v.Count
	u.Pending = make([][]byte, len(v.Pending))
	for i, v := range v.Pending {
		u.Pending[i] = encoding.BytesCopy(v)
	}
	u.Entries = make([][]byte, len(v.Entries))
	for i, v := range v.Entries {
		u.Entries[i] = encoding.BytesCopy(v)
	}

	return u
}

func (v *merkleState) CopyAsInterface() interface{} { return v.Copy() }

func (v *sigSetData) Copy() *sigSetData {
	u := new(sigSetData)

	u.Version = v.Version
	u.Entries = make([]SigSetEntry, len(v.Entries))
	for i, v := range v.Entries {
		u.Entries[i] = *(&v).Copy()
	}

	return u
}

func (v *sigSetData) CopyAsInterface() interface{} { return v.Copy() }

func (v *transactionState) Copy() *transactionState {
	u := new(transactionState)

	if v.Transaction != nil {
		u.Transaction = (v.Transaction).Copy()
	}
	if v.State != nil {
		u.State = (v.State).Copy()
	}
	u.Signatures = make([]*sigSetData, len(v.Signatures))
	for i, v := range v.Signatures {
		if v != nil {
			u.Signatures[i] = (v).Copy()
		}
	}

	return u
}

func (v *transactionState) CopyAsInterface() interface{} { return v.Copy() }

func (v *txSyntheticTxns) Copy() *txSyntheticTxns {
	u := new(txSyntheticTxns)

	u.Txids = make([]*url.TxID, len(v.Txids))
	for i, v := range v.Txids {
		if v != nil {
			u.Txids[i] = (v).Copy()
		}
	}

	return u
}

func (v *txSyntheticTxns) CopyAsInterface() interface{} { return v.Copy() }

func (v *SigOrTxn) Equal(u *SigOrTxn) bool {
	switch {
	case v.Transaction == u.Transaction:
		// equal
	case v.Transaction == nil || u.Transaction == nil:
		return false
	case !((v.Transaction).Equal(u.Transaction)):
		return false
	}
	if !(protocol.EqualSignature(v.Signature, u.Signature)) {
		return false
	}
	switch {
	case v.Txid == u.Txid:
		// equal
	case v.Txid == nil || u.Txid == nil:
		return false
	case !((v.Txid).Equal(u.Txid)):
		return false
	}

	return true
}

func (v *SigSetEntry) Equal(u *SigSetEntry) bool {
	if !(v.System == u.System) {
		return false
	}
	if !(v.Type == u.Type) {
		return false
	}
	if !(v.KeyEntryIndex == u.KeyEntryIndex) {
		return false
	}
	if !(v.SignatureHash == u.SignatureHash) {
		return false
	}

	return true
}

func (v *accountState) Equal(u *accountState) bool {
	if !(protocol.EqualAccount(v.Main, u.Main)) {
		return false
	}
	if len(v.Chains) != len(u.Chains) {
		return false
	}
	for i := range v.Chains {
		if !((v.Chains[i]).Equal(u.Chains[i])) {
			return false
		}
	}
	if len(v.Pending) != len(u.Pending) {
		return false
	}
	for i := range v.Pending {
		if !((v.Pending[i]).Equal(u.Pending[i])) {
			return false
		}
	}
	if len(v.Transactions) != len(u.Transactions) {
		return false
	}
	for i := range v.Transactions {
		if !((v.Transactions[i]).Equal(u.Transactions[i])) {
			return false
		}
	}

	return true
}

func (v *hashValue) Equal(u *hashValue) bool {
	if !(bytes.Equal(v.Value, u.Value)) {
		return false
	}

	return true
}

func (v *intValue) Equal(u *intValue) bool {
	if !(v.Value == u.Value) {
		return false
	}

	return true
}

func (v *merkleState) Equal(u *merkleState) bool {
	if !(v.Name == u.Name) {
		return false
	}
	if !(v.Type == u.Type) {
		return false
	}
	if !(v.Count == u.Count) {
		return false
	}
	if len(v.Pending) != len(u.Pending) {
		return false
	}
	for i := range v.Pending {
		if !(bytes.Equal(v.Pending[i], u.Pending[i])) {
			return false
		}
	}
	if len(v.Entries) != len(u.Entries) {
		return false
	}
	for i := range v.Entries {
		if !(bytes.Equal(v.Entries[i], u.Entries[i])) {
			return false
		}
	}

	return true
}

func (v *sigSetData) Equal(u *sigSetData) bool {
	if !(v.Version == u.Version) {
		return false
	}
	if len(v.Entries) != len(u.Entries) {
		return false
	}
	for i := range v.Entries {
		if !((&v.Entries[i]).Equal(&u.Entries[i])) {
			return false
		}
	}

	return true
}

func (v *transactionState) Equal(u *transactionState) bool {
	switch {
	case v.Transaction == u.Transaction:
		// equal
	case v.Transaction == nil || u.Transaction == nil:
		return false
	case !((v.Transaction).Equal(u.Transaction)):
		return false
	}
	switch {
	case v.State == u.State:
		// equal
	case v.State == nil || u.State == nil:
		return false
	case !((v.State).Equal(u.State)):
		return false
	}
	if len(v.Signatures) != len(u.Signatures) {
		return false
	}
	for i := range v.Signatures {
		if !((v.Signatures[i]).Equal(u.Signatures[i])) {
			return false
		}
	}

	return true
}

func (v *txSyntheticTxns) Equal(u *txSyntheticTxns) bool {
	if len(v.Txids) != len(u.Txids) {
		return false
	}
	for i := range v.Txids {
		if !((v.Txids[i]).Equal(u.Txids[i])) {
			return false
		}
	}

	return true
}

var fieldNames_SigOrTxn = []string{
	1: "Transaction",
	2: "Signature",
	3: "Txid",
}

func (v *SigOrTxn) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(v.Transaction == nil) {
		writer.WriteValue(1, v.Transaction)
	}
	if !(v.Signature == nil) {
		writer.WriteValue(2, v.Signature)
	}
	if !(v.Txid == nil) {
		writer.WriteTxid(3, v.Txid)
	}

	_, _, err := writer.Reset(fieldNames_SigOrTxn)
	if err != nil {
		return nil, err
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), err
}

func (v *SigOrTxn) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Transaction is missing")
	} else if v.Transaction == nil {
		errs = append(errs, "field Transaction is not set")
	}
	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field Signature is missing")
	} else if v.Signature == nil {
		errs = append(errs, "field Signature is not set")
	}
	if len(v.fieldsSet) > 3 && !v.fieldsSet[3] {
		errs = append(errs, "field Txid is missing")
	} else if v.Txid == nil {
		errs = append(errs, "field Txid is not set")
	}

	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errors.New(errs[0])
	default:
		return errors.New(strings.Join(errs, "; "))
	}
}

var fieldNames_SigSetEntry = []string{
	1: "System",
	2: "Type",
	3: "KeyEntryIndex",
	4: "SignatureHash",
}

func (v *SigSetEntry) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(!v.System) {
		writer.WriteBool(1, v.System)
	}
	if !(v.Type == 0) {
		writer.WriteEnum(2, v.Type)
	}
	if !(v.KeyEntryIndex == 0) {
		writer.WriteUint(3, v.KeyEntryIndex)
	}
	if !(v.SignatureHash == ([32]byte{})) {
		writer.WriteHash(4, &v.SignatureHash)
	}

	_, _, err := writer.Reset(fieldNames_SigSetEntry)
	if err != nil {
		return nil, err
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), err
}

func (v *SigSetEntry) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field System is missing")
	} else if !v.System {
		errs = append(errs, "field System is not set")
	}
	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field Type is missing")
	} else if v.Type == 0 {
		errs = append(errs, "field Type is not set")
	}
	if len(v.fieldsSet) > 3 && !v.fieldsSet[3] {
		errs = append(errs, "field KeyEntryIndex is missing")
	} else if v.KeyEntryIndex == 0 {
		errs = append(errs, "field KeyEntryIndex is not set")
	}
	if len(v.fieldsSet) > 4 && !v.fieldsSet[4] {
		errs = append(errs, "field SignatureHash is missing")
	} else if v.SignatureHash == ([32]byte{}) {
		errs = append(errs, "field SignatureHash is not set")
	}

	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errors.New(errs[0])
	default:
		return errors.New(strings.Join(errs, "; "))
	}
}

var fieldNames_accountState = []string{
	1: "Main",
	2: "Chains",
	3: "Pending",
	4: "Transactions",
}

func (v *accountState) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(v.Main == nil) {
		writer.WriteValue(1, v.Main)
	}
	if !(len(v.Chains) == 0) {
		for _, v := range v.Chains {
			writer.WriteValue(2, v)
		}
	}
	if !(len(v.Pending) == 0) {
		for _, v := range v.Pending {
			writer.WriteValue(3, v)
		}
	}
	if !(len(v.Transactions) == 0) {
		for _, v := range v.Transactions {
			writer.WriteValue(4, v)
		}
	}

	_, _, err := writer.Reset(fieldNames_accountState)
	if err != nil {
		return nil, err
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), err
}

func (v *accountState) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Main is missing")
	} else if v.Main == nil {
		errs = append(errs, "field Main is not set")
	}
	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field Chains is missing")
	} else if len(v.Chains) == 0 {
		errs = append(errs, "field Chains is not set")
	}
	if len(v.fieldsSet) > 3 && !v.fieldsSet[3] {
		errs = append(errs, "field Pending is missing")
	} else if len(v.Pending) == 0 {
		errs = append(errs, "field Pending is not set")
	}
	if len(v.fieldsSet) > 4 && !v.fieldsSet[4] {
		errs = append(errs, "field Transactions is missing")
	} else if len(v.Transactions) == 0 {
		errs = append(errs, "field Transactions is not set")
	}

	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errors.New(errs[0])
	default:
		return errors.New(strings.Join(errs, "; "))
	}
}

var fieldNames_hashValue = []string{
	1: "Value",
}

func (v *hashValue) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(len(v.Value) == 0) {
		writer.WriteBytes(1, v.Value)
	}

	_, _, err := writer.Reset(fieldNames_hashValue)
	if err != nil {
		return nil, err
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), err
}

func (v *hashValue) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Value is missing")
	} else if len(v.Value) == 0 {
		errs = append(errs, "field Value is not set")
	}

	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errors.New(errs[0])
	default:
		return errors.New(strings.Join(errs, "; "))
	}
}

var fieldNames_intValue = []string{
	1: "Value",
}

func (v *intValue) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(v.Value == 0) {
		writer.WriteInt(1, v.Value)
	}

	_, _, err := writer.Reset(fieldNames_intValue)
	if err != nil {
		return nil, err
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), err
}

func (v *intValue) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Value is missing")
	} else if v.Value == 0 {
		errs = append(errs, "field Value is not set")
	}

	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errors.New(errs[0])
	default:
		return errors.New(strings.Join(errs, "; "))
	}
}

var fieldNames_merkleState = []string{
	1: "Name",
	2: "Type",
	3: "Count",
	4: "Pending",
	5: "Entries",
}

func (v *merkleState) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(len(v.Name) == 0) {
		writer.WriteString(1, v.Name)
	}
	if !(v.Type == 0) {
		writer.WriteEnum(2, v.Type)
	}
	if !(v.Count == 0) {
		writer.WriteUint(3, v.Count)
	}
	if !(len(v.Pending) == 0) {
		for _, v := range v.Pending {
			writer.WriteBytes(4, v)
		}
	}
	if !(len(v.Entries) == 0) {
		for _, v := range v.Entries {
			writer.WriteBytes(5, v)
		}
	}

	_, _, err := writer.Reset(fieldNames_merkleState)
	if err != nil {
		return nil, err
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), err
}

func (v *merkleState) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Name is missing")
	} else if len(v.Name) == 0 {
		errs = append(errs, "field Name is not set")
	}
	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field Type is missing")
	} else if v.Type == 0 {
		errs = append(errs, "field Type is not set")
	}
	if len(v.fieldsSet) > 3 && !v.fieldsSet[3] {
		errs = append(errs, "field Count is missing")
	} else if v.Count == 0 {
		errs = append(errs, "field Count is not set")
	}
	if len(v.fieldsSet) > 4 && !v.fieldsSet[4] {
		errs = append(errs, "field Pending is missing")
	} else if len(v.Pending) == 0 {
		errs = append(errs, "field Pending is not set")
	}
	if len(v.fieldsSet) > 5 && !v.fieldsSet[5] {
		errs = append(errs, "field Entries is missing")
	} else if len(v.Entries) == 0 {
		errs = append(errs, "field Entries is not set")
	}

	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errors.New(errs[0])
	default:
		return errors.New(strings.Join(errs, "; "))
	}
}

var fieldNames_sigSetData = []string{
	1: "Version",
	2: "Entries",
}

func (v *sigSetData) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(v.Version == 0) {
		writer.WriteUint(1, v.Version)
	}
	if !(len(v.Entries) == 0) {
		for _, v := range v.Entries {
			writer.WriteValue(2, &v)
		}
	}

	_, _, err := writer.Reset(fieldNames_sigSetData)
	if err != nil {
		return nil, err
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), err
}

func (v *sigSetData) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Version is missing")
	} else if v.Version == 0 {
		errs = append(errs, "field Version is not set")
	}
	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field Entries is missing")
	} else if len(v.Entries) == 0 {
		errs = append(errs, "field Entries is not set")
	}

	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errors.New(errs[0])
	default:
		return errors.New(strings.Join(errs, "; "))
	}
}

var fieldNames_transactionState = []string{
	1: "Transaction",
	2: "State",
	3: "Signatures",
}

func (v *transactionState) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(v.Transaction == nil) {
		writer.WriteValue(1, v.Transaction)
	}
	if !(v.State == nil) {
		writer.WriteValue(2, v.State)
	}
	if !(len(v.Signatures) == 0) {
		for _, v := range v.Signatures {
			writer.WriteValue(3, v)
		}
	}

	_, _, err := writer.Reset(fieldNames_transactionState)
	if err != nil {
		return nil, err
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), err
}

func (v *transactionState) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Transaction is missing")
	} else if v.Transaction == nil {
		errs = append(errs, "field Transaction is not set")
	}
	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field State is missing")
	} else if v.State == nil {
		errs = append(errs, "field State is not set")
	}
	if len(v.fieldsSet) > 3 && !v.fieldsSet[3] {
		errs = append(errs, "field Signatures is missing")
	} else if len(v.Signatures) == 0 {
		errs = append(errs, "field Signatures is not set")
	}

	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errors.New(errs[0])
	default:
		return errors.New(strings.Join(errs, "; "))
	}
}

var fieldNames_txSyntheticTxns = []string{
	1: "Txids",
}

func (v *txSyntheticTxns) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(len(v.Txids) == 0) {
		for _, v := range v.Txids {
			writer.WriteTxid(1, v)
		}
	}

	_, _, err := writer.Reset(fieldNames_txSyntheticTxns)
	if err != nil {
		return nil, err
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), err
}

func (v *txSyntheticTxns) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Txids is missing")
	} else if len(v.Txids) == 0 {
		errs = append(errs, "field Txids is not set")
	}

	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errors.New(errs[0])
	default:
		return errors.New(strings.Join(errs, "; "))
	}
}

func (v *SigOrTxn) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *SigOrTxn) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x := new(protocol.Transaction); reader.ReadValue(1, x.UnmarshalBinary) {
		v.Transaction = x
	}
	reader.ReadValue(2, func(b []byte) error {
		x, err := protocol.UnmarshalSignature(b)
		if err == nil {
			v.Signature = x
		}
		return err
	})
	if x, ok := reader.ReadTxid(3); ok {
		v.Txid = x
	}

	seen, err := reader.Reset(fieldNames_SigOrTxn)
	if err != nil {
		return err
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	return err
}

func (v *SigSetEntry) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *SigSetEntry) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadBool(1); ok {
		v.System = x
	}
	if x := new(protocol.SignatureType); reader.ReadEnum(2, x) {
		v.Type = *x
	}
	if x, ok := reader.ReadUint(3); ok {
		v.KeyEntryIndex = x
	}
	if x, ok := reader.ReadHash(4); ok {
		v.SignatureHash = *x
	}

	seen, err := reader.Reset(fieldNames_SigSetEntry)
	if err != nil {
		return err
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	return err
}

func (v *accountState) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *accountState) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	reader.ReadValue(1, func(b []byte) error {
		x, err := protocol.UnmarshalAccount(b)
		if err == nil {
			v.Main = x
		}
		return err
	})
	for {
		if x := new(merkleState); reader.ReadValue(2, x.UnmarshalBinary) {
			v.Chains = append(v.Chains, x)
		} else {
			break
		}
	}
	for {
		if x := new(transactionState); reader.ReadValue(3, x.UnmarshalBinary) {
			v.Pending = append(v.Pending, x)
		} else {
			break
		}
	}
	for {
		if x := new(transactionState); reader.ReadValue(4, x.UnmarshalBinary) {
			v.Transactions = append(v.Transactions, x)
		} else {
			break
		}
	}

	seen, err := reader.Reset(fieldNames_accountState)
	if err != nil {
		return err
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	return err
}

func (v *hashValue) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *hashValue) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadBytes(1); ok {
		v.Value = x
	}

	seen, err := reader.Reset(fieldNames_hashValue)
	if err != nil {
		return err
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	return err
}

func (v *intValue) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *intValue) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadInt(1); ok {
		v.Value = x
	}

	seen, err := reader.Reset(fieldNames_intValue)
	if err != nil {
		return err
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	return err
}

func (v *merkleState) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *merkleState) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadString(1); ok {
		v.Name = x
	}
	if x := new(protocol.ChainType); reader.ReadEnum(2, x) {
		v.Type = *x
	}
	if x, ok := reader.ReadUint(3); ok {
		v.Count = x
	}
	for {
		if x, ok := reader.ReadBytes(4); ok {
			v.Pending = append(v.Pending, x)
		} else {
			break
		}
	}
	for {
		if x, ok := reader.ReadBytes(5); ok {
			v.Entries = append(v.Entries, x)
		} else {
			break
		}
	}

	seen, err := reader.Reset(fieldNames_merkleState)
	if err != nil {
		return err
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	return err
}

func (v *sigSetData) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *sigSetData) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadUint(1); ok {
		v.Version = x
	}
	for {
		if x := new(SigSetEntry); reader.ReadValue(2, x.UnmarshalBinary) {
			v.Entries = append(v.Entries, *x)
		} else {
			break
		}
	}

	seen, err := reader.Reset(fieldNames_sigSetData)
	if err != nil {
		return err
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	return err
}

func (v *transactionState) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *transactionState) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x := new(protocol.Transaction); reader.ReadValue(1, x.UnmarshalBinary) {
		v.Transaction = x
	}
	if x := new(protocol.TransactionStatus); reader.ReadValue(2, x.UnmarshalBinary) {
		v.State = x
	}
	for {
		if x := new(sigSetData); reader.ReadValue(3, x.UnmarshalBinary) {
			v.Signatures = append(v.Signatures, x)
		} else {
			break
		}
	}

	seen, err := reader.Reset(fieldNames_transactionState)
	if err != nil {
		return err
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	return err
}

func (v *txSyntheticTxns) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *txSyntheticTxns) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	for {
		if x, ok := reader.ReadTxid(1); ok {
			v.Txids = append(v.Txids, x)
		} else {
			break
		}
	}

	seen, err := reader.Reset(fieldNames_txSyntheticTxns)
	if err != nil {
		return err
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	return err
}

func (v *SigOrTxn) MarshalJSON() ([]byte, error) {
	u := struct {
		Transaction *protocol.Transaction                          `json:"transaction,omitempty"`
		Signature   encoding.JsonUnmarshalWith[protocol.Signature] `json:"signature,omitempty"`
		Txid        *url.TxID                                      `json:"txid,omitempty"`
	}{}
	u.Transaction = v.Transaction
	u.Signature = encoding.JsonUnmarshalWith[protocol.Signature]{Value: v.Signature, Func: protocol.UnmarshalSignatureJSON}
	u.Txid = v.Txid
	return json.Marshal(&u)
}

func (v *SigSetEntry) MarshalJSON() ([]byte, error) {
	u := struct {
		System        bool                   `json:"system,omitempty"`
		Type          protocol.SignatureType `json:"type,omitempty"`
		KeyEntryIndex uint64                 `json:"keyEntryIndex,omitempty"`
		SignatureHash string                 `json:"signatureHash,omitempty"`
	}{}
	u.System = v.System
	u.Type = v.Type
	u.KeyEntryIndex = v.KeyEntryIndex
	u.SignatureHash = encoding.ChainToJSON(v.SignatureHash)
	return json.Marshal(&u)
}

func (v *accountState) MarshalJSON() ([]byte, error) {
	u := struct {
		Main         encoding.JsonUnmarshalWith[protocol.Account] `json:"main,omitempty"`
		Chains       encoding.JsonList[*merkleState]              `json:"chains,omitempty"`
		Pending      encoding.JsonList[*transactionState]         `json:"pending,omitempty"`
		Transactions encoding.JsonList[*transactionState]         `json:"transactions,omitempty"`
	}{}
	u.Main = encoding.JsonUnmarshalWith[protocol.Account]{Value: v.Main, Func: protocol.UnmarshalAccountJSON}
	u.Chains = v.Chains
	u.Pending = v.Pending
	u.Transactions = v.Transactions
	return json.Marshal(&u)
}

func (v *hashValue) MarshalJSON() ([]byte, error) {
	u := struct {
		Value *string `json:"value,omitempty"`
	}{}
	u.Value = encoding.BytesToJSON(v.Value)
	return json.Marshal(&u)
}

func (v *merkleState) MarshalJSON() ([]byte, error) {
	u := struct {
		Name    string                     `json:"name,omitempty"`
		Type    protocol.ChainType         `json:"type,omitempty"`
		Count   uint64                     `json:"count,omitempty"`
		Pending encoding.JsonList[*string] `json:"pending,omitempty"`
		Entries encoding.JsonList[*string] `json:"entries,omitempty"`
	}{}
	u.Name = v.Name
	u.Type = v.Type
	u.Count = v.Count
	u.Pending = make(encoding.JsonList[*string], len(v.Pending))
	for i, x := range v.Pending {
		u.Pending[i] = encoding.BytesToJSON(x)
	}
	u.Entries = make(encoding.JsonList[*string], len(v.Entries))
	for i, x := range v.Entries {
		u.Entries[i] = encoding.BytesToJSON(x)
	}
	return json.Marshal(&u)
}

func (v *sigSetData) MarshalJSON() ([]byte, error) {
	u := struct {
		Version uint64                         `json:"version,omitempty"`
		Entries encoding.JsonList[SigSetEntry] `json:"entries,omitempty"`
	}{}
	u.Version = v.Version
	u.Entries = v.Entries
	return json.Marshal(&u)
}

func (v *transactionState) MarshalJSON() ([]byte, error) {
	u := struct {
		Transaction *protocol.Transaction          `json:"transaction,omitempty"`
		State       *protocol.TransactionStatus    `json:"state,omitempty"`
		Signatures  encoding.JsonList[*sigSetData] `json:"signatures,omitempty"`
	}{}
	u.Transaction = v.Transaction
	u.State = v.State
	u.Signatures = v.Signatures
	return json.Marshal(&u)
}

func (v *txSyntheticTxns) MarshalJSON() ([]byte, error) {
	u := struct {
		Txids encoding.JsonList[*url.TxID] `json:"txids,omitempty"`
	}{}
	u.Txids = v.Txids
	return json.Marshal(&u)
}

func (v *SigOrTxn) UnmarshalJSON(data []byte) error {
	u := struct {
		Transaction *protocol.Transaction                          `json:"transaction,omitempty"`
		Signature   encoding.JsonUnmarshalWith[protocol.Signature] `json:"signature,omitempty"`
		Txid        *url.TxID                                      `json:"txid,omitempty"`
	}{}
	u.Transaction = v.Transaction
	u.Signature = encoding.JsonUnmarshalWith[protocol.Signature]{Value: v.Signature, Func: protocol.UnmarshalSignatureJSON}
	u.Txid = v.Txid
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Transaction = u.Transaction
	v.Signature = u.Signature.Value

	v.Txid = u.Txid
	return nil
}

func (v *SigSetEntry) UnmarshalJSON(data []byte) error {
	u := struct {
		System        bool                   `json:"system,omitempty"`
		Type          protocol.SignatureType `json:"type,omitempty"`
		KeyEntryIndex uint64                 `json:"keyEntryIndex,omitempty"`
		SignatureHash string                 `json:"signatureHash,omitempty"`
	}{}
	u.System = v.System
	u.Type = v.Type
	u.KeyEntryIndex = v.KeyEntryIndex
	u.SignatureHash = encoding.ChainToJSON(v.SignatureHash)
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.System = u.System
	v.Type = u.Type
	v.KeyEntryIndex = u.KeyEntryIndex
	if x, err := encoding.ChainFromJSON(u.SignatureHash); err != nil {
		return fmt.Errorf("error decoding SignatureHash: %w", err)
	} else {
		v.SignatureHash = x
	}
	return nil
}

func (v *accountState) UnmarshalJSON(data []byte) error {
	u := struct {
		Main         encoding.JsonUnmarshalWith[protocol.Account] `json:"main,omitempty"`
		Chains       encoding.JsonList[*merkleState]              `json:"chains,omitempty"`
		Pending      encoding.JsonList[*transactionState]         `json:"pending,omitempty"`
		Transactions encoding.JsonList[*transactionState]         `json:"transactions,omitempty"`
	}{}
	u.Main = encoding.JsonUnmarshalWith[protocol.Account]{Value: v.Main, Func: protocol.UnmarshalAccountJSON}
	u.Chains = v.Chains
	u.Pending = v.Pending
	u.Transactions = v.Transactions
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Main = u.Main.Value

	v.Chains = u.Chains
	v.Pending = u.Pending
	v.Transactions = u.Transactions
	return nil
}

func (v *hashValue) UnmarshalJSON(data []byte) error {
	u := struct {
		Value *string `json:"value,omitempty"`
	}{}
	u.Value = encoding.BytesToJSON(v.Value)
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	if x, err := encoding.BytesFromJSON(u.Value); err != nil {
		return fmt.Errorf("error decoding Value: %w", err)
	} else {
		v.Value = x
	}
	return nil
}

func (v *merkleState) UnmarshalJSON(data []byte) error {
	u := struct {
		Name    string                     `json:"name,omitempty"`
		Type    protocol.ChainType         `json:"type,omitempty"`
		Count   uint64                     `json:"count,omitempty"`
		Pending encoding.JsonList[*string] `json:"pending,omitempty"`
		Entries encoding.JsonList[*string] `json:"entries,omitempty"`
	}{}
	u.Name = v.Name
	u.Type = v.Type
	u.Count = v.Count
	u.Pending = make(encoding.JsonList[*string], len(v.Pending))
	for i, x := range v.Pending {
		u.Pending[i] = encoding.BytesToJSON(x)
	}
	u.Entries = make(encoding.JsonList[*string], len(v.Entries))
	for i, x := range v.Entries {
		u.Entries[i] = encoding.BytesToJSON(x)
	}
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Name = u.Name
	v.Type = u.Type
	v.Count = u.Count
	v.Pending = make([][]byte, len(u.Pending))
	for i, x := range u.Pending {
		if x, err := encoding.BytesFromJSON(x); err != nil {
			return fmt.Errorf("error decoding Pending: %w", err)
		} else {
			v.Pending[i] = x
		}
	}
	v.Entries = make([][]byte, len(u.Entries))
	for i, x := range u.Entries {
		if x, err := encoding.BytesFromJSON(x); err != nil {
			return fmt.Errorf("error decoding Entries: %w", err)
		} else {
			v.Entries[i] = x
		}
	}
	return nil
}

func (v *sigSetData) UnmarshalJSON(data []byte) error {
	u := struct {
		Version uint64                         `json:"version,omitempty"`
		Entries encoding.JsonList[SigSetEntry] `json:"entries,omitempty"`
	}{}
	u.Version = v.Version
	u.Entries = v.Entries
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Version = u.Version
	v.Entries = u.Entries
	return nil
}

func (v *transactionState) UnmarshalJSON(data []byte) error {
	u := struct {
		Transaction *protocol.Transaction          `json:"transaction,omitempty"`
		State       *protocol.TransactionStatus    `json:"state,omitempty"`
		Signatures  encoding.JsonList[*sigSetData] `json:"signatures,omitempty"`
	}{}
	u.Transaction = v.Transaction
	u.State = v.State
	u.Signatures = v.Signatures
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Transaction = u.Transaction
	v.State = u.State
	v.Signatures = u.Signatures
	return nil
}

func (v *txSyntheticTxns) UnmarshalJSON(data []byte) error {
	u := struct {
		Txids encoding.JsonList[*url.TxID] `json:"txids,omitempty"`
	}{}
	u.Txids = v.Txids
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Txids = u.Txids
	return nil
}
