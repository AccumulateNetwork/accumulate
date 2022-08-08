package api

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

//lint:file-ignore S1001,S1002,S1008,SA4013 generated code

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type AddTokenTransactionOutput struct {
	Name   string `json:"name,omitempty" form:"name" query:"name" validate:"required"`
	Url    string `json:"url,omitempty" form:"url" query:"url" validate:"required"`
	Amount string `json:"amount,omitempty" form:"amount" query:"amount" validate:"required"`
}

type AddTransactionBodyRequest struct {
	Name string `json:"name,omitempty" form:"name" query:"name" validate:"required"`
	Body string `json:"body,omitempty" form:"body" query:"body" validate:"required"`
}

type AddTransactionInputRequest struct {
	Name string `json:"name,omitempty" form:"name" query:"name" validate:"required"`
	Url  string `json:"url,omitempty" form:"url" query:"url" validate:"required"`
}

type AddTransactionToEnvelopeRequest struct {
	EnvelopeName    string `json:"envelopeName,omitempty" form:"envelopeName" query:"envelopeName" validate:"required"`
	TransactionName string `json:"transactionName,omitempty" form:"transactionName" query:"transactionName" validate:"required"`
}

type AdiListResponse struct {
	Urls []string `json:"urls,omitempty" form:"urls" query:"urls" validate:"required"`
}

type AuthorizationRequired struct {
	fieldsSet []bool
	Key       []byte `json:"key,omitempty" form:"key" query:"key" validate:"required"`
	Version   string `json:"version,omitempty" form:"version" query:"version" validate:"required"`
	extraData []byte
}

type CreateEnvelopeRequest struct {
	Name string `json:"name,omitempty" form:"name" query:"name" validate:"required"`
}

type CreateTransactionRequest struct {
	Name            string                   `json:"name,omitempty" form:"name" query:"name" validate:"required"`
	TransactionType protocol.TransactionType `json:"transactionType,omitempty" form:"transactionType" query:"transactionType" validate:"required"`
}

type DecodeRequest struct {
	DataBinary []byte `json:"dataBinary,omitempty" form:"dataBinary" query:"dataBinary" validate:"required"`
}

type DecodeResponse struct {
	DataJson string `json:"dataJson,omitempty" form:"dataJson" query:"dataJson" validate:"required"`
}

type DeleteTransactionRequest struct {
	Name string `json:"name,omitempty" form:"name" query:"name" validate:"required"`
}

type DeleteTransactionResponse struct {
	Name string `json:"name,omitempty" form:"name" query:"name" validate:"required"`
}

type EncodeAccountResponse struct {
	AccountBinary []byte `json:"accountBinary,omitempty" form:"accountBinary" query:"accountBinary" validate:"required"`
}

type EncodeRequest struct {
	DataJson string `json:"dataJson,omitempty" form:"dataJson" query:"dataJson" validate:"required"`
}

type EncodeTransactionBodyResponse struct {
	TransactionBodyBinary []byte `json:"transactionBodyBinary,omitempty" form:"transactionBodyBinary" query:"transactionBodyBinary" validate:"required"`
}

type EncodeTransactionHeaderResponse struct {
	TransactionHeaderBinary []byte `json:"transactionHeaderBinary,omitempty" form:"transactionHeaderBinary" query:"transactionHeaderBinary" validate:"required"`
}

type EncodeTransactionResponse struct {
	TransactionBinary []byte `json:"transactionBinary,omitempty" form:"transactionBinary" query:"transactionBinary" validate:"required"`
	TransactionHash   []byte `json:"transactionHash,omitempty" form:"transactionHash" query:"transactionHash" validate:"required"`
}

type FinalizeEnvelopeRequest struct {
	Name string `json:"name,omitempty" form:"name" query:"name" validate:"required"`
}

type GeneralResponse struct {
	Info  string    `json:"info,omitempty" form:"info" query:"info" validate:"required"`
	Code  ErrorCode `json:"code,omitempty" form:"code" query:"code" validate:"required"`
	Error string    `json:"error,omitempty" form:"error" query:"error" validate:"required"`
}

type KeyData struct {
	Name       string                 `json:"name,omitempty" form:"name" query:"name" validate:"required"`
	PublicKey  []byte                 `json:"publicKey,omitempty" form:"publicKey" query:"publicKey" validate:"required"`
	Derivation string                 `json:"derivation,omitempty" form:"derivation" query:"derivation" validate:"required"`
	KeyType    protocol.SignatureType `json:"keyType,omitempty" form:"keyType" query:"keyType" validate:"required"`
}

type KeyListResponse struct {
	KeyList []KeyData `json:"keyList,omitempty" form:"keyList" query:"keyList" validate:"required"`
}

type ProveReceiptRequest struct {
	DataJson    string `json:"dataJson,omitempty" form:"dataJson" query:"dataJson" validate:"required"`
	ReceiptJson string `json:"receiptJson,omitempty" form:"receiptJson" query:"receiptJson" validate:"required"`
}

type ResolveKeyRequest struct {
	KeyNameOrLiteAddress string `json:"keyNameOrLiteAddress,omitempty" form:"keyNameOrLiteAddress" query:"keyNameOrLiteAddress" validate:"required"`
}

type ResolveKeyResponse struct {
	KeyData KeyData `json:"keyData,omitempty" form:"keyData" query:"keyData" validate:"required"`
}

type SignRequest struct {
	Name    int64  `json:"name,omitempty" form:"name" query:"name" validate:"required"`
	KeyName string `json:"keyName,omitempty" form:"keyName" query:"keyName"`
}

type SignResponse struct {
	Signature []byte `json:"signature,omitempty" form:"signature" query:"signature" validate:"required"`
	PublicKey []byte `json:"publicKey,omitempty" form:"publicKey" query:"publicKey" validate:"required"`
}

type VersionResponse struct {
	fieldsSet []bool
	Version   string `json:"version,omitempty" form:"version" query:"version" validate:"required"`
	Commit    string `json:"commit,omitempty" form:"commit" query:"commit" validate:"required"`
	extraData []byte
}

func (v *AddTokenTransactionOutput) Copy() *AddTokenTransactionOutput {
	u := new(AddTokenTransactionOutput)

	u.Name = v.Name
	u.Url = v.Url
	u.Amount = v.Amount

	return u
}

func (v *AddTokenTransactionOutput) CopyAsInterface() interface{} { return v.Copy() }

func (v *AddTransactionBodyRequest) Copy() *AddTransactionBodyRequest {
	u := new(AddTransactionBodyRequest)

	u.Name = v.Name
	u.Body = v.Body

	return u
}

func (v *AddTransactionBodyRequest) CopyAsInterface() interface{} { return v.Copy() }

func (v *AddTransactionInputRequest) Copy() *AddTransactionInputRequest {
	u := new(AddTransactionInputRequest)

	u.Name = v.Name
	u.Url = v.Url

	return u
}

func (v *AddTransactionInputRequest) CopyAsInterface() interface{} { return v.Copy() }

func (v *AddTransactionToEnvelopeRequest) Copy() *AddTransactionToEnvelopeRequest {
	u := new(AddTransactionToEnvelopeRequest)

	u.EnvelopeName = v.EnvelopeName
	u.TransactionName = v.TransactionName

	return u
}

func (v *AddTransactionToEnvelopeRequest) CopyAsInterface() interface{} { return v.Copy() }

func (v *AdiListResponse) Copy() *AdiListResponse {
	u := new(AdiListResponse)

	u.Urls = make([]string, len(v.Urls))
	for i, v := range v.Urls {
		u.Urls[i] = v
	}

	return u
}

func (v *AdiListResponse) CopyAsInterface() interface{} { return v.Copy() }

func (v *AuthorizationRequired) Copy() *AuthorizationRequired {
	u := new(AuthorizationRequired)

	u.Key = encoding.BytesCopy(v.Key)
	u.Version = v.Version

	return u
}

func (v *AuthorizationRequired) CopyAsInterface() interface{} { return v.Copy() }

func (v *CreateEnvelopeRequest) Copy() *CreateEnvelopeRequest {
	u := new(CreateEnvelopeRequest)

	u.Name = v.Name

	return u
}

func (v *CreateEnvelopeRequest) CopyAsInterface() interface{} { return v.Copy() }

func (v *CreateTransactionRequest) Copy() *CreateTransactionRequest {
	u := new(CreateTransactionRequest)

	u.Name = v.Name
	u.TransactionType = v.TransactionType

	return u
}

func (v *CreateTransactionRequest) CopyAsInterface() interface{} { return v.Copy() }

func (v *DecodeRequest) Copy() *DecodeRequest {
	u := new(DecodeRequest)

	u.DataBinary = encoding.BytesCopy(v.DataBinary)

	return u
}

func (v *DecodeRequest) CopyAsInterface() interface{} { return v.Copy() }

func (v *DecodeResponse) Copy() *DecodeResponse {
	u := new(DecodeResponse)

	u.DataJson = v.DataJson

	return u
}

func (v *DecodeResponse) CopyAsInterface() interface{} { return v.Copy() }

func (v *DeleteTransactionRequest) Copy() *DeleteTransactionRequest {
	u := new(DeleteTransactionRequest)

	u.Name = v.Name

	return u
}

func (v *DeleteTransactionRequest) CopyAsInterface() interface{} { return v.Copy() }

func (v *DeleteTransactionResponse) Copy() *DeleteTransactionResponse {
	u := new(DeleteTransactionResponse)

	u.Name = v.Name

	return u
}

func (v *DeleteTransactionResponse) CopyAsInterface() interface{} { return v.Copy() }

func (v *EncodeAccountResponse) Copy() *EncodeAccountResponse {
	u := new(EncodeAccountResponse)

	u.AccountBinary = encoding.BytesCopy(v.AccountBinary)

	return u
}

func (v *EncodeAccountResponse) CopyAsInterface() interface{} { return v.Copy() }

func (v *EncodeRequest) Copy() *EncodeRequest {
	u := new(EncodeRequest)

	u.DataJson = v.DataJson

	return u
}

func (v *EncodeRequest) CopyAsInterface() interface{} { return v.Copy() }

func (v *EncodeTransactionBodyResponse) Copy() *EncodeTransactionBodyResponse {
	u := new(EncodeTransactionBodyResponse)

	u.TransactionBodyBinary = encoding.BytesCopy(v.TransactionBodyBinary)

	return u
}

func (v *EncodeTransactionBodyResponse) CopyAsInterface() interface{} { return v.Copy() }

func (v *EncodeTransactionHeaderResponse) Copy() *EncodeTransactionHeaderResponse {
	u := new(EncodeTransactionHeaderResponse)

	u.TransactionHeaderBinary = encoding.BytesCopy(v.TransactionHeaderBinary)

	return u
}

func (v *EncodeTransactionHeaderResponse) CopyAsInterface() interface{} { return v.Copy() }

func (v *EncodeTransactionResponse) Copy() *EncodeTransactionResponse {
	u := new(EncodeTransactionResponse)

	u.TransactionBinary = encoding.BytesCopy(v.TransactionBinary)
	u.TransactionHash = encoding.BytesCopy(v.TransactionHash)

	return u
}

func (v *EncodeTransactionResponse) CopyAsInterface() interface{} { return v.Copy() }

func (v *FinalizeEnvelopeRequest) Copy() *FinalizeEnvelopeRequest {
	u := new(FinalizeEnvelopeRequest)

	u.Name = v.Name

	return u
}

func (v *FinalizeEnvelopeRequest) CopyAsInterface() interface{} { return v.Copy() }

func (v *GeneralResponse) Copy() *GeneralResponse {
	u := new(GeneralResponse)

	u.Info = v.Info
	u.Code = v.Code
	u.Error = v.Error

	return u
}

func (v *GeneralResponse) CopyAsInterface() interface{} { return v.Copy() }

func (v *KeyData) Copy() *KeyData {
	u := new(KeyData)

	u.Name = v.Name
	u.PublicKey = encoding.BytesCopy(v.PublicKey)
	u.Derivation = v.Derivation
	u.KeyType = v.KeyType

	return u
}

func (v *KeyData) CopyAsInterface() interface{} { return v.Copy() }

func (v *KeyListResponse) Copy() *KeyListResponse {
	u := new(KeyListResponse)

	u.KeyList = make([]KeyData, len(v.KeyList))
	for i, v := range v.KeyList {
		u.KeyList[i] = *(&v).Copy()
	}

	return u
}

func (v *KeyListResponse) CopyAsInterface() interface{} { return v.Copy() }

func (v *ProveReceiptRequest) Copy() *ProveReceiptRequest {
	u := new(ProveReceiptRequest)

	u.DataJson = v.DataJson
	u.ReceiptJson = v.ReceiptJson

	return u
}

func (v *ProveReceiptRequest) CopyAsInterface() interface{} { return v.Copy() }

func (v *ResolveKeyRequest) Copy() *ResolveKeyRequest {
	u := new(ResolveKeyRequest)

	u.KeyNameOrLiteAddress = v.KeyNameOrLiteAddress

	return u
}

func (v *ResolveKeyRequest) CopyAsInterface() interface{} { return v.Copy() }

func (v *ResolveKeyResponse) Copy() *ResolveKeyResponse {
	u := new(ResolveKeyResponse)

	u.KeyData = *(&v.KeyData).Copy()

	return u
}

func (v *ResolveKeyResponse) CopyAsInterface() interface{} { return v.Copy() }

func (v *SignRequest) Copy() *SignRequest {
	u := new(SignRequest)

	u.Name = v.Name
	u.KeyName = v.KeyName

	return u
}

func (v *SignRequest) CopyAsInterface() interface{} { return v.Copy() }

func (v *SignResponse) Copy() *SignResponse {
	u := new(SignResponse)

	u.Signature = encoding.BytesCopy(v.Signature)
	u.PublicKey = encoding.BytesCopy(v.PublicKey)

	return u
}

func (v *SignResponse) CopyAsInterface() interface{} { return v.Copy() }

func (v *VersionResponse) Copy() *VersionResponse {
	u := new(VersionResponse)

	u.Version = v.Version
	u.Commit = v.Commit

	return u
}

func (v *VersionResponse) CopyAsInterface() interface{} { return v.Copy() }

func (v *AddTokenTransactionOutput) Equal(u *AddTokenTransactionOutput) bool {
	if !(v.Name == u.Name) {
		return false
	}
	if !(v.Url == u.Url) {
		return false
	}
	if !(v.Amount == u.Amount) {
		return false
	}

	return true
}

func (v *AddTransactionBodyRequest) Equal(u *AddTransactionBodyRequest) bool {
	if !(v.Name == u.Name) {
		return false
	}
	if !(v.Body == u.Body) {
		return false
	}

	return true
}

func (v *AddTransactionInputRequest) Equal(u *AddTransactionInputRequest) bool {
	if !(v.Name == u.Name) {
		return false
	}
	if !(v.Url == u.Url) {
		return false
	}

	return true
}

func (v *AddTransactionToEnvelopeRequest) Equal(u *AddTransactionToEnvelopeRequest) bool {
	if !(v.EnvelopeName == u.EnvelopeName) {
		return false
	}
	if !(v.TransactionName == u.TransactionName) {
		return false
	}

	return true
}

func (v *AdiListResponse) Equal(u *AdiListResponse) bool {
	if len(v.Urls) != len(u.Urls) {
		return false
	}
	for i := range v.Urls {
		if !(v.Urls[i] == u.Urls[i]) {
			return false
		}
	}

	return true
}

func (v *AuthorizationRequired) Equal(u *AuthorizationRequired) bool {
	if !(bytes.Equal(v.Key, u.Key)) {
		return false
	}
	if !(v.Version == u.Version) {
		return false
	}

	return true
}

func (v *CreateEnvelopeRequest) Equal(u *CreateEnvelopeRequest) bool {
	if !(v.Name == u.Name) {
		return false
	}

	return true
}

func (v *CreateTransactionRequest) Equal(u *CreateTransactionRequest) bool {
	if !(v.Name == u.Name) {
		return false
	}
	if !(v.TransactionType == u.TransactionType) {
		return false
	}

	return true
}

func (v *DecodeRequest) Equal(u *DecodeRequest) bool {
	if !(bytes.Equal(v.DataBinary, u.DataBinary)) {
		return false
	}

	return true
}

func (v *DecodeResponse) Equal(u *DecodeResponse) bool {
	if !(v.DataJson == u.DataJson) {
		return false
	}

	return true
}

func (v *DeleteTransactionRequest) Equal(u *DeleteTransactionRequest) bool {
	if !(v.Name == u.Name) {
		return false
	}

	return true
}

func (v *DeleteTransactionResponse) Equal(u *DeleteTransactionResponse) bool {
	if !(v.Name == u.Name) {
		return false
	}

	return true
}

func (v *EncodeAccountResponse) Equal(u *EncodeAccountResponse) bool {
	if !(bytes.Equal(v.AccountBinary, u.AccountBinary)) {
		return false
	}

	return true
}

func (v *EncodeRequest) Equal(u *EncodeRequest) bool {
	if !(v.DataJson == u.DataJson) {
		return false
	}

	return true
}

func (v *EncodeTransactionBodyResponse) Equal(u *EncodeTransactionBodyResponse) bool {
	if !(bytes.Equal(v.TransactionBodyBinary, u.TransactionBodyBinary)) {
		return false
	}

	return true
}

func (v *EncodeTransactionHeaderResponse) Equal(u *EncodeTransactionHeaderResponse) bool {
	if !(bytes.Equal(v.TransactionHeaderBinary, u.TransactionHeaderBinary)) {
		return false
	}

	return true
}

func (v *EncodeTransactionResponse) Equal(u *EncodeTransactionResponse) bool {
	if !(bytes.Equal(v.TransactionBinary, u.TransactionBinary)) {
		return false
	}
	if !(bytes.Equal(v.TransactionHash, u.TransactionHash)) {
		return false
	}

	return true
}

func (v *FinalizeEnvelopeRequest) Equal(u *FinalizeEnvelopeRequest) bool {
	if !(v.Name == u.Name) {
		return false
	}

	return true
}

func (v *GeneralResponse) Equal(u *GeneralResponse) bool {
	if !(v.Info == u.Info) {
		return false
	}
	if !(v.Code == u.Code) {
		return false
	}
	if !(v.Error == u.Error) {
		return false
	}

	return true
}

func (v *KeyData) Equal(u *KeyData) bool {
	if !(v.Name == u.Name) {
		return false
	}
	if !(bytes.Equal(v.PublicKey, u.PublicKey)) {
		return false
	}
	if !(v.Derivation == u.Derivation) {
		return false
	}
	if !(v.KeyType == u.KeyType) {
		return false
	}

	return true
}

func (v *KeyListResponse) Equal(u *KeyListResponse) bool {
	if len(v.KeyList) != len(u.KeyList) {
		return false
	}
	for i := range v.KeyList {
		if !((&v.KeyList[i]).Equal(&u.KeyList[i])) {
			return false
		}
	}

	return true
}

func (v *ProveReceiptRequest) Equal(u *ProveReceiptRequest) bool {
	if !(v.DataJson == u.DataJson) {
		return false
	}
	if !(v.ReceiptJson == u.ReceiptJson) {
		return false
	}

	return true
}

func (v *ResolveKeyRequest) Equal(u *ResolveKeyRequest) bool {
	if !(v.KeyNameOrLiteAddress == u.KeyNameOrLiteAddress) {
		return false
	}

	return true
}

func (v *ResolveKeyResponse) Equal(u *ResolveKeyResponse) bool {
	if !((&v.KeyData).Equal(&u.KeyData)) {
		return false
	}

	return true
}

func (v *SignRequest) Equal(u *SignRequest) bool {
	if !(v.Name == u.Name) {
		return false
	}
	if !(v.KeyName == u.KeyName) {
		return false
	}

	return true
}

func (v *SignResponse) Equal(u *SignResponse) bool {
	if !(bytes.Equal(v.Signature, u.Signature)) {
		return false
	}
	if !(bytes.Equal(v.PublicKey, u.PublicKey)) {
		return false
	}

	return true
}

func (v *VersionResponse) Equal(u *VersionResponse) bool {
	if !(v.Version == u.Version) {
		return false
	}
	if !(v.Commit == u.Commit) {
		return false
	}

	return true
}

var fieldNames_AuthorizationRequired = []string{
	1: "Key",
	2: "Version",
}

func (v *AuthorizationRequired) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(len(v.Key) == 0) {
		writer.WriteBytes(1, v.Key)
	}
	if !(len(v.Version) == 0) {
		writer.WriteString(2, v.Version)
	}

	_, _, err := writer.Reset(fieldNames_AuthorizationRequired)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *AuthorizationRequired) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Key is missing")
	} else if len(v.Key) == 0 {
		errs = append(errs, "field Key is not set")
	}
	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field Version is missing")
	} else if len(v.Version) == 0 {
		errs = append(errs, "field Version is not set")
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

var fieldNames_VersionResponse = []string{
	1: "Version",
	2: "Commit",
}

func (v *VersionResponse) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(len(v.Version) == 0) {
		writer.WriteString(1, v.Version)
	}
	if !(len(v.Commit) == 0) {
		writer.WriteString(2, v.Commit)
	}

	_, _, err := writer.Reset(fieldNames_VersionResponse)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *VersionResponse) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Version is missing")
	} else if len(v.Version) == 0 {
		errs = append(errs, "field Version is not set")
	}
	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field Commit is missing")
	} else if len(v.Commit) == 0 {
		errs = append(errs, "field Commit is not set")
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

func (v *AuthorizationRequired) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *AuthorizationRequired) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadBytes(1); ok {
		v.Key = x
	}
	if x, ok := reader.ReadString(2); ok {
		v.Version = x
	}

	seen, err := reader.Reset(fieldNames_AuthorizationRequired)
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

func (v *VersionResponse) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *VersionResponse) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadString(1); ok {
		v.Version = x
	}
	if x, ok := reader.ReadString(2); ok {
		v.Commit = x
	}

	seen, err := reader.Reset(fieldNames_VersionResponse)
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

func (v *AdiListResponse) MarshalJSON() ([]byte, error) {
	u := struct {
		Urls encoding.JsonList[string] `json:"urls,omitempty"`
	}{}
	u.Urls = v.Urls
	return json.Marshal(&u)
}

func (v *AuthorizationRequired) MarshalJSON() ([]byte, error) {
	u := struct {
		Key     *string `json:"key,omitempty"`
		Version string  `json:"version,omitempty"`
	}{}
	u.Key = encoding.BytesToJSON(v.Key)
	u.Version = v.Version
	return json.Marshal(&u)
}

func (v *DecodeRequest) MarshalJSON() ([]byte, error) {
	u := struct {
		DataBinary *string `json:"dataBinary,omitempty"`
	}{}
	u.DataBinary = encoding.BytesToJSON(v.DataBinary)
	return json.Marshal(&u)
}

func (v *EncodeAccountResponse) MarshalJSON() ([]byte, error) {
	u := struct {
		AccountBinary *string `json:"accountBinary,omitempty"`
	}{}
	u.AccountBinary = encoding.BytesToJSON(v.AccountBinary)
	return json.Marshal(&u)
}

func (v *EncodeTransactionBodyResponse) MarshalJSON() ([]byte, error) {
	u := struct {
		TransactionBodyBinary *string `json:"transactionBodyBinary,omitempty"`
	}{}
	u.TransactionBodyBinary = encoding.BytesToJSON(v.TransactionBodyBinary)
	return json.Marshal(&u)
}

func (v *EncodeTransactionHeaderResponse) MarshalJSON() ([]byte, error) {
	u := struct {
		TransactionHeaderBinary *string `json:"transactionHeaderBinary,omitempty"`
	}{}
	u.TransactionHeaderBinary = encoding.BytesToJSON(v.TransactionHeaderBinary)
	return json.Marshal(&u)
}

func (v *EncodeTransactionResponse) MarshalJSON() ([]byte, error) {
	u := struct {
		TransactionBinary *string `json:"transactionBinary,omitempty"`
		TransactionHash   *string `json:"transactionHash,omitempty"`
	}{}
	u.TransactionBinary = encoding.BytesToJSON(v.TransactionBinary)
	u.TransactionHash = encoding.BytesToJSON(v.TransactionHash)
	return json.Marshal(&u)
}

func (v *KeyData) MarshalJSON() ([]byte, error) {
	u := struct {
		Name       string                 `json:"name,omitempty"`
		PublicKey  *string                `json:"publicKey,omitempty"`
		Derivation string                 `json:"derivation,omitempty"`
		KeyType    protocol.SignatureType `json:"keyType,omitempty"`
	}{}
	u.Name = v.Name
	u.PublicKey = encoding.BytesToJSON(v.PublicKey)
	u.Derivation = v.Derivation
	u.KeyType = v.KeyType
	return json.Marshal(&u)
}

func (v *KeyListResponse) MarshalJSON() ([]byte, error) {
	u := struct {
		KeyList encoding.JsonList[KeyData] `json:"keyList,omitempty"`
	}{}
	u.KeyList = v.KeyList
	return json.Marshal(&u)
}

func (v *SignResponse) MarshalJSON() ([]byte, error) {
	u := struct {
		Signature *string `json:"signature,omitempty"`
		PublicKey *string `json:"publicKey,omitempty"`
	}{}
	u.Signature = encoding.BytesToJSON(v.Signature)
	u.PublicKey = encoding.BytesToJSON(v.PublicKey)
	return json.Marshal(&u)
}

func (v *AdiListResponse) UnmarshalJSON(data []byte) error {
	u := struct {
		Urls encoding.JsonList[string] `json:"urls,omitempty"`
	}{}
	u.Urls = v.Urls
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Urls = u.Urls
	return nil
}

func (v *AuthorizationRequired) UnmarshalJSON(data []byte) error {
	u := struct {
		Key     *string `json:"key,omitempty"`
		Version string  `json:"version,omitempty"`
	}{}
	u.Key = encoding.BytesToJSON(v.Key)
	u.Version = v.Version
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	if x, err := encoding.BytesFromJSON(u.Key); err != nil {
		return fmt.Errorf("error decoding Key: %w", err)
	} else {
		v.Key = x
	}
	v.Version = u.Version
	return nil
}

func (v *DecodeRequest) UnmarshalJSON(data []byte) error {
	u := struct {
		DataBinary *string `json:"dataBinary,omitempty"`
	}{}
	u.DataBinary = encoding.BytesToJSON(v.DataBinary)
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	if x, err := encoding.BytesFromJSON(u.DataBinary); err != nil {
		return fmt.Errorf("error decoding DataBinary: %w", err)
	} else {
		v.DataBinary = x
	}
	return nil
}

func (v *EncodeAccountResponse) UnmarshalJSON(data []byte) error {
	u := struct {
		AccountBinary *string `json:"accountBinary,omitempty"`
	}{}
	u.AccountBinary = encoding.BytesToJSON(v.AccountBinary)
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	if x, err := encoding.BytesFromJSON(u.AccountBinary); err != nil {
		return fmt.Errorf("error decoding AccountBinary: %w", err)
	} else {
		v.AccountBinary = x
	}
	return nil
}

func (v *EncodeTransactionBodyResponse) UnmarshalJSON(data []byte) error {
	u := struct {
		TransactionBodyBinary *string `json:"transactionBodyBinary,omitempty"`
	}{}
	u.TransactionBodyBinary = encoding.BytesToJSON(v.TransactionBodyBinary)
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	if x, err := encoding.BytesFromJSON(u.TransactionBodyBinary); err != nil {
		return fmt.Errorf("error decoding TransactionBodyBinary: %w", err)
	} else {
		v.TransactionBodyBinary = x
	}
	return nil
}

func (v *EncodeTransactionHeaderResponse) UnmarshalJSON(data []byte) error {
	u := struct {
		TransactionHeaderBinary *string `json:"transactionHeaderBinary,omitempty"`
	}{}
	u.TransactionHeaderBinary = encoding.BytesToJSON(v.TransactionHeaderBinary)
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	if x, err := encoding.BytesFromJSON(u.TransactionHeaderBinary); err != nil {
		return fmt.Errorf("error decoding TransactionHeaderBinary: %w", err)
	} else {
		v.TransactionHeaderBinary = x
	}
	return nil
}

func (v *EncodeTransactionResponse) UnmarshalJSON(data []byte) error {
	u := struct {
		TransactionBinary *string `json:"transactionBinary,omitempty"`
		TransactionHash   *string `json:"transactionHash,omitempty"`
	}{}
	u.TransactionBinary = encoding.BytesToJSON(v.TransactionBinary)
	u.TransactionHash = encoding.BytesToJSON(v.TransactionHash)
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	if x, err := encoding.BytesFromJSON(u.TransactionBinary); err != nil {
		return fmt.Errorf("error decoding TransactionBinary: %w", err)
	} else {
		v.TransactionBinary = x
	}
	if x, err := encoding.BytesFromJSON(u.TransactionHash); err != nil {
		return fmt.Errorf("error decoding TransactionHash: %w", err)
	} else {
		v.TransactionHash = x
	}
	return nil
}

func (v *KeyData) UnmarshalJSON(data []byte) error {
	u := struct {
		Name       string                 `json:"name,omitempty"`
		PublicKey  *string                `json:"publicKey,omitempty"`
		Derivation string                 `json:"derivation,omitempty"`
		KeyType    protocol.SignatureType `json:"keyType,omitempty"`
	}{}
	u.Name = v.Name
	u.PublicKey = encoding.BytesToJSON(v.PublicKey)
	u.Derivation = v.Derivation
	u.KeyType = v.KeyType
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Name = u.Name
	if x, err := encoding.BytesFromJSON(u.PublicKey); err != nil {
		return fmt.Errorf("error decoding PublicKey: %w", err)
	} else {
		v.PublicKey = x
	}
	v.Derivation = u.Derivation
	v.KeyType = u.KeyType
	return nil
}

func (v *KeyListResponse) UnmarshalJSON(data []byte) error {
	u := struct {
		KeyList encoding.JsonList[KeyData] `json:"keyList,omitempty"`
	}{}
	u.KeyList = v.KeyList
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.KeyList = u.KeyList
	return nil
}

func (v *SignResponse) UnmarshalJSON(data []byte) error {
	u := struct {
		Signature *string `json:"signature,omitempty"`
		PublicKey *string `json:"publicKey,omitempty"`
	}{}
	u.Signature = encoding.BytesToJSON(v.Signature)
	u.PublicKey = encoding.BytesToJSON(v.PublicKey)
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	if x, err := encoding.BytesFromJSON(u.Signature); err != nil {
		return fmt.Errorf("error decoding Signature: %w", err)
	} else {
		v.Signature = x
	}
	if x, err := encoding.BytesFromJSON(u.PublicKey); err != nil {
		return fmt.Errorf("error decoding PublicKey: %w", err)
	} else {
		v.PublicKey = x
	}
	return nil
}
