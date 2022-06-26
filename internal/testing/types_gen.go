package testing

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

type FakeAccount struct {
	fieldsSet []bool
	FakeLiteAccount
	protocol.AccountAuth
	extraData []byte
}

type FakeAuthority struct {
	fieldsSet []bool
	FakeAccount
	Signers   *url.URL `json:"signers,omitempty" form:"signers" query:"signers" validate:"required"`
	extraData []byte
}

type FakeLiteAccount struct {
	fieldsSet []bool
	TheType   protocol.AccountType `json:"theType,omitempty" form:"theType" query:"theType" validate:"required"`
	Url       *url.URL             `json:"url,omitempty" form:"url" query:"url" validate:"required"`
	extraData []byte
}

type FakeSignature struct {
	fieldsSet     []bool
	TheType       protocol.SignatureType `json:"theType,omitempty" form:"theType" query:"theType" validate:"required"`
	Vote          protocol.VoteType      `json:"vote,omitempty" form:"vote" query:"vote" validate:"required"`
	PublicKey     []byte                 `json:"publicKey,omitempty" form:"publicKey" query:"publicKey" validate:"required"`
	Signer        *url.URL               `json:"signer,omitempty" form:"signer" query:"signer" validate:"required"`
	SignerVersion uint64                 `json:"signerVersion,omitempty" form:"signerVersion" query:"signerVersion" validate:"required"`
	Timestamp     uint64                 `json:"timestamp,omitempty" form:"timestamp" query:"timestamp"`
	extraData     []byte
}

type FakeSigner struct {
	fieldsSet []bool
	FakeLiteAccount
	CreditBalance uint64              `json:"creditBalance,omitempty" form:"creditBalance" query:"creditBalance" validate:"required"`
	Threshold     uint64              `json:"threshold,omitempty" form:"threshold" query:"threshold" validate:"required"`
	Version       uint64              `json:"version,omitempty" form:"version" query:"version" validate:"required"`
	Keys          []*protocol.KeySpec `json:"keys,omitempty" form:"keys" query:"keys" validate:"required"`
	extraData     []byte
}

type FakeTransactionBody struct {
	fieldsSet []bool
	TheType   protocol.TransactionType `json:"theType,omitempty" form:"theType" query:"theType" validate:"required"`
	extraData []byte
}

func (v *FakeAccount) Copy() *FakeAccount {
	u := new(FakeAccount)

	u.FakeLiteAccount = *v.FakeLiteAccount.Copy()
	u.AccountAuth = *v.AccountAuth.Copy()

	return u
}

func (v *FakeAccount) CopyAsInterface() interface{} { return v.Copy() }

func (v *FakeAuthority) Copy() *FakeAuthority {
	u := new(FakeAuthority)

	u.FakeAccount = *v.FakeAccount.Copy()
	if v.Signers != nil {
		u.Signers = (v.Signers).Copy()
	}

	return u
}

func (v *FakeAuthority) CopyAsInterface() interface{} { return v.Copy() }

func (v *FakeLiteAccount) Copy() *FakeLiteAccount {
	u := new(FakeLiteAccount)

	u.TheType = v.TheType
	if v.Url != nil {
		u.Url = (v.Url).Copy()
	}

	return u
}

func (v *FakeLiteAccount) CopyAsInterface() interface{} { return v.Copy() }

func (v *FakeSignature) Copy() *FakeSignature {
	u := new(FakeSignature)

	u.TheType = v.TheType
	u.Vote = v.Vote
	u.PublicKey = encoding.BytesCopy(v.PublicKey)
	if v.Signer != nil {
		u.Signer = (v.Signer).Copy()
	}
	u.SignerVersion = v.SignerVersion
	u.Timestamp = v.Timestamp

	return u
}

func (v *FakeSignature) CopyAsInterface() interface{} { return v.Copy() }

func (v *FakeSigner) Copy() *FakeSigner {
	u := new(FakeSigner)

	u.FakeLiteAccount = *v.FakeLiteAccount.Copy()
	u.CreditBalance = v.CreditBalance
	u.Threshold = v.Threshold
	u.Version = v.Version
	u.Keys = make([]*protocol.KeySpec, len(v.Keys))
	for i, v := range v.Keys {
		if v != nil {
			u.Keys[i] = (v).Copy()
		}
	}

	return u
}

func (v *FakeSigner) CopyAsInterface() interface{} { return v.Copy() }

func (v *FakeTransactionBody) Copy() *FakeTransactionBody {
	u := new(FakeTransactionBody)

	u.TheType = v.TheType

	return u
}

func (v *FakeTransactionBody) CopyAsInterface() interface{} { return v.Copy() }

func (v *FakeAccount) Equal(u *FakeAccount) bool {
	if !v.FakeLiteAccount.Equal(&u.FakeLiteAccount) {
		return false
	}
	if !v.AccountAuth.Equal(&u.AccountAuth) {
		return false
	}

	return true
}

func (v *FakeAuthority) Equal(u *FakeAuthority) bool {
	if !v.FakeAccount.Equal(&u.FakeAccount) {
		return false
	}
	switch {
	case v.Signers == u.Signers:
		// equal
	case v.Signers == nil || u.Signers == nil:
		return false
	case !((v.Signers).Equal(u.Signers)):
		return false
	}

	return true
}

func (v *FakeLiteAccount) Equal(u *FakeLiteAccount) bool {
	if !(v.TheType == u.TheType) {
		return false
	}
	switch {
	case v.Url == u.Url:
		// equal
	case v.Url == nil || u.Url == nil:
		return false
	case !((v.Url).Equal(u.Url)):
		return false
	}

	return true
}

func (v *FakeSignature) Equal(u *FakeSignature) bool {
	if !(v.TheType == u.TheType) {
		return false
	}
	if !(v.Vote == u.Vote) {
		return false
	}
	if !(bytes.Equal(v.PublicKey, u.PublicKey)) {
		return false
	}
	switch {
	case v.Signer == u.Signer:
		// equal
	case v.Signer == nil || u.Signer == nil:
		return false
	case !((v.Signer).Equal(u.Signer)):
		return false
	}
	if !(v.SignerVersion == u.SignerVersion) {
		return false
	}
	if !(v.Timestamp == u.Timestamp) {
		return false
	}

	return true
}

func (v *FakeSigner) Equal(u *FakeSigner) bool {
	if !v.FakeLiteAccount.Equal(&u.FakeLiteAccount) {
		return false
	}
	if !(v.CreditBalance == u.CreditBalance) {
		return false
	}
	if !(v.Threshold == u.Threshold) {
		return false
	}
	if !(v.Version == u.Version) {
		return false
	}
	if len(v.Keys) != len(u.Keys) {
		return false
	}
	for i := range v.Keys {
		if !((v.Keys[i]).Equal(u.Keys[i])) {
			return false
		}
	}

	return true
}

func (v *FakeTransactionBody) Equal(u *FakeTransactionBody) bool {
	if !(v.TheType == u.TheType) {
		return false
	}

	return true
}

var fieldNames_FakeAccount = []string{
	1: "FakeLiteAccount",
	2: "AccountAuth",
}

func (v *FakeAccount) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	writer.WriteValue(1, v.FakeLiteAccount.MarshalBinary)
	writer.WriteValue(2, v.AccountAuth.MarshalBinary)

	_, _, err := writer.Reset(fieldNames_FakeAccount)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *FakeAccount) IsValid() error {
	var errs []string

	if err := v.FakeLiteAccount.IsValid(); err != nil {
		errs = append(errs, err.Error())
	}
	if err := v.AccountAuth.IsValid(); err != nil {
		errs = append(errs, err.Error())
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

var fieldNames_FakeAuthority = []string{
	1: "FakeAccount",
	2: "Signers",
}

func (v *FakeAuthority) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	writer.WriteValue(1, v.FakeAccount.MarshalBinary)
	if !(v.Signers == nil) {
		writer.WriteUrl(2, v.Signers)
	}

	_, _, err := writer.Reset(fieldNames_FakeAuthority)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *FakeAuthority) IsValid() error {
	var errs []string

	if err := v.FakeAccount.IsValid(); err != nil {
		errs = append(errs, err.Error())
	}
	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field Signers is missing")
	} else if v.Signers == nil {
		errs = append(errs, "field Signers is not set")
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

var fieldNames_FakeLiteAccount = []string{
	1: "TheType",
	2: "Url",
}

func (v *FakeLiteAccount) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(v.TheType == 0) {
		writer.WriteEnum(1, v.TheType)
	}
	if !(v.Url == nil) {
		writer.WriteUrl(2, v.Url)
	}

	_, _, err := writer.Reset(fieldNames_FakeLiteAccount)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *FakeLiteAccount) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field TheType is missing")
	} else if v.TheType == 0 {
		errs = append(errs, "field TheType is not set")
	}
	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field Url is missing")
	} else if v.Url == nil {
		errs = append(errs, "field Url is not set")
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

var fieldNames_FakeSignature = []string{
	1: "TheType",
	2: "Vote",
	3: "PublicKey",
	4: "Signer",
	5: "SignerVersion",
	6: "Timestamp",
}

func (v *FakeSignature) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(v.TheType == 0) {
		writer.WriteEnum(1, v.TheType)
	}
	if !(v.Vote == 0) {
		writer.WriteEnum(2, v.Vote)
	}
	if !(len(v.PublicKey) == 0) {
		writer.WriteBytes(3, v.PublicKey)
	}
	if !(v.Signer == nil) {
		writer.WriteUrl(4, v.Signer)
	}
	if !(v.SignerVersion == 0) {
		writer.WriteUint(5, v.SignerVersion)
	}
	if !(v.Timestamp == 0) {
		writer.WriteUint(6, v.Timestamp)
	}

	_, _, err := writer.Reset(fieldNames_FakeSignature)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *FakeSignature) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field TheType is missing")
	} else if v.TheType == 0 {
		errs = append(errs, "field TheType is not set")
	}
	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field Vote is missing")
	} else if v.Vote == 0 {
		errs = append(errs, "field Vote is not set")
	}
	if len(v.fieldsSet) > 3 && !v.fieldsSet[3] {
		errs = append(errs, "field PublicKey is missing")
	} else if len(v.PublicKey) == 0 {
		errs = append(errs, "field PublicKey is not set")
	}
	if len(v.fieldsSet) > 4 && !v.fieldsSet[4] {
		errs = append(errs, "field Signer is missing")
	} else if v.Signer == nil {
		errs = append(errs, "field Signer is not set")
	}
	if len(v.fieldsSet) > 5 && !v.fieldsSet[5] {
		errs = append(errs, "field SignerVersion is missing")
	} else if v.SignerVersion == 0 {
		errs = append(errs, "field SignerVersion is not set")
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

var fieldNames_FakeSigner = []string{
	1: "FakeLiteAccount",
	2: "CreditBalance",
	3: "Threshold",
	4: "Version",
	5: "Keys",
}

func (v *FakeSigner) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	writer.WriteValue(1, v.FakeLiteAccount.MarshalBinary)
	if !(v.CreditBalance == 0) {
		writer.WriteUint(2, v.CreditBalance)
	}
	if !(v.Threshold == 0) {
		writer.WriteUint(3, v.Threshold)
	}
	if !(v.Version == 0) {
		writer.WriteUint(4, v.Version)
	}
	if !(len(v.Keys) == 0) {
		for _, v := range v.Keys {
			writer.WriteValue(5, v.MarshalBinary)
		}
	}

	_, _, err := writer.Reset(fieldNames_FakeSigner)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *FakeSigner) IsValid() error {
	var errs []string

	if err := v.FakeLiteAccount.IsValid(); err != nil {
		errs = append(errs, err.Error())
	}
	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field CreditBalance is missing")
	} else if v.CreditBalance == 0 {
		errs = append(errs, "field CreditBalance is not set")
	}
	if len(v.fieldsSet) > 3 && !v.fieldsSet[3] {
		errs = append(errs, "field Threshold is missing")
	} else if v.Threshold == 0 {
		errs = append(errs, "field Threshold is not set")
	}
	if len(v.fieldsSet) > 4 && !v.fieldsSet[4] {
		errs = append(errs, "field Version is missing")
	} else if v.Version == 0 {
		errs = append(errs, "field Version is not set")
	}
	if len(v.fieldsSet) > 5 && !v.fieldsSet[5] {
		errs = append(errs, "field Keys is missing")
	} else if len(v.Keys) == 0 {
		errs = append(errs, "field Keys is not set")
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

var fieldNames_FakeTransactionBody = []string{
	1: "TheType",
}

func (v *FakeTransactionBody) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(v.TheType == 0) {
		writer.WriteEnum(1, v.TheType)
	}

	_, _, err := writer.Reset(fieldNames_FakeTransactionBody)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *FakeTransactionBody) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field TheType is missing")
	} else if v.TheType == 0 {
		errs = append(errs, "field TheType is not set")
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

func (v *FakeAccount) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *FakeAccount) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	reader.ReadValue(1, v.FakeLiteAccount.UnmarshalBinary)
	reader.ReadValue(2, v.AccountAuth.UnmarshalBinary)

	seen, err := reader.Reset(fieldNames_FakeAccount)
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

func (v *FakeAuthority) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *FakeAuthority) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	reader.ReadValue(1, v.FakeAccount.UnmarshalBinary)
	if x, ok := reader.ReadUrl(2); ok {
		v.Signers = x
	}

	seen, err := reader.Reset(fieldNames_FakeAuthority)
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

func (v *FakeLiteAccount) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *FakeLiteAccount) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x := new(protocol.AccountType); reader.ReadEnum(1, x) {
		v.TheType = *x
	}
	if x, ok := reader.ReadUrl(2); ok {
		v.Url = x
	}

	seen, err := reader.Reset(fieldNames_FakeLiteAccount)
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

func (v *FakeSignature) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *FakeSignature) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x := new(protocol.SignatureType); reader.ReadEnum(1, x) {
		v.TheType = *x
	}
	if x := new(protocol.VoteType); reader.ReadEnum(2, x) {
		v.Vote = *x
	}
	if x, ok := reader.ReadBytes(3); ok {
		v.PublicKey = x
	}
	if x, ok := reader.ReadUrl(4); ok {
		v.Signer = x
	}
	if x, ok := reader.ReadUint(5); ok {
		v.SignerVersion = x
	}
	if x, ok := reader.ReadUint(6); ok {
		v.Timestamp = x
	}

	seen, err := reader.Reset(fieldNames_FakeSignature)
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

func (v *FakeSigner) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *FakeSigner) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	reader.ReadValue(1, v.FakeLiteAccount.UnmarshalBinary)
	if x, ok := reader.ReadUint(2); ok {
		v.CreditBalance = x
	}
	if x, ok := reader.ReadUint(3); ok {
		v.Threshold = x
	}
	if x, ok := reader.ReadUint(4); ok {
		v.Version = x
	}
	for {
		if x := new(protocol.KeySpec); reader.ReadValue(5, x.UnmarshalBinary) {
			v.Keys = append(v.Keys, x)
		} else {
			break
		}
	}

	seen, err := reader.Reset(fieldNames_FakeSigner)
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

func (v *FakeTransactionBody) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *FakeTransactionBody) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x := new(protocol.TransactionType); reader.ReadEnum(1, x) {
		v.TheType = *x
	}

	seen, err := reader.Reset(fieldNames_FakeTransactionBody)
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

func (v *FakeSignature) MarshalJSON() ([]byte, error) {
	u := struct {
		TheType       protocol.SignatureType `json:"theType,omitempty"`
		Vote          protocol.VoteType      `json:"vote,omitempty"`
		PublicKey     *string                `json:"publicKey,omitempty"`
		Signer        *url.URL               `json:"signer,omitempty"`
		SignerVersion uint64                 `json:"signerVersion,omitempty"`
		Timestamp     uint64                 `json:"timestamp,omitempty"`
	}{}
	u.TheType = v.TheType
	u.Vote = v.Vote
	u.PublicKey = encoding.BytesToJSON(v.PublicKey)
	u.Signer = v.Signer
	u.SignerVersion = v.SignerVersion
	u.Timestamp = v.Timestamp
	return json.Marshal(&u)
}

func (v *FakeSigner) MarshalJSON() ([]byte, error) {
	u := struct {
		TheType       protocol.AccountType                 `json:"theType,omitempty"`
		Url           *url.URL                             `json:"url,omitempty"`
		CreditBalance uint64                               `json:"creditBalance,omitempty"`
		Threshold     uint64                               `json:"threshold,omitempty"`
		Version       uint64                               `json:"version,omitempty"`
		Keys          encoding.JsonList[*protocol.KeySpec] `json:"keys,omitempty"`
	}{}
	u.TheType = v.FakeLiteAccount.TheType
	u.Url = v.FakeLiteAccount.Url
	u.CreditBalance = v.CreditBalance
	u.Threshold = v.Threshold
	u.Version = v.Version
	u.Keys = v.Keys
	return json.Marshal(&u)
}

func (v *FakeSignature) UnmarshalJSON(data []byte) error {
	u := struct {
		TheType       protocol.SignatureType `json:"theType,omitempty"`
		Vote          protocol.VoteType      `json:"vote,omitempty"`
		PublicKey     *string                `json:"publicKey,omitempty"`
		Signer        *url.URL               `json:"signer,omitempty"`
		SignerVersion uint64                 `json:"signerVersion,omitempty"`
		Timestamp     uint64                 `json:"timestamp,omitempty"`
	}{}
	u.TheType = v.TheType
	u.Vote = v.Vote
	u.PublicKey = encoding.BytesToJSON(v.PublicKey)
	u.Signer = v.Signer
	u.SignerVersion = v.SignerVersion
	u.Timestamp = v.Timestamp
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.TheType = u.TheType
	v.Vote = u.Vote
	if x, err := encoding.BytesFromJSON(u.PublicKey); err != nil {
		return fmt.Errorf("error decoding PublicKey: %w", err)
	} else {
		v.PublicKey = x
	}
	v.Signer = u.Signer
	v.SignerVersion = u.SignerVersion
	v.Timestamp = u.Timestamp
	return nil
}

func (v *FakeSigner) UnmarshalJSON(data []byte) error {
	u := struct {
		TheType       protocol.AccountType                 `json:"theType,omitempty"`
		Url           *url.URL                             `json:"url,omitempty"`
		CreditBalance uint64                               `json:"creditBalance,omitempty"`
		Threshold     uint64                               `json:"threshold,omitempty"`
		Version       uint64                               `json:"version,omitempty"`
		Keys          encoding.JsonList[*protocol.KeySpec] `json:"keys,omitempty"`
	}{}
	u.TheType = v.FakeLiteAccount.TheType
	u.Url = v.FakeLiteAccount.Url
	u.CreditBalance = v.CreditBalance
	u.Threshold = v.Threshold
	u.Version = v.Version
	u.Keys = v.Keys
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.FakeLiteAccount.TheType = u.TheType
	v.FakeLiteAccount.Url = u.Url
	v.CreditBalance = u.CreditBalance
	v.Threshold = u.Threshold
	v.Version = u.Version
	v.Keys = u.Keys
	return nil
}
