package snapshot

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

//lint:file-ignore S1001,S1002,S1008,SA4013 generated code

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Account struct {
	fieldsSet []bool
	Hash      [32]byte
	Url       *url.URL `json:"url,omitempty" form:"url" query:"url" validate:"required"`
	// Main is the main state of the account.
	Main protocol.Account `json:"main,omitempty" form:"main" query:"main" validate:"required"`
	// Chains is the state of the account's chains.
	Chains []*ChainState `json:"chains,omitempty" form:"chains" query:"chains" validate:"required"`
	// Pending is the state of the account's pending transactions.
	Pending []*url.TxID `json:"pending,omitempty" form:"pending" query:"pending" validate:"required"`
	// Directory lists the account's sub-accounts.
	Directory []*url.URL `json:"directory,omitempty" form:"directory" query:"directory" validate:"required"`
	extraData []byte
}

type Chain struct {
	fieldsSet []bool
	Name      string             `json:"name,omitempty" form:"name" query:"name" validate:"required"`
	Type      protocol.ChainType `json:"type,omitempty" form:"type" query:"type" validate:"required"`
	Count     uint64             `json:"count,omitempty" form:"count" query:"count" validate:"required"`
	Pending   [][]byte           `json:"pending,omitempty" form:"pending" query:"pending" validate:"required"`
	Entries   [][]byte           `json:"entries,omitempty" form:"entries" query:"entries" validate:"required"`
	extraData []byte
}

type Header struct {
	fieldsSet []bool
	// Version is the snapshot format version.
	Version uint64 `json:"version,omitempty" form:"version" query:"version" validate:"required"`
	// Height is the snapshot's block height.
	Height uint64 `json:"height,omitempty" form:"height" query:"height" validate:"required"`
	// RootHash is the snapshot's root hash.
	RootHash  [32]byte `json:"rootHash,omitempty" form:"rootHash" query:"rootHash" validate:"required"`
	extraData []byte
}

type Signature struct {
	fieldsSet []bool
	Txid      *url.TxID          `json:"txid,omitempty" form:"txid" query:"txid" validate:"required"`
	Signature protocol.Signature `json:"signature,omitempty" form:"signature" query:"signature" validate:"required"`
	extraData []byte
}

type Transaction struct {
	fieldsSet     []bool
	Transaction   *protocol.Transaction       `json:"transaction,omitempty" form:"transaction" query:"transaction" validate:"required"`
	Status        *protocol.TransactionStatus `json:"status,omitempty" form:"status" query:"status" validate:"required"`
	SignatureSets []*TxnSigSet                `json:"signatureSets,omitempty" form:"signatureSets" query:"signatureSets" validate:"required"`
	extraData     []byte
}

type TxnSigSet struct {
	fieldsSet []bool
	Signer    *url.URL               `json:"signer,omitempty" form:"signer" query:"signer" validate:"required"`
	Version   uint64                 `json:"version,omitempty" form:"version" query:"version" validate:"required"`
	Entries   []database.SigSetEntry `json:"entries,omitempty" form:"entries" query:"entries" validate:"required"`
	extraData []byte
}

type sigSection struct {
	fieldsSet  []bool
	Signatures []*Signature `json:"signatures,omitempty" form:"signatures" query:"signatures" validate:"required"`
	extraData  []byte
}

type txnSection struct {
	fieldsSet    []bool
	Transactions []*Transaction `json:"transactions,omitempty" form:"transactions" query:"transactions" validate:"required"`
	extraData    []byte
}

func (v *Account) Copy() *Account {
	u := new(Account)

	if v.Url != nil {
		u.Url = v.Url
	}
	if v.Main != nil {
		u.Main = (v.Main).CopyAsInterface().(protocol.Account)
	}
	u.Chains = make([]*ChainState, len(v.Chains))
	for i, v := range v.Chains {
		if v != nil {
			u.Chains[i] = (v).Copy()
		}
	}
	u.Pending = make([]*url.TxID, len(v.Pending))
	for i, v := range v.Pending {
		if v != nil {
			u.Pending[i] = v
		}
	}
	u.Directory = make([]*url.URL, len(v.Directory))
	for i, v := range v.Directory {
		if v != nil {
			u.Directory[i] = v
		}
	}

	return u
}

func (v *Account) CopyAsInterface() interface{} { return v.Copy() }

func (v *Chain) Copy() *Chain {
	u := new(Chain)

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

func (v *Chain) CopyAsInterface() interface{} { return v.Copy() }

func (v *Header) Copy() *Header {
	u := new(Header)

	u.Version = v.Version
	u.Height = v.Height
	u.RootHash = v.RootHash

	return u
}

func (v *Header) CopyAsInterface() interface{} { return v.Copy() }

func (v *Signature) Copy() *Signature {
	u := new(Signature)

	if v.Txid != nil {
		u.Txid = v.Txid
	}
	if v.Signature != nil {
		u.Signature = (v.Signature).CopyAsInterface().(protocol.Signature)
	}

	return u
}

func (v *Signature) CopyAsInterface() interface{} { return v.Copy() }

func (v *Transaction) Copy() *Transaction {
	u := new(Transaction)

	if v.Transaction != nil {
		u.Transaction = (v.Transaction).Copy()
	}
	if v.Status != nil {
		u.Status = (v.Status).Copy()
	}
	u.SignatureSets = make([]*TxnSigSet, len(v.SignatureSets))
	for i, v := range v.SignatureSets {
		if v != nil {
			u.SignatureSets[i] = (v).Copy()
		}
	}

	return u
}

func (v *Transaction) CopyAsInterface() interface{} { return v.Copy() }

func (v *TxnSigSet) Copy() *TxnSigSet {
	u := new(TxnSigSet)

	if v.Signer != nil {
		u.Signer = v.Signer
	}
	u.Version = v.Version
	u.Entries = make([]database.SigSetEntry, len(v.Entries))
	for i, v := range v.Entries {
		u.Entries[i] = *(&v).Copy()
	}

	return u
}

func (v *TxnSigSet) CopyAsInterface() interface{} { return v.Copy() }

func (v *sigSection) Copy() *sigSection {
	u := new(sigSection)

	u.Signatures = make([]*Signature, len(v.Signatures))
	for i, v := range v.Signatures {
		if v != nil {
			u.Signatures[i] = (v).Copy()
		}
	}

	return u
}

func (v *sigSection) CopyAsInterface() interface{} { return v.Copy() }

func (v *txnSection) Copy() *txnSection {
	u := new(txnSection)

	u.Transactions = make([]*Transaction, len(v.Transactions))
	for i, v := range v.Transactions {
		if v != nil {
			u.Transactions[i] = (v).Copy()
		}
	}

	return u
}

func (v *txnSection) CopyAsInterface() interface{} { return v.Copy() }

func (v *Account) Equal(u *Account) bool {
	switch {
	case v.Url == u.Url:
		// equal
	case v.Url == nil || u.Url == nil:
		return false
	case !((v.Url).Equal(u.Url)):
		return false
	}
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
	if len(v.Directory) != len(u.Directory) {
		return false
	}
	for i := range v.Directory {
		if !((v.Directory[i]).Equal(u.Directory[i])) {
			return false
		}
	}

	return true
}

func (v *Chain) Equal(u *Chain) bool {
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

func (v *Header) Equal(u *Header) bool {
	if !(v.Version == u.Version) {
		return false
	}
	if !(v.Height == u.Height) {
		return false
	}
	if !(v.RootHash == u.RootHash) {
		return false
	}

	return true
}

func (v *Signature) Equal(u *Signature) bool {
	switch {
	case v.Txid == u.Txid:
		// equal
	case v.Txid == nil || u.Txid == nil:
		return false
	case !((v.Txid).Equal(u.Txid)):
		return false
	}
	if !(protocol.EqualSignature(v.Signature, u.Signature)) {
		return false
	}

	return true
}

func (v *Transaction) Equal(u *Transaction) bool {
	switch {
	case v.Transaction == u.Transaction:
		// equal
	case v.Transaction == nil || u.Transaction == nil:
		return false
	case !((v.Transaction).Equal(u.Transaction)):
		return false
	}
	switch {
	case v.Status == u.Status:
		// equal
	case v.Status == nil || u.Status == nil:
		return false
	case !((v.Status).Equal(u.Status)):
		return false
	}
	if len(v.SignatureSets) != len(u.SignatureSets) {
		return false
	}
	for i := range v.SignatureSets {
		if !((v.SignatureSets[i]).Equal(u.SignatureSets[i])) {
			return false
		}
	}

	return true
}

func (v *TxnSigSet) Equal(u *TxnSigSet) bool {
	switch {
	case v.Signer == u.Signer:
		// equal
	case v.Signer == nil || u.Signer == nil:
		return false
	case !((v.Signer).Equal(u.Signer)):
		return false
	}
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

func (v *sigSection) Equal(u *sigSection) bool {
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

func (v *txnSection) Equal(u *txnSection) bool {
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

var fieldNames_Account = []string{
	1: "Url",
	2: "Main",
	3: "Chains",
	4: "Pending",
	5: "Directory",
}

func (v *Account) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(v.Url == nil) {
		writer.WriteUrl(1, v.Url)
	}
	if !(v.Main == nil) {
		writer.WriteValue(2, v.Main.MarshalBinary)
	}
	if !(len(v.Chains) == 0) {
		for _, v := range v.Chains {
			writer.WriteValue(3, v.MarshalBinary)
		}
	}
	if !(len(v.Pending) == 0) {
		for _, v := range v.Pending {
			writer.WriteTxid(4, v)
		}
	}
	if !(len(v.Directory) == 0) {
		for _, v := range v.Directory {
			writer.WriteUrl(5, v)
		}
	}

	_, _, err := writer.Reset(fieldNames_Account)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *Account) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Url is missing")
	} else if v.Url == nil {
		errs = append(errs, "field Url is not set")
	}
	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field Main is missing")
	} else if v.Main == nil {
		errs = append(errs, "field Main is not set")
	}
	if len(v.fieldsSet) > 3 && !v.fieldsSet[3] {
		errs = append(errs, "field Chains is missing")
	} else if len(v.Chains) == 0 {
		errs = append(errs, "field Chains is not set")
	}
	if len(v.fieldsSet) > 4 && !v.fieldsSet[4] {
		errs = append(errs, "field Pending is missing")
	} else if len(v.Pending) == 0 {
		errs = append(errs, "field Pending is not set")
	}
	if len(v.fieldsSet) > 5 && !v.fieldsSet[5] {
		errs = append(errs, "field Directory is missing")
	} else if len(v.Directory) == 0 {
		errs = append(errs, "field Directory is not set")
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

var fieldNames_Chain = []string{
	1: "Name",
	2: "Type",
	3: "Count",
	4: "Pending",
	5: "Entries",
}

func (v *Chain) MarshalBinary() ([]byte, error) {
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

	_, _, err := writer.Reset(fieldNames_Chain)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *Chain) IsValid() error {
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

var fieldNames_Header = []string{
	1: "Version",
	2: "Height",
	3: "RootHash",
}

func (v *Header) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(v.Version == 0) {
		writer.WriteUint(1, v.Version)
	}
	if !(v.Height == 0) {
		writer.WriteUint(2, v.Height)
	}
	if !(v.RootHash == ([32]byte{})) {
		writer.WriteHash(3, &v.RootHash)
	}

	_, _, err := writer.Reset(fieldNames_Header)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *Header) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Version is missing")
	} else if v.Version == 0 {
		errs = append(errs, "field Version is not set")
	}
	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field Height is missing")
	} else if v.Height == 0 {
		errs = append(errs, "field Height is not set")
	}
	if len(v.fieldsSet) > 3 && !v.fieldsSet[3] {
		errs = append(errs, "field RootHash is missing")
	} else if v.RootHash == ([32]byte{}) {
		errs = append(errs, "field RootHash is not set")
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

var fieldNames_Signature = []string{
	1: "Txid",
	2: "Signature",
}

func (v *Signature) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(v.Txid == nil) {
		writer.WriteTxid(1, v.Txid)
	}
	if !(v.Signature == nil) {
		writer.WriteValue(2, v.Signature.MarshalBinary)
	}

	_, _, err := writer.Reset(fieldNames_Signature)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *Signature) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Txid is missing")
	} else if v.Txid == nil {
		errs = append(errs, "field Txid is not set")
	}
	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field Signature is missing")
	} else if v.Signature == nil {
		errs = append(errs, "field Signature is not set")
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

var fieldNames_Transaction = []string{
	2: "Transaction",
	3: "Status",
	4: "SignatureSets",
}

func (v *Transaction) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(v.Transaction == nil) {
		writer.WriteValue(2, v.Transaction.MarshalBinary)
	}
	if !(v.Status == nil) {
		writer.WriteValue(3, v.Status.MarshalBinary)
	}
	if !(len(v.SignatureSets) == 0) {
		for _, v := range v.SignatureSets {
			writer.WriteValue(4, v.MarshalBinary)
		}
	}

	_, _, err := writer.Reset(fieldNames_Transaction)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *Transaction) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field Transaction is missing")
	} else if v.Transaction == nil {
		errs = append(errs, "field Transaction is not set")
	}
	if len(v.fieldsSet) > 3 && !v.fieldsSet[3] {
		errs = append(errs, "field Status is missing")
	} else if v.Status == nil {
		errs = append(errs, "field Status is not set")
	}
	if len(v.fieldsSet) > 4 && !v.fieldsSet[4] {
		errs = append(errs, "field SignatureSets is missing")
	} else if len(v.SignatureSets) == 0 {
		errs = append(errs, "field SignatureSets is not set")
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

var fieldNames_TxnSigSet = []string{
	1: "Signer",
	2: "Version",
	3: "Entries",
}

func (v *TxnSigSet) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(v.Signer == nil) {
		writer.WriteUrl(1, v.Signer)
	}
	if !(v.Version == 0) {
		writer.WriteUint(2, v.Version)
	}
	if !(len(v.Entries) == 0) {
		for _, v := range v.Entries {
			writer.WriteValue(3, v.MarshalBinary)
		}
	}

	_, _, err := writer.Reset(fieldNames_TxnSigSet)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *TxnSigSet) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Signer is missing")
	} else if v.Signer == nil {
		errs = append(errs, "field Signer is not set")
	}
	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field Version is missing")
	} else if v.Version == 0 {
		errs = append(errs, "field Version is not set")
	}
	if len(v.fieldsSet) > 3 && !v.fieldsSet[3] {
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

var fieldNames_sigSection = []string{
	1: "Signatures",
}

func (v *sigSection) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(len(v.Signatures) == 0) {
		for _, v := range v.Signatures {
			writer.WriteValue(1, v.MarshalBinary)
		}
	}

	_, _, err := writer.Reset(fieldNames_sigSection)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *sigSection) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
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

var fieldNames_txnSection = []string{
	1: "Transactions",
}

func (v *txnSection) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(len(v.Transactions) == 0) {
		for _, v := range v.Transactions {
			writer.WriteValue(1, v.MarshalBinary)
		}
	}

	_, _, err := writer.Reset(fieldNames_txnSection)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *txnSection) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
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

func (v *Account) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *Account) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadUrl(1); ok {
		v.Url = x
	}
	reader.ReadValue(2, func(b []byte) error {
		x, err := protocol.UnmarshalAccount(b)
		if err == nil {
			v.Main = x
		}
		return err
	})
	for {
		if x := new(ChainState); reader.ReadValue(3, x.UnmarshalBinary) {
			v.Chains = append(v.Chains, x)
		} else {
			break
		}
	}
	for {
		if x, ok := reader.ReadTxid(4); ok {
			v.Pending = append(v.Pending, x)
		} else {
			break
		}
	}
	for {
		if x, ok := reader.ReadUrl(5); ok {
			v.Directory = append(v.Directory, x)
		} else {
			break
		}
	}

	seen, err := reader.Reset(fieldNames_Account)
	if err != nil {
		return encoding.Error{E: err}
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	if err != nil {
		return encoding.Error{E: err}
	}
	return nil
}

func (v *Chain) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *Chain) UnmarshalBinaryFrom(rd io.Reader) error {
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

	seen, err := reader.Reset(fieldNames_Chain)
	if err != nil {
		return encoding.Error{E: err}
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	if err != nil {
		return encoding.Error{E: err}
	}
	return nil
}

func (v *Header) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *Header) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadUint(1); ok {
		v.Version = x
	}
	if x, ok := reader.ReadUint(2); ok {
		v.Height = x
	}
	if x, ok := reader.ReadHash(3); ok {
		v.RootHash = *x
	}

	seen, err := reader.Reset(fieldNames_Header)
	if err != nil {
		return encoding.Error{E: err}
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	if err != nil {
		return encoding.Error{E: err}
	}
	return nil
}

func (v *Signature) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *Signature) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadTxid(1); ok {
		v.Txid = x
	}
	reader.ReadValue(2, func(b []byte) error {
		x, err := protocol.UnmarshalSignature(b)
		if err == nil {
			v.Signature = x
		}
		return err
	})

	seen, err := reader.Reset(fieldNames_Signature)
	if err != nil {
		return encoding.Error{E: err}
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	if err != nil {
		return encoding.Error{E: err}
	}
	return nil
}

func (v *Transaction) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *Transaction) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x := new(protocol.Transaction); reader.ReadValue(2, x.UnmarshalBinary) {
		v.Transaction = x
	}
	if x := new(protocol.TransactionStatus); reader.ReadValue(3, x.UnmarshalBinary) {
		v.Status = x
	}
	for {
		if x := new(TxnSigSet); reader.ReadValue(4, x.UnmarshalBinary) {
			v.SignatureSets = append(v.SignatureSets, x)
		} else {
			break
		}
	}

	seen, err := reader.Reset(fieldNames_Transaction)
	if err != nil {
		return encoding.Error{E: err}
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	if err != nil {
		return encoding.Error{E: err}
	}
	return nil
}

func (v *TxnSigSet) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *TxnSigSet) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadUrl(1); ok {
		v.Signer = x
	}
	if x, ok := reader.ReadUint(2); ok {
		v.Version = x
	}
	for {
		if x := new(database.SigSetEntry); reader.ReadValue(3, x.UnmarshalBinary) {
			v.Entries = append(v.Entries, *x)
		} else {
			break
		}
	}

	seen, err := reader.Reset(fieldNames_TxnSigSet)
	if err != nil {
		return encoding.Error{E: err}
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	if err != nil {
		return encoding.Error{E: err}
	}
	return nil
}

func (v *sigSection) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *sigSection) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	for {
		if x := new(Signature); reader.ReadValue(1, x.UnmarshalBinary) {
			v.Signatures = append(v.Signatures, x)
		} else {
			break
		}
	}

	seen, err := reader.Reset(fieldNames_sigSection)
	if err != nil {
		return encoding.Error{E: err}
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	if err != nil {
		return encoding.Error{E: err}
	}
	return nil
}

func (v *txnSection) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *txnSection) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	for {
		if x := new(Transaction); reader.ReadValue(1, x.UnmarshalBinary) {
			v.Transactions = append(v.Transactions, x)
		} else {
			break
		}
	}

	seen, err := reader.Reset(fieldNames_txnSection)
	if err != nil {
		return encoding.Error{E: err}
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	if err != nil {
		return encoding.Error{E: err}
	}
	return nil
}

func (v *Account) MarshalJSON() ([]byte, error) {
	u := struct {
		Url       *url.URL                                     `json:"url,omitempty"`
		Main      encoding.JsonUnmarshalWith[protocol.Account] `json:"main,omitempty"`
		Chains    encoding.JsonList[*ChainState]               `json:"chains,omitempty"`
		Pending   encoding.JsonList[*url.TxID]                 `json:"pending,omitempty"`
		Directory encoding.JsonList[*url.URL]                  `json:"directory,omitempty"`
	}{}
	u.Url = v.Url
	u.Main = encoding.JsonUnmarshalWith[protocol.Account]{Value: v.Main, Func: protocol.UnmarshalAccountJSON}
	u.Chains = v.Chains
	u.Pending = v.Pending
	u.Directory = v.Directory
	return json.Marshal(&u)
}

func (v *Chain) MarshalJSON() ([]byte, error) {
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

func (v *Header) MarshalJSON() ([]byte, error) {
	u := struct {
		Version  uint64 `json:"version,omitempty"`
		Height   uint64 `json:"height,omitempty"`
		RootHash string `json:"rootHash,omitempty"`
	}{}
	u.Version = v.Version
	u.Height = v.Height
	u.RootHash = encoding.ChainToJSON(v.RootHash)
	return json.Marshal(&u)
}

func (v *Signature) MarshalJSON() ([]byte, error) {
	u := struct {
		Txid      *url.TxID                                      `json:"txid,omitempty"`
		Signature encoding.JsonUnmarshalWith[protocol.Signature] `json:"signature,omitempty"`
	}{}
	u.Txid = v.Txid
	u.Signature = encoding.JsonUnmarshalWith[protocol.Signature]{Value: v.Signature, Func: protocol.UnmarshalSignatureJSON}
	return json.Marshal(&u)
}

func (v *Transaction) MarshalJSON() ([]byte, error) {
	u := struct {
		Transaction   *protocol.Transaction         `json:"transaction,omitempty"`
		Status        *protocol.TransactionStatus   `json:"status,omitempty"`
		SignatureSets encoding.JsonList[*TxnSigSet] `json:"signatureSets,omitempty"`
	}{}
	u.Transaction = v.Transaction
	u.Status = v.Status
	u.SignatureSets = v.SignatureSets
	return json.Marshal(&u)
}

func (v *TxnSigSet) MarshalJSON() ([]byte, error) {
	u := struct {
		Signer  *url.URL                                `json:"signer,omitempty"`
		Version uint64                                  `json:"version,omitempty"`
		Entries encoding.JsonList[database.SigSetEntry] `json:"entries,omitempty"`
	}{}
	u.Signer = v.Signer
	u.Version = v.Version
	u.Entries = v.Entries
	return json.Marshal(&u)
}

func (v *sigSection) MarshalJSON() ([]byte, error) {
	u := struct {
		Signatures encoding.JsonList[*Signature] `json:"signatures,omitempty"`
	}{}
	u.Signatures = v.Signatures
	return json.Marshal(&u)
}

func (v *txnSection) MarshalJSON() ([]byte, error) {
	u := struct {
		Transactions encoding.JsonList[*Transaction] `json:"transactions,omitempty"`
	}{}
	u.Transactions = v.Transactions
	return json.Marshal(&u)
}

func (v *Account) UnmarshalJSON(data []byte) error {
	u := struct {
		Url       *url.URL                                     `json:"url,omitempty"`
		Main      encoding.JsonUnmarshalWith[protocol.Account] `json:"main,omitempty"`
		Chains    encoding.JsonList[*ChainState]               `json:"chains,omitempty"`
		Pending   encoding.JsonList[*url.TxID]                 `json:"pending,omitempty"`
		Directory encoding.JsonList[*url.URL]                  `json:"directory,omitempty"`
	}{}
	u.Url = v.Url
	u.Main = encoding.JsonUnmarshalWith[protocol.Account]{Value: v.Main, Func: protocol.UnmarshalAccountJSON}
	u.Chains = v.Chains
	u.Pending = v.Pending
	u.Directory = v.Directory
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Url = u.Url
	v.Main = u.Main.Value

	v.Chains = u.Chains
	v.Pending = u.Pending
	v.Directory = u.Directory
	return nil
}

func (v *Chain) UnmarshalJSON(data []byte) error {
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

func (v *Header) UnmarshalJSON(data []byte) error {
	u := struct {
		Version  uint64 `json:"version,omitempty"`
		Height   uint64 `json:"height,omitempty"`
		RootHash string `json:"rootHash,omitempty"`
	}{}
	u.Version = v.Version
	u.Height = v.Height
	u.RootHash = encoding.ChainToJSON(v.RootHash)
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Version = u.Version
	v.Height = u.Height
	if x, err := encoding.ChainFromJSON(u.RootHash); err != nil {
		return fmt.Errorf("error decoding RootHash: %w", err)
	} else {
		v.RootHash = x
	}
	return nil
}

func (v *Signature) UnmarshalJSON(data []byte) error {
	u := struct {
		Txid      *url.TxID                                      `json:"txid,omitempty"`
		Signature encoding.JsonUnmarshalWith[protocol.Signature] `json:"signature,omitempty"`
	}{}
	u.Txid = v.Txid
	u.Signature = encoding.JsonUnmarshalWith[protocol.Signature]{Value: v.Signature, Func: protocol.UnmarshalSignatureJSON}
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Txid = u.Txid
	v.Signature = u.Signature.Value

	return nil
}

func (v *Transaction) UnmarshalJSON(data []byte) error {
	u := struct {
		Transaction   *protocol.Transaction         `json:"transaction,omitempty"`
		Status        *protocol.TransactionStatus   `json:"status,omitempty"`
		SignatureSets encoding.JsonList[*TxnSigSet] `json:"signatureSets,omitempty"`
	}{}
	u.Transaction = v.Transaction
	u.Status = v.Status
	u.SignatureSets = v.SignatureSets
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Transaction = u.Transaction
	v.Status = u.Status
	v.SignatureSets = u.SignatureSets
	return nil
}

func (v *TxnSigSet) UnmarshalJSON(data []byte) error {
	u := struct {
		Signer  *url.URL                                `json:"signer,omitempty"`
		Version uint64                                  `json:"version,omitempty"`
		Entries encoding.JsonList[database.SigSetEntry] `json:"entries,omitempty"`
	}{}
	u.Signer = v.Signer
	u.Version = v.Version
	u.Entries = v.Entries
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Signer = u.Signer
	v.Version = u.Version
	v.Entries = u.Entries
	return nil
}

func (v *sigSection) UnmarshalJSON(data []byte) error {
	u := struct {
		Signatures encoding.JsonList[*Signature] `json:"signatures,omitempty"`
	}{}
	u.Signatures = v.Signatures
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Signatures = u.Signatures
	return nil
}

func (v *txnSection) UnmarshalJSON(data []byte) error {
	u := struct {
		Transactions encoding.JsonList[*Transaction] `json:"transactions,omitempty"`
	}{}
	u.Transactions = v.Transactions
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Transactions = u.Transactions
	return nil
}
