package protocol

// GENERATED BY go run ./tools/cmd/gen-enum. DO NOT EDIT.

import (
	"encoding/json"
	"fmt"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
)

// AccountTypeUnknown represents an unknown account type.
const AccountTypeUnknown AccountType = 0

// AccountTypeAnchor is one or more Merkle DAG anchors.
const AccountTypeAnchor AccountType = 1

// AccountTypeIdentity is an Identity account, aka an ADI.
const AccountTypeIdentity AccountType = 2

// AccountTypeTokenIssuer is a Token Issuer account.
const AccountTypeTokenIssuer AccountType = 3

// AccountTypeTokenAccount is an ADI Token Account.
const AccountTypeTokenAccount AccountType = 4

// AccountTypeLiteTokenAccount is a Lite Token Account.
const AccountTypeLiteTokenAccount AccountType = 5

// AccountTypeTransaction is a completed transaction.
const AccountTypeTransaction AccountType = 7

// AccountTypePendingTransaction is a pending transaction.
const AccountTypePendingTransaction AccountType = 8

// AccountTypeKeyPage is a Key Page account.
const AccountTypeKeyPage AccountType = 9

// AccountTypeKeyBook is a Key Book account.
const AccountTypeKeyBook AccountType = 10

// AccountTypeDataAccount is an ADI Data Account.
const AccountTypeDataAccount AccountType = 11

// AccountTypeLiteDataAccount is a Lite Data Account.
const AccountTypeLiteDataAccount AccountType = 12

// AccountTypeInternalLedger is a ledger that tracks the state of internal operations.
const AccountTypeInternalLedger AccountType = 14

// AccountTypeLiteIdentity is a lite identity account.
const AccountTypeLiteIdentity AccountType = 15

// ChainTypeUnknown is used when the chain type is not known.
const ChainTypeUnknown ChainType = 0

// ChainTypeTransaction holds transaction hashes.
const ChainTypeTransaction ChainType = 1

// ChainTypeAnchor holds chain anchors.
const ChainTypeAnchor ChainType = 2

// ChainTypeData holds data entry hashes.
const ChainTypeData ChainType = 3

// ErrorCodeOK indicates the request succeeded.
const ErrorCodeOK ErrorCode = 0

// ErrorCodeEncodingError indicates the request could not be decoded.
const ErrorCodeEncodingError ErrorCode = 1

// ErrorCodeBadNonce indicates the transaction nonce was rejected.
const ErrorCodeBadNonce ErrorCode = 2

// ErrorCodeDidPanic indicates the request failed due to a fatal error.
const ErrorCodeDidPanic ErrorCode = 3

// ErrorCodeUnknownError indicates the request failed due to an unknown error.
const ErrorCodeUnknownError ErrorCode = 4

// ErrorCodeNotFound indicates the request specified a record that does not exist.
const ErrorCodeNotFound ErrorCode = 5

// ErrorCodeTxnRange is returned when txn range query fails.
const ErrorCodeTxnRange ErrorCode = 6

// ErrorCodeTxnHistory is returned when txn history query fails.
const ErrorCodeTxnHistory ErrorCode = 7

// ErrorCodeInvalidURL is returned when invalid URL is passed in query.
const ErrorCodeInvalidURL ErrorCode = 8

// ErrorCodeDirectoryURL is returned when invalid directory URL is passed in query.
const ErrorCodeDirectoryURL ErrorCode = 9

// ErrorCodeChainIdError is returned when query by in id fails.
const ErrorCodeChainIdError ErrorCode = 10

// ErrorCodeRoutingChainId is returned when setting routing chain id fails.
const ErrorCodeRoutingChainId ErrorCode = 11

// ErrorCodeCheckTxError is returned when txn validation check fails.
const ErrorCodeCheckTxError ErrorCode = 12

// ErrorCodeDeliverTxError is returned when txn deliver method fails.
const ErrorCodeDeliverTxError ErrorCode = 13

// ErrorCodeTxnStateError is returned when adding txn to state fails.
const ErrorCodeTxnStateError ErrorCode = 14

// ErrorCodeRecordTxnError is returned when storing pending state updates fail.
const ErrorCodeRecordTxnError ErrorCode = 15

// ErrorCodeSyntheticTxnError is returned when submit synthetic txn fails.
const ErrorCodeSyntheticTxnError ErrorCode = 16

// ErrorCodeMarshallingError is returned when marshaling  object or binary fails.
const ErrorCodeMarshallingError ErrorCode = 17

// ErrorCodeUnMarshallingError is returned when unmarshaling  object or binary fails.
const ErrorCodeUnMarshallingError ErrorCode = 18

// ErrorCodeInvalidQueryType is returned when query type in request is not matched with the available ones.
const ErrorCodeInvalidQueryType ErrorCode = 19

// ErrorCodeInvalidTxnType is returned when txn type passed is not available.
const ErrorCodeInvalidTxnType ErrorCode = 20

// ErrorCodeValidateTxnError is returned when execution validation of txn fails.
const ErrorCodeValidateTxnError ErrorCode = 21

// ErrorCodeInvalidTxnError is returned when txn doesn't contains proper data.
const ErrorCodeInvalidTxnError ErrorCode = 22

// ErrorCodeAddTxnError is returned when adding txn to state db fails.
const ErrorCodeAddTxnError ErrorCode = 23

// ErrorCodeDataUrlError is returned when a url passed to a data query fails.
const ErrorCodeDataUrlError ErrorCode = 24

// ErrorCodeDataEntryHashError is returned when an entry hash query fails on a data chain.
const ErrorCodeDataEntryHashError ErrorCode = 25

// ErrorCodeTxnQueryError is returned when txn is not found.
const ErrorCodeTxnQueryError ErrorCode = 26

// KeyPageOperationUnknown is used when the key page operation is not known.
const KeyPageOperationUnknown KeyPageOperation = 0

// KeyPageOperationUpdate replaces a key in the page with a new key.
const KeyPageOperationUpdate KeyPageOperation = 1

// KeyPageOperationRemove removes a key from the page.
const KeyPageOperationRemove KeyPageOperation = 2

// KeyPageOperationAdd adds a key to the page.
const KeyPageOperationAdd KeyPageOperation = 3

// KeyPageOperationSetThreshold sets the signing threshold (the M of "M of N" signatures required).
const KeyPageOperationSetThreshold KeyPageOperation = 4

// ObjectTypeUnknown is used when the object type is not known.
const ObjectTypeUnknown ObjectType = 0

// ObjectTypeAccount represents an account object.
const ObjectTypeAccount ObjectType = 1

// ObjectTypeTransaction represents a transaction object.
const ObjectTypeTransaction ObjectType = 2

// SignatureTypeUnknown is used when the signature type is not known.
const SignatureTypeUnknown SignatureType = 0

// SignatureTypeLegacyED25519 represents a legacy ED25519 signature.
const SignatureTypeLegacyED25519 SignatureType = 1

// SignatureTypeED25519 represents an ED25519 signature.
const SignatureTypeED25519 SignatureType = 2

// TransactionMaxUser is the highest number reserved for user transactions.
const TransactionMaxUser TransactionMax = 47

// TransactionMaxSynthetic is the highest number reserved for synthetic transactions.
const TransactionMaxSynthetic TransactionMax = 95

// TransactionMaxInternal is the highest number reserved for internal transactions.
const TransactionMaxInternal TransactionMax = 255

// TransactionTypeUnknown represents an unknown transaction type.
const TransactionTypeUnknown TransactionType = 0

// TransactionTypeCreateIdentity creates an ADI, which produces a synthetic chain.
const TransactionTypeCreateIdentity TransactionType = 1

// TransactionTypeCreateTokenAccount creates an ADI token account, which produces a synthetic chain create transaction.
const TransactionTypeCreateTokenAccount TransactionType = 2

// TransactionTypeSendTokens transfers tokens between token accounts, which produces a synthetic deposit tokens transaction.
const TransactionTypeSendTokens TransactionType = 3

// TransactionTypeCreateDataAccount creates an ADI Data Account, which produces a synthetic chain create transaction.
const TransactionTypeCreateDataAccount TransactionType = 4

// TransactionTypeWriteData writes data to an ADI Data Account, which *does not* produce a synthetic transaction.
const TransactionTypeWriteData TransactionType = 5

// TransactionTypeWriteDataTo writes data to a Lite Data Account, which produces a synthetic write data transaction.
const TransactionTypeWriteDataTo TransactionType = 6

// TransactionTypeAcmeFaucet produces a synthetic deposit tokens transaction that deposits ACME tokens into a lite token account.
const TransactionTypeAcmeFaucet TransactionType = 7

// TransactionTypeCreateToken creates a token issuer, which produces a synthetic chain create transaction.
const TransactionTypeCreateToken TransactionType = 8

// TransactionTypeIssueTokens issues tokens to a token account, which produces a synthetic token deposit transaction.
const TransactionTypeIssueTokens TransactionType = 9

// TransactionTypeBurnTokens burns tokens from a token account, which produces a synthetic burn tokens transaction.
const TransactionTypeBurnTokens TransactionType = 10

// TransactionTypeCreateKeyPage creates a key page, which produces a synthetic chain create transaction.
const TransactionTypeCreateKeyPage TransactionType = 12

// TransactionTypeCreateKeyBook creates a key book, which produces a synthetic chain create transaction.
const TransactionTypeCreateKeyBook TransactionType = 13

// TransactionTypeAddCredits converts ACME tokens to credits, which produces a synthetic deposit credits transaction.
const TransactionTypeAddCredits TransactionType = 14

// TransactionTypeUpdateKeyPage adds, removes, or updates keys in a key page, which *does not* produce a synthetic transaction.
const TransactionTypeUpdateKeyPage TransactionType = 15

// TransactionTypeUpdateManager updates manager for the existing chain.
const TransactionTypeUpdateManager TransactionType = 16

// TransactionTypeRemoveManager remove manager from existing chain.
const TransactionTypeRemoveManager TransactionType = 17

// TransactionTypeSignPending is used to sign a pending transaction.
const TransactionTypeSignPending TransactionType = 48

// TransactionTypeSyntheticCreateChain creates or updates chains.
const TransactionTypeSyntheticCreateChain TransactionType = 49

// TransactionTypeSyntheticWriteData writes data to a data account.
const TransactionTypeSyntheticWriteData TransactionType = 50

// TransactionTypeSyntheticDepositTokens deposits tokens into token accounts.
const TransactionTypeSyntheticDepositTokens TransactionType = 51

// TransactionTypeSyntheticAnchor anchors one network to another.
const TransactionTypeSyntheticAnchor TransactionType = 52

// TransactionTypeSyntheticDepositCredits deposits credits into a credit holder.
const TransactionTypeSyntheticDepositCredits TransactionType = 53

// TransactionTypeSyntheticBurnTokens returns tokens to a token issuer's pool of issuable tokens.
const TransactionTypeSyntheticBurnTokens TransactionType = 54

// TransactionTypeSyntheticMirror mirrors records from one network to another.
const TransactionTypeSyntheticMirror TransactionType = 56

// TransactionTypeSegWitDataEntry is a surrogate transaction segregated witness for a WriteData transaction.
const TransactionTypeSegWitDataEntry TransactionType = 57

// TransactionTypeInternalGenesis initializes system chains.
const TransactionTypeInternalGenesis TransactionType = 96

// TransactionTypeInternalSendTransactions reserved for internal send.
const TransactionTypeInternalSendTransactions TransactionType = 97

// TransactionTypeInternalTransactionsSigned notifies the executor of synthetic transactions that have been signed.
const TransactionTypeInternalTransactionsSigned TransactionType = 98

// TransactionTypeInternalTransactionsSent notifies the executor of synthetic transactions that have been sent.
const TransactionTypeInternalTransactionsSent TransactionType = 99

// TransactionTypeCreateDirectory creates an ADI directory, which produces a synthetic chain create transaction.
const TransactionTypeCreateDirectory TransactionType = 100

// ID returns the ID of the Account Type
func (v AccountType) ID() uint64 { return uint64(v) }

// Set sets the value. Set returns false if the value is invalid.
func (v *AccountType) Set(id uint64) bool {
	u := AccountType(id)
	switch u {
	case AccountTypeUnknown, AccountTypeAnchor, AccountTypeIdentity, AccountTypeTokenIssuer, AccountTypeTokenAccount, AccountTypeLiteTokenAccount, AccountTypeTransaction, AccountTypePendingTransaction, AccountTypeKeyPage, AccountTypeKeyBook, AccountTypeDataAccount, AccountTypeLiteDataAccount, AccountTypeInternalLedger, AccountTypeLiteIdentity:
		*v = u
		return true
	default:
		return false
	}
}

// String returns the name of the Account Type
func (v AccountType) String() string {
	switch v {
	case AccountTypeUnknown:
		return "unknown"
	case AccountTypeAnchor:
		return "anchor"
	case AccountTypeIdentity:
		return "identity"
	case AccountTypeTokenIssuer:
		return "tokenIssuer"
	case AccountTypeTokenAccount:
		return "tokenAccount"
	case AccountTypeLiteTokenAccount:
		return "liteTokenAccount"
	case AccountTypeTransaction:
		return "transaction"
	case AccountTypePendingTransaction:
		return "pendingTransaction"
	case AccountTypeKeyPage:
		return "keyPage"
	case AccountTypeKeyBook:
		return "keyBook"
	case AccountTypeDataAccount:
		return "dataAccount"
	case AccountTypeLiteDataAccount:
		return "liteDataAccount"
	case AccountTypeInternalLedger:
		return "internalLedger"
	case AccountTypeLiteIdentity:
		return "liteIdentity"
	default:
		return fmt.Sprintf("AccountType:%d", v)
	}
}

// AccountTypeByName returns the named Account Type.
func AccountTypeByName(name string) (AccountType, bool) {
	switch name {
	case "unknown":
		return AccountTypeUnknown, true
	case "anchor":
		return AccountTypeAnchor, true
	case "identity":
		return AccountTypeIdentity, true
	case "tokenIssuer":
		return AccountTypeTokenIssuer, true
	case "token":
		return AccountTypeTokenIssuer, true
	case "tokenAccount":
		return AccountTypeTokenAccount, true
	case "liteTokenAccount":
		return AccountTypeLiteTokenAccount, true
	case "transaction":
		return AccountTypeTransaction, true
	case "pendingTransaction":
		return AccountTypePendingTransaction, true
	case "keyPage":
		return AccountTypeKeyPage, true
	case "keyBook":
		return AccountTypeKeyBook, true
	case "dataAccount":
		return AccountTypeDataAccount, true
	case "liteDataAccount":
		return AccountTypeLiteDataAccount, true
	case "internalLedger":
		return AccountTypeInternalLedger, true
	case "liteIdentity":
		return AccountTypeLiteIdentity, true
	default:
		return 0, false
	}
}

// MarshalJSON marshals the Account Type to JSON as a string.
func (v AccountType) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// UnmarshalJSON unmarshals the Account Type from JSON as a string.
func (v *AccountType) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	var ok bool
	*v, ok = AccountTypeByName(s)
	if !ok || strings.ContainsRune(v.String(), ':') {
		return fmt.Errorf("invalid Account Type %q", s)
	}
	return nil
}

// BinarySize returns the number of bytes required to binary marshal the Account Type.
func (v AccountType) BinarySize() int {
	return encoding.UvarintBinarySize(v.ID())
}

// MarshalBinary marshals the Account Type to bytes as a unsigned varint.
func (v AccountType) MarshalBinary() ([]byte, error) {
	return encoding.UvarintMarshalBinary(v.ID()), nil
}

// UnmarshalBinary unmarshals the Account Type from bytes as a unsigned varint.
func (v *AccountType) UnmarshalBinary(data []byte) error {
	u, err := encoding.UvarintUnmarshalBinary(data)
	if err != nil {
		return err
	}

	*v = AccountType(u)
	return nil
}

// ID returns the ID of the Chain Type
func (v ChainType) ID() uint64 { return uint64(v) }

// Set sets the value. Set returns false if the value is invalid.
func (v *ChainType) Set(id uint64) bool {
	u := ChainType(id)
	switch u {
	case ChainTypeUnknown, ChainTypeTransaction, ChainTypeAnchor, ChainTypeData:
		*v = u
		return true
	default:
		return false
	}
}

// String returns the name of the Chain Type
func (v ChainType) String() string {
	switch v {
	case ChainTypeUnknown:
		return "unknown"
	case ChainTypeTransaction:
		return "transaction"
	case ChainTypeAnchor:
		return "anchor"
	case ChainTypeData:
		return "data"
	default:
		return fmt.Sprintf("ChainType:%d", v)
	}
}

// ChainTypeByName returns the named Chain Type.
func ChainTypeByName(name string) (ChainType, bool) {
	switch name {
	case "unknown":
		return ChainTypeUnknown, true
	case "transaction":
		return ChainTypeTransaction, true
	case "anchor":
		return ChainTypeAnchor, true
	case "data":
		return ChainTypeData, true
	default:
		return 0, false
	}
}

// MarshalJSON marshals the Chain Type to JSON as a string.
func (v ChainType) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// UnmarshalJSON unmarshals the Chain Type from JSON as a string.
func (v *ChainType) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	var ok bool
	*v, ok = ChainTypeByName(s)
	if !ok || strings.ContainsRune(v.String(), ':') {
		return fmt.Errorf("invalid Chain Type %q", s)
	}
	return nil
}

// BinarySize returns the number of bytes required to binary marshal the Chain Type.
func (v ChainType) BinarySize() int {
	return encoding.UvarintBinarySize(v.ID())
}

// MarshalBinary marshals the Chain Type to bytes as a unsigned varint.
func (v ChainType) MarshalBinary() ([]byte, error) {
	return encoding.UvarintMarshalBinary(v.ID()), nil
}

// UnmarshalBinary unmarshals the Chain Type from bytes as a unsigned varint.
func (v *ChainType) UnmarshalBinary(data []byte) error {
	u, err := encoding.UvarintUnmarshalBinary(data)
	if err != nil {
		return err
	}

	*v = ChainType(u)
	return nil
}

// ID returns the ID of the Error Code
func (v ErrorCode) ID() uint64 { return uint64(v) }

// Set sets the value. Set returns false if the value is invalid.
func (v *ErrorCode) Set(id uint64) bool {
	u := ErrorCode(id)
	switch u {
	case ErrorCodeOK, ErrorCodeEncodingError, ErrorCodeBadNonce, ErrorCodeDidPanic, ErrorCodeUnknownError, ErrorCodeNotFound, ErrorCodeTxnRange, ErrorCodeTxnHistory, ErrorCodeInvalidURL, ErrorCodeDirectoryURL, ErrorCodeChainIdError, ErrorCodeRoutingChainId, ErrorCodeCheckTxError, ErrorCodeDeliverTxError, ErrorCodeTxnStateError, ErrorCodeRecordTxnError, ErrorCodeSyntheticTxnError, ErrorCodeMarshallingError, ErrorCodeUnMarshallingError, ErrorCodeInvalidQueryType, ErrorCodeInvalidTxnType, ErrorCodeValidateTxnError, ErrorCodeInvalidTxnError, ErrorCodeAddTxnError, ErrorCodeDataUrlError, ErrorCodeDataEntryHashError, ErrorCodeTxnQueryError:
		*v = u
		return true
	default:
		return false
	}
}

// String returns the name of the Error Code
func (v ErrorCode) String() string {
	switch v {
	case ErrorCodeOK:
		return "oK"
	case ErrorCodeEncodingError:
		return "encodingError"
	case ErrorCodeBadNonce:
		return "badNonce"
	case ErrorCodeDidPanic:
		return "didPanic"
	case ErrorCodeUnknownError:
		return "unknownError"
	case ErrorCodeNotFound:
		return "notFound"
	case ErrorCodeTxnRange:
		return "txnRange"
	case ErrorCodeTxnHistory:
		return "txnHistory"
	case ErrorCodeInvalidURL:
		return "invalidURL"
	case ErrorCodeDirectoryURL:
		return "directoryURL"
	case ErrorCodeChainIdError:
		return "chainIdError"
	case ErrorCodeRoutingChainId:
		return "routingChainId"
	case ErrorCodeCheckTxError:
		return "checkTxError"
	case ErrorCodeDeliverTxError:
		return "deliverTxError"
	case ErrorCodeTxnStateError:
		return "txnStateError"
	case ErrorCodeRecordTxnError:
		return "recordTxnError"
	case ErrorCodeSyntheticTxnError:
		return "syntheticTxnError"
	case ErrorCodeMarshallingError:
		return "marshallingError"
	case ErrorCodeUnMarshallingError:
		return "unMarshallingError"
	case ErrorCodeInvalidQueryType:
		return "invalidQueryType"
	case ErrorCodeInvalidTxnType:
		return "invalidTxnType"
	case ErrorCodeValidateTxnError:
		return "validateTxnError"
	case ErrorCodeInvalidTxnError:
		return "invalidTxnError"
	case ErrorCodeAddTxnError:
		return "addTxnError"
	case ErrorCodeDataUrlError:
		return "dataUrlError"
	case ErrorCodeDataEntryHashError:
		return "dataEntryHashError"
	case ErrorCodeTxnQueryError:
		return "txnQueryError"
	default:
		return fmt.Sprintf("ErrorCode:%d", v)
	}
}

// ErrorCodeByName returns the named Error Code.
func ErrorCodeByName(name string) (ErrorCode, bool) {
	switch name {
	case "oK":
		return ErrorCodeOK, true
	case "encodingError":
		return ErrorCodeEncodingError, true
	case "badNonce":
		return ErrorCodeBadNonce, true
	case "didPanic":
		return ErrorCodeDidPanic, true
	case "unknownError":
		return ErrorCodeUnknownError, true
	case "notFound":
		return ErrorCodeNotFound, true
	case "txnRange":
		return ErrorCodeTxnRange, true
	case "txnHistory":
		return ErrorCodeTxnHistory, true
	case "invalidURL":
		return ErrorCodeInvalidURL, true
	case "directoryURL":
		return ErrorCodeDirectoryURL, true
	case "chainIdError":
		return ErrorCodeChainIdError, true
	case "routingChainId":
		return ErrorCodeRoutingChainId, true
	case "checkTxError":
		return ErrorCodeCheckTxError, true
	case "deliverTxError":
		return ErrorCodeDeliverTxError, true
	case "txnStateError":
		return ErrorCodeTxnStateError, true
	case "recordTxnError":
		return ErrorCodeRecordTxnError, true
	case "syntheticTxnError":
		return ErrorCodeSyntheticTxnError, true
	case "marshallingError":
		return ErrorCodeMarshallingError, true
	case "unMarshallingError":
		return ErrorCodeUnMarshallingError, true
	case "invalidQueryType":
		return ErrorCodeInvalidQueryType, true
	case "invalidTxnType":
		return ErrorCodeInvalidTxnType, true
	case "validateTxnError":
		return ErrorCodeValidateTxnError, true
	case "invalidTxnError":
		return ErrorCodeInvalidTxnError, true
	case "addTxnError":
		return ErrorCodeAddTxnError, true
	case "dataUrlError":
		return ErrorCodeDataUrlError, true
	case "dataEntryHashError":
		return ErrorCodeDataEntryHashError, true
	case "txnQueryError":
		return ErrorCodeTxnQueryError, true
	default:
		return 0, false
	}
}

// MarshalJSON marshals the Error Code to JSON as a string.
func (v ErrorCode) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// UnmarshalJSON unmarshals the Error Code from JSON as a string.
func (v *ErrorCode) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	var ok bool
	*v, ok = ErrorCodeByName(s)
	if !ok || strings.ContainsRune(v.String(), ':') {
		return fmt.Errorf("invalid Error Code %q", s)
	}
	return nil
}

// BinarySize returns the number of bytes required to binary marshal the Error Code.
func (v ErrorCode) BinarySize() int {
	return encoding.UvarintBinarySize(v.ID())
}

// MarshalBinary marshals the Error Code to bytes as a unsigned varint.
func (v ErrorCode) MarshalBinary() ([]byte, error) {
	return encoding.UvarintMarshalBinary(v.ID()), nil
}

// UnmarshalBinary unmarshals the Error Code from bytes as a unsigned varint.
func (v *ErrorCode) UnmarshalBinary(data []byte) error {
	u, err := encoding.UvarintUnmarshalBinary(data)
	if err != nil {
		return err
	}

	*v = ErrorCode(u)
	return nil
}

// ID returns the ID of the Key PageOpe ration
func (v KeyPageOperation) ID() uint64 { return uint64(v) }

// Set sets the value. Set returns false if the value is invalid.
func (v *KeyPageOperation) Set(id uint64) bool {
	u := KeyPageOperation(id)
	switch u {
	case KeyPageOperationUnknown, KeyPageOperationUpdate, KeyPageOperationRemove, KeyPageOperationAdd, KeyPageOperationSetThreshold:
		*v = u
		return true
	default:
		return false
	}
}

// String returns the name of the Key PageOpe ration
func (v KeyPageOperation) String() string {
	switch v {
	case KeyPageOperationUnknown:
		return "unknown"
	case KeyPageOperationUpdate:
		return "update"
	case KeyPageOperationRemove:
		return "remove"
	case KeyPageOperationAdd:
		return "add"
	case KeyPageOperationSetThreshold:
		return "setThreshold"
	default:
		return fmt.Sprintf("KeyPageOperation:%d", v)
	}
}

// KeyPageOperationByName returns the named Key PageOpe ration.
func KeyPageOperationByName(name string) (KeyPageOperation, bool) {
	switch name {
	case "unknown":
		return KeyPageOperationUnknown, true
	case "update":
		return KeyPageOperationUpdate, true
	case "remove":
		return KeyPageOperationRemove, true
	case "add":
		return KeyPageOperationAdd, true
	case "setThreshold":
		return KeyPageOperationSetThreshold, true
	default:
		return 0, false
	}
}

// MarshalJSON marshals the Key PageOpe ration to JSON as a string.
func (v KeyPageOperation) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// UnmarshalJSON unmarshals the Key PageOpe ration from JSON as a string.
func (v *KeyPageOperation) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	var ok bool
	*v, ok = KeyPageOperationByName(s)
	if !ok || strings.ContainsRune(v.String(), ':') {
		return fmt.Errorf("invalid Key PageOpe ration %q", s)
	}
	return nil
}

// BinarySize returns the number of bytes required to binary marshal the Key PageOpe ration.
func (v KeyPageOperation) BinarySize() int {
	return encoding.UvarintBinarySize(v.ID())
}

// MarshalBinary marshals the Key PageOpe ration to bytes as a unsigned varint.
func (v KeyPageOperation) MarshalBinary() ([]byte, error) {
	return encoding.UvarintMarshalBinary(v.ID()), nil
}

// UnmarshalBinary unmarshals the Key PageOpe ration from bytes as a unsigned varint.
func (v *KeyPageOperation) UnmarshalBinary(data []byte) error {
	u, err := encoding.UvarintUnmarshalBinary(data)
	if err != nil {
		return err
	}

	*v = KeyPageOperation(u)
	return nil
}

// ID returns the ID of the Object Type
func (v ObjectType) ID() uint64 { return uint64(v) }

// Set sets the value. Set returns false if the value is invalid.
func (v *ObjectType) Set(id uint64) bool {
	u := ObjectType(id)
	switch u {
	case ObjectTypeUnknown, ObjectTypeAccount, ObjectTypeTransaction:
		*v = u
		return true
	default:
		return false
	}
}

// String returns the name of the Object Type
func (v ObjectType) String() string {
	switch v {
	case ObjectTypeUnknown:
		return "unknown"
	case ObjectTypeAccount:
		return "account"
	case ObjectTypeTransaction:
		return "transaction"
	default:
		return fmt.Sprintf("ObjectType:%d", v)
	}
}

// ObjectTypeByName returns the named Object Type.
func ObjectTypeByName(name string) (ObjectType, bool) {
	switch name {
	case "unknown":
		return ObjectTypeUnknown, true
	case "account":
		return ObjectTypeAccount, true
	case "transaction":
		return ObjectTypeTransaction, true
	default:
		return 0, false
	}
}

// MarshalJSON marshals the Object Type to JSON as a string.
func (v ObjectType) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// UnmarshalJSON unmarshals the Object Type from JSON as a string.
func (v *ObjectType) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	var ok bool
	*v, ok = ObjectTypeByName(s)
	if !ok || strings.ContainsRune(v.String(), ':') {
		return fmt.Errorf("invalid Object Type %q", s)
	}
	return nil
}

// BinarySize returns the number of bytes required to binary marshal the Object Type.
func (v ObjectType) BinarySize() int {
	return encoding.UvarintBinarySize(v.ID())
}

// MarshalBinary marshals the Object Type to bytes as a unsigned varint.
func (v ObjectType) MarshalBinary() ([]byte, error) {
	return encoding.UvarintMarshalBinary(v.ID()), nil
}

// UnmarshalBinary unmarshals the Object Type from bytes as a unsigned varint.
func (v *ObjectType) UnmarshalBinary(data []byte) error {
	u, err := encoding.UvarintUnmarshalBinary(data)
	if err != nil {
		return err
	}

	*v = ObjectType(u)
	return nil
}

// ID returns the ID of the Signature Type
func (v SignatureType) ID() uint64 { return uint64(v) }

// Set sets the value. Set returns false if the value is invalid.
func (v *SignatureType) Set(id uint64) bool {
	u := SignatureType(id)
	switch u {
	case SignatureTypeUnknown, SignatureTypeLegacyED25519, SignatureTypeED25519:
		*v = u
		return true
	default:
		return false
	}
}

// String returns the name of the Signature Type
func (v SignatureType) String() string {
	switch v {
	case SignatureTypeUnknown:
		return "unknown"
	case SignatureTypeLegacyED25519:
		return "legacyED25519"
	case SignatureTypeED25519:
		return "eD25519"
	default:
		return fmt.Sprintf("SignatureType:%d", v)
	}
}

// SignatureTypeByName returns the named Signature Type.
func SignatureTypeByName(name string) (SignatureType, bool) {
	switch name {
	case "unknown":
		return SignatureTypeUnknown, true
	case "legacyED25519":
		return SignatureTypeLegacyED25519, true
	case "eD25519":
		return SignatureTypeED25519, true
	default:
		return 0, false
	}
}

// MarshalJSON marshals the Signature Type to JSON as a string.
func (v SignatureType) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// UnmarshalJSON unmarshals the Signature Type from JSON as a string.
func (v *SignatureType) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	var ok bool
	*v, ok = SignatureTypeByName(s)
	if !ok || strings.ContainsRune(v.String(), ':') {
		return fmt.Errorf("invalid Signature Type %q", s)
	}
	return nil
}

// BinarySize returns the number of bytes required to binary marshal the Signature Type.
func (v SignatureType) BinarySize() int {
	return encoding.UvarintBinarySize(v.ID())
}

// MarshalBinary marshals the Signature Type to bytes as a unsigned varint.
func (v SignatureType) MarshalBinary() ([]byte, error) {
	return encoding.UvarintMarshalBinary(v.ID()), nil
}

// UnmarshalBinary unmarshals the Signature Type from bytes as a unsigned varint.
func (v *SignatureType) UnmarshalBinary(data []byte) error {
	u, err := encoding.UvarintUnmarshalBinary(data)
	if err != nil {
		return err
	}

	*v = SignatureType(u)
	return nil
}

// ID returns the ID of the Transaction Max
func (v TransactionMax) ID() uint64 { return uint64(v) }

// Set sets the value. Set returns false if the value is invalid.
func (v *TransactionMax) Set(id uint64) bool {
	u := TransactionMax(id)
	switch u {
	case TransactionMaxUser, TransactionMaxSynthetic, TransactionMaxInternal:
		*v = u
		return true
	default:
		return false
	}
}

// String returns the name of the Transaction Max
func (v TransactionMax) String() string {
	switch v {
	case TransactionMaxUser:
		return "user"
	case TransactionMaxSynthetic:
		return "synthetic"
	case TransactionMaxInternal:
		return "internal"
	default:
		return fmt.Sprintf("TransactionMax:%d", v)
	}
}

// TransactionMaxByName returns the named Transaction Max.
func TransactionMaxByName(name string) (TransactionMax, bool) {
	switch name {
	case "user":
		return TransactionMaxUser, true
	case "synthetic":
		return TransactionMaxSynthetic, true
	case "internal":
		return TransactionMaxInternal, true
	default:
		return 0, false
	}
}

// MarshalJSON marshals the Transaction Max to JSON as a string.
func (v TransactionMax) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// UnmarshalJSON unmarshals the Transaction Max from JSON as a string.
func (v *TransactionMax) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	var ok bool
	*v, ok = TransactionMaxByName(s)
	if !ok || strings.ContainsRune(v.String(), ':') {
		return fmt.Errorf("invalid Transaction Max %q", s)
	}
	return nil
}

// BinarySize returns the number of bytes required to binary marshal the Transaction Max.
func (v TransactionMax) BinarySize() int {
	return encoding.UvarintBinarySize(v.ID())
}

// MarshalBinary marshals the Transaction Max to bytes as a unsigned varint.
func (v TransactionMax) MarshalBinary() ([]byte, error) {
	return encoding.UvarintMarshalBinary(v.ID()), nil
}

// UnmarshalBinary unmarshals the Transaction Max from bytes as a unsigned varint.
func (v *TransactionMax) UnmarshalBinary(data []byte) error {
	u, err := encoding.UvarintUnmarshalBinary(data)
	if err != nil {
		return err
	}

	*v = TransactionMax(u)
	return nil
}

// ID returns the ID of the Transaction Type
func (v TransactionType) ID() uint64 { return uint64(v) }

// Set sets the value. Set returns false if the value is invalid.
func (v *TransactionType) Set(id uint64) bool {
	u := TransactionType(id)
	switch u {
	case TransactionTypeUnknown, TransactionTypeCreateIdentity, TransactionTypeCreateTokenAccount, TransactionTypeSendTokens, TransactionTypeCreateDataAccount, TransactionTypeWriteData, TransactionTypeWriteDataTo, TransactionTypeAcmeFaucet, TransactionTypeCreateToken, TransactionTypeIssueTokens, TransactionTypeBurnTokens, TransactionTypeCreateKeyPage, TransactionTypeCreateKeyBook, TransactionTypeAddCredits, TransactionTypeUpdateKeyPage, TransactionTypeUpdateManager, TransactionTypeRemoveManager, TransactionTypeSignPending, TransactionTypeSyntheticCreateChain, TransactionTypeSyntheticWriteData, TransactionTypeSyntheticDepositTokens, TransactionTypeSyntheticAnchor, TransactionTypeSyntheticDepositCredits, TransactionTypeSyntheticBurnTokens, TransactionTypeSyntheticMirror, TransactionTypeSegWitDataEntry, TransactionTypeInternalGenesis, TransactionTypeInternalSendTransactions, TransactionTypeInternalTransactionsSigned, TransactionTypeInternalTransactionsSent, TransactionTypeCreateDirectory:
		*v = u
		return true
	default:
		return false
	}
}

// String returns the name of the Transaction Type
func (v TransactionType) String() string {
	switch v {
	case TransactionTypeUnknown:
		return "unknown"
	case TransactionTypeCreateIdentity:
		return "createIdentity"
	case TransactionTypeCreateTokenAccount:
		return "createTokenAccount"
	case TransactionTypeSendTokens:
		return "sendTokens"
	case TransactionTypeCreateDataAccount:
		return "createDataAccount"
	case TransactionTypeWriteData:
		return "writeData"
	case TransactionTypeWriteDataTo:
		return "writeDataTo"
	case TransactionTypeAcmeFaucet:
		return "acmeFaucet"
	case TransactionTypeCreateToken:
		return "createToken"
	case TransactionTypeIssueTokens:
		return "issueTokens"
	case TransactionTypeBurnTokens:
		return "burnTokens"
	case TransactionTypeCreateKeyPage:
		return "createKeyPage"
	case TransactionTypeCreateKeyBook:
		return "createKeyBook"
	case TransactionTypeAddCredits:
		return "addCredits"
	case TransactionTypeUpdateKeyPage:
		return "updateKeyPage"
	case TransactionTypeUpdateManager:
		return "updateManager"
	case TransactionTypeRemoveManager:
		return "removeManager"
	case TransactionTypeSignPending:
		return "signPending"
	case TransactionTypeSyntheticCreateChain:
		return "syntheticCreateChain"
	case TransactionTypeSyntheticWriteData:
		return "syntheticWriteData"
	case TransactionTypeSyntheticDepositTokens:
		return "syntheticDepositTokens"
	case TransactionTypeSyntheticAnchor:
		return "syntheticAnchor"
	case TransactionTypeSyntheticDepositCredits:
		return "syntheticDepositCredits"
	case TransactionTypeSyntheticBurnTokens:
		return "syntheticBurnTokens"
	case TransactionTypeSyntheticMirror:
		return "syntheticMirror"
	case TransactionTypeSegWitDataEntry:
		return "segWitDataEntry"
	case TransactionTypeInternalGenesis:
		return "internalGenesis"
	case TransactionTypeInternalSendTransactions:
		return "internalSendTransactions"
	case TransactionTypeInternalTransactionsSigned:
		return "internalTransactionsSigned"
	case TransactionTypeInternalTransactionsSent:
		return "internalTransactionsSent"
	case TransactionTypeCreateDirectory:
		return "createDirectory"
	default:
		return fmt.Sprintf("TransactionType:%d", v)
	}
}

// TransactionTypeByName returns the named Transaction Type.
func TransactionTypeByName(name string) (TransactionType, bool) {
	switch name {
	case "unknown":
		return TransactionTypeUnknown, true
	case "createIdentity":
		return TransactionTypeCreateIdentity, true
	case "createTokenAccount":
		return TransactionTypeCreateTokenAccount, true
	case "sendTokens":
		return TransactionTypeSendTokens, true
	case "createDataAccount":
		return TransactionTypeCreateDataAccount, true
	case "writeData":
		return TransactionTypeWriteData, true
	case "writeDataTo":
		return TransactionTypeWriteDataTo, true
	case "acmeFaucet":
		return TransactionTypeAcmeFaucet, true
	case "createToken":
		return TransactionTypeCreateToken, true
	case "issueTokens":
		return TransactionTypeIssueTokens, true
	case "burnTokens":
		return TransactionTypeBurnTokens, true
	case "createKeyPage":
		return TransactionTypeCreateKeyPage, true
	case "createKeyBook":
		return TransactionTypeCreateKeyBook, true
	case "addCredits":
		return TransactionTypeAddCredits, true
	case "updateKeyPage":
		return TransactionTypeUpdateKeyPage, true
	case "updateManager":
		return TransactionTypeUpdateManager, true
	case "removeManager":
		return TransactionTypeRemoveManager, true
	case "signPending":
		return TransactionTypeSignPending, true
	case "syntheticCreateChain":
		return TransactionTypeSyntheticCreateChain, true
	case "syntheticWriteData":
		return TransactionTypeSyntheticWriteData, true
	case "syntheticDepositTokens":
		return TransactionTypeSyntheticDepositTokens, true
	case "syntheticAnchor":
		return TransactionTypeSyntheticAnchor, true
	case "syntheticDepositCredits":
		return TransactionTypeSyntheticDepositCredits, true
	case "syntheticBurnTokens":
		return TransactionTypeSyntheticBurnTokens, true
	case "syntheticMirror":
		return TransactionTypeSyntheticMirror, true
	case "segWitDataEntry":
		return TransactionTypeSegWitDataEntry, true
	case "internalGenesis":
		return TransactionTypeInternalGenesis, true
	case "internalSendTransactions":
		return TransactionTypeInternalSendTransactions, true
	case "internalTransactionsSigned":
		return TransactionTypeInternalTransactionsSigned, true
	case "internalTransactionsSent":
		return TransactionTypeInternalTransactionsSent, true
	case "createDirectory":
		return TransactionTypeCreateDirectory, true
	default:
		return 0, false
	}
}

// MarshalJSON marshals the Transaction Type to JSON as a string.
func (v TransactionType) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// UnmarshalJSON unmarshals the Transaction Type from JSON as a string.
func (v *TransactionType) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	var ok bool
	*v, ok = TransactionTypeByName(s)
	if !ok || strings.ContainsRune(v.String(), ':') {
		return fmt.Errorf("invalid Transaction Type %q", s)
	}
	return nil
}

// BinarySize returns the number of bytes required to binary marshal the Transaction Type.
func (v TransactionType) BinarySize() int {
	return encoding.UvarintBinarySize(v.ID())
}

// MarshalBinary marshals the Transaction Type to bytes as a unsigned varint.
func (v TransactionType) MarshalBinary() ([]byte, error) {
	return encoding.UvarintMarshalBinary(v.ID()), nil
}

// UnmarshalBinary unmarshals the Transaction Type from bytes as a unsigned varint.
func (v *TransactionType) UnmarshalBinary(data []byte) error {
	u, err := encoding.UvarintUnmarshalBinary(data)
	if err != nil {
		return err
	}

	*v = TransactionType(u)
	return nil
}
