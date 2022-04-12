package protocol

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
)

// NewAccount creates a new Account for the specified AccountType.
func NewAccount(typ AccountType) (Account, error) {
	switch typ {
	case AccountTypeIdentity:
		return new(ADI), nil
	case AccountTypeAnchor:
		return new(Anchor), nil
	case AccountTypeDataAccount:
		return new(DataAccount), nil
	case AccountTypeInternalLedger:
		return new(InternalLedger), nil
	case AccountTypeInternalSyntheticLedger:
		return new(InternalSyntheticLedger), nil
	case AccountTypeKeyBook:
		return new(KeyBook), nil
	case AccountTypeKeyPage:
		return new(KeyPage), nil
	case AccountTypeLiteDataAccount:
		return new(LiteDataAccount), nil
	case AccountTypeLiteIdentity:
		return new(LiteIdentity), nil
	case AccountTypeLiteTokenAccount:
		return new(LiteTokenAccount), nil
	case AccountTypeTokenAccount:
		return new(TokenAccount), nil
	case AccountTypeTokenIssuer:
		return new(TokenIssuer), nil
	case AccountTypeUnknown:
		return new(UnknownAccount), nil
	case AccountTypeUnknownSigner:
		return new(UnknownSigner), nil
	default:
		return nil, fmt.Errorf("unknown account %v", typ)
	}
}

// UnmarshalAccountType unmarshals the AccountType from the start of a Account.
func UnmarshalAccountType(r io.Reader) (AccountType, error) {
	var typ AccountType
	err := encoding.UnmarshalEnumType(r, &typ)
	return typ, err
}

// UnmarshalAccount unmarshals a Account.
func UnmarshalAccount(data []byte) (Account, error) {
	typ, err := UnmarshalAccountType(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	v, err := NewAccount(typ)
	if err != nil {
		return nil, err
	}

	err = v.UnmarshalBinary(data)
	if err != nil {
		return nil, err
	}

	return v, nil
}

// UnmarshalAccountFrom unmarshals a Account.
func UnmarshalAccountFrom(rd io.ReadSeeker) (Account, error) {
	// Get the reader's current position
	pos, err := rd.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}

	// Read the type code
	typ, err := UnmarshalAccountType(rd)
	if err != nil {
		return nil, err
	}

	// Reset the reader's position
	_, err = rd.Seek(pos, io.SeekStart)
	if err != nil {
		return nil, err
	}

	// Create a new transaction result
	v, err := NewAccount(AccountType(typ))
	if err != nil {
		return nil, err
	}

	// Unmarshal the result
	err = v.UnmarshalBinaryFrom(rd)
	if err != nil {
		return nil, err
	}

	return v, nil
}

// UnmarshalAccountJson unmarshals a Account.
func UnmarshalAccountJSON(data []byte) (Account, error) {
	var typ struct{ Type AccountType }
	err := json.Unmarshal(data, &typ)
	if err != nil {
		return nil, err
	}

	acnt, err := NewAccount(typ.Type)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, acnt)
	if err != nil {
		return nil, err
	}

	return acnt, nil
}

// NewTransactionBody creates a new TransactionBody for the specified TransactionType.
func NewTransactionBody(typ TransactionType) (TransactionBody, error) {
	switch typ {
	case TransactionTypeAcmeFaucet:
		return new(AcmeFaucet), nil
	case TransactionTypeAddCredits:
		return new(AddCredits), nil
	case TransactionTypeAddValidator:
		return new(AddValidator), nil
	case TransactionTypeBurnTokens:
		return new(BurnTokens), nil
	case TransactionTypeCreateDataAccount:
		return new(CreateDataAccount), nil
	case TransactionTypeCreateIdentity:
		return new(CreateIdentity), nil
	case TransactionTypeCreateKeyBook:
		return new(CreateKeyBook), nil
	case TransactionTypeCreateKeyPage:
		return new(CreateKeyPage), nil
	case TransactionTypeCreateToken:
		return new(CreateToken), nil
	case TransactionTypeCreateTokenAccount:
		return new(CreateTokenAccount), nil
	case TransactionTypeInternalGenesis:
		return new(InternalGenesis), nil
	case TransactionTypeInternalSendTransactions:
		return new(InternalSendTransactions), nil
	case TransactionTypeInternalTransactionsSent:
		return new(InternalTransactionsSent), nil
	case TransactionTypeInternalTransactionsSigned:
		return new(InternalTransactionsSigned), nil
	case TransactionTypeIssueTokens:
		return new(IssueTokens), nil
	case TransactionTypeRemote:
		return new(RemoteTransaction), nil
	case TransactionTypeRemoveValidator:
		return new(RemoveValidator), nil
	case TransactionTypeSegWitDataEntry:
		return new(SegWitDataEntry), nil
	case TransactionTypeSendTokens:
		return new(SendTokens), nil
	case TransactionTypeSyntheticAnchor:
		return new(SyntheticAnchor), nil
	case TransactionTypeSyntheticBurnTokens:
		return new(SyntheticBurnTokens), nil
	case TransactionTypeSyntheticCreateChain:
		return new(SyntheticCreateChain), nil
	case TransactionTypeSyntheticDepositCredits:
		return new(SyntheticDepositCredits), nil
	case TransactionTypeSyntheticDepositTokens:
		return new(SyntheticDepositTokens), nil
	case TransactionTypeSyntheticForwardTransaction:
		return new(SyntheticForwardTransaction), nil
	case TransactionTypeSyntheticMirror:
		return new(SyntheticMirror), nil
	case TransactionTypeSyntheticReceipt:
		return new(SyntheticReceipt), nil
	case TransactionTypeSyntheticWriteData:
		return new(SyntheticWriteData), nil
	case TransactionTypeUpdateAccountAuth:
		return new(UpdateAccountAuth), nil
	case TransactionTypeUpdateKey:
		return new(UpdateKey), nil
	case TransactionTypeUpdateKeyPage:
		return new(UpdateKeyPage), nil
	case TransactionTypeUpdateValidatorKey:
		return new(UpdateValidatorKey), nil
	case TransactionTypeWriteData:
		return new(WriteData), nil
	case TransactionTypeWriteDataTo:
		return new(WriteDataTo), nil
	default:
		return nil, fmt.Errorf("unknown transaction %v", typ)
	}
}

// UnmarshalTransactionType unmarshals the TransactionType from the start of a TransactionBody.
func UnmarshalTransactionType(r io.Reader) (TransactionType, error) {
	var typ TransactionType
	err := encoding.UnmarshalEnumType(r, &typ)
	return typ, err
}

// UnmarshalTransactionBody unmarshals a TransactionBody.
func UnmarshalTransactionBody(data []byte) (TransactionBody, error) {
	typ, err := UnmarshalTransactionType(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	v, err := NewTransactionBody(typ)
	if err != nil {
		return nil, err
	}

	err = v.UnmarshalBinary(data)
	if err != nil {
		return nil, err
	}

	return v, nil
}

// UnmarshalTransactionBodyFrom unmarshals a TransactionBody.
func UnmarshalTransactionBodyFrom(rd io.ReadSeeker) (TransactionBody, error) {
	// Get the reader's current position
	pos, err := rd.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}

	// Read the type code
	typ, err := UnmarshalTransactionType(rd)
	if err != nil {
		return nil, err
	}

	// Reset the reader's position
	_, err = rd.Seek(pos, io.SeekStart)
	if err != nil {
		return nil, err
	}

	// Create a new transaction result
	v, err := NewTransactionBody(TransactionType(typ))
	if err != nil {
		return nil, err
	}

	// Unmarshal the result
	err = v.UnmarshalBinaryFrom(rd)
	if err != nil {
		return nil, err
	}

	return v, nil
}

// UnmarshalTransactionBodyJson unmarshals a TransactionBody.
func UnmarshalTransactionBodyJSON(data []byte) (TransactionBody, error) {
	var typ struct{ Type TransactionType }
	err := json.Unmarshal(data, &typ)
	if err != nil {
		return nil, err
	}

	acnt, err := NewTransactionBody(typ.Type)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, acnt)
	if err != nil {
		return nil, err
	}

	return acnt, nil
}

// NewAccountAuthOperation creates a new AccountAuthOperation for the specified AccountAuthOperationType.
func NewAccountAuthOperation(typ AccountAuthOperationType) (AccountAuthOperation, error) {
	switch typ {
	case AccountAuthOperationTypeAddAuthority:
		return new(AddAccountAuthorityOperation), nil
	case AccountAuthOperationTypeDisable:
		return new(DisableAccountAuthOperation), nil
	case AccountAuthOperationTypeEnable:
		return new(EnableAccountAuthOperation), nil
	case AccountAuthOperationTypeRemoveAuthority:
		return new(RemoveAccountAuthorityOperation), nil
	default:
		return nil, fmt.Errorf("unknown account auth operation %v", typ)
	}
}

// UnmarshalAccountAuthOperationType unmarshals the AccountAuthOperationType from the start of a AccountAuthOperation.
func UnmarshalAccountAuthOperationType(r io.Reader) (AccountAuthOperationType, error) {
	var typ AccountAuthOperationType
	err := encoding.UnmarshalEnumType(r, &typ)
	return typ, err
}

// UnmarshalAccountAuthOperation unmarshals a AccountAuthOperation.
func UnmarshalAccountAuthOperation(data []byte) (AccountAuthOperation, error) {
	typ, err := UnmarshalAccountAuthOperationType(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	v, err := NewAccountAuthOperation(typ)
	if err != nil {
		return nil, err
	}

	err = v.UnmarshalBinary(data)
	if err != nil {
		return nil, err
	}

	return v, nil
}

// UnmarshalAccountAuthOperationFrom unmarshals a AccountAuthOperation.
func UnmarshalAccountAuthOperationFrom(rd io.ReadSeeker) (AccountAuthOperation, error) {
	// Get the reader's current position
	pos, err := rd.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}

	// Read the type code
	typ, err := UnmarshalAccountAuthOperationType(rd)
	if err != nil {
		return nil, err
	}

	// Reset the reader's position
	_, err = rd.Seek(pos, io.SeekStart)
	if err != nil {
		return nil, err
	}

	// Create a new transaction result
	v, err := NewAccountAuthOperation(AccountAuthOperationType(typ))
	if err != nil {
		return nil, err
	}

	// Unmarshal the result
	err = v.UnmarshalBinaryFrom(rd)
	if err != nil {
		return nil, err
	}

	return v, nil
}

// UnmarshalAccountAuthOperationJson unmarshals a AccountAuthOperation.
func UnmarshalAccountAuthOperationJSON(data []byte) (AccountAuthOperation, error) {
	var typ struct{ Type AccountAuthOperationType }
	err := json.Unmarshal(data, &typ)
	if err != nil {
		return nil, err
	}

	acnt, err := NewAccountAuthOperation(typ.Type)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, acnt)
	if err != nil {
		return nil, err
	}

	return acnt, nil
}

// NewKeyPageOperation creates a new KeyPageOperation for the specified KeyPageOperationType.
func NewKeyPageOperation(typ KeyPageOperationType) (KeyPageOperation, error) {
	switch typ {
	case KeyPageOperationTypeAdd:
		return new(AddKeyOperation), nil
	case KeyPageOperationTypeRemove:
		return new(RemoveKeyOperation), nil
	case KeyPageOperationTypeSetThreshold:
		return new(SetThresholdKeyPageOperation), nil
	case KeyPageOperationTypeUpdateAllowed:
		return new(UpdateAllowedKeyPageOperation), nil
	case KeyPageOperationTypeUpdate:
		return new(UpdateKeyOperation), nil
	default:
		return nil, fmt.Errorf("unknown key page operation %v", typ)
	}
}

// UnmarshalKeyPageOperationType unmarshals the KeyPageOperationType from the start of a KeyPageOperation.
func UnmarshalKeyPageOperationType(r io.Reader) (KeyPageOperationType, error) {
	var typ KeyPageOperationType
	err := encoding.UnmarshalEnumType(r, &typ)
	return typ, err
}

// UnmarshalKeyPageOperation unmarshals a KeyPageOperation.
func UnmarshalKeyPageOperation(data []byte) (KeyPageOperation, error) {
	typ, err := UnmarshalKeyPageOperationType(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	v, err := NewKeyPageOperation(typ)
	if err != nil {
		return nil, err
	}

	err = v.UnmarshalBinary(data)
	if err != nil {
		return nil, err
	}

	return v, nil
}

// UnmarshalKeyPageOperationFrom unmarshals a KeyPageOperation.
func UnmarshalKeyPageOperationFrom(rd io.ReadSeeker) (KeyPageOperation, error) {
	// Get the reader's current position
	pos, err := rd.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}

	// Read the type code
	typ, err := UnmarshalKeyPageOperationType(rd)
	if err != nil {
		return nil, err
	}

	// Reset the reader's position
	_, err = rd.Seek(pos, io.SeekStart)
	if err != nil {
		return nil, err
	}

	// Create a new transaction result
	v, err := NewKeyPageOperation(KeyPageOperationType(typ))
	if err != nil {
		return nil, err
	}

	// Unmarshal the result
	err = v.UnmarshalBinaryFrom(rd)
	if err != nil {
		return nil, err
	}

	return v, nil
}

// UnmarshalKeyPageOperationJson unmarshals a KeyPageOperation.
func UnmarshalKeyPageOperationJSON(data []byte) (KeyPageOperation, error) {
	var typ struct{ Type KeyPageOperationType }
	err := json.Unmarshal(data, &typ)
	if err != nil {
		return nil, err
	}

	acnt, err := NewKeyPageOperation(typ.Type)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, acnt)
	if err != nil {
		return nil, err
	}

	return acnt, nil
}

// NewSignature creates a new Signature for the specified SignatureType.
func NewSignature(typ SignatureType) (Signature, error) {
	switch typ {
	case SignatureTypeBTC:
		return new(BTCSignature), nil
	case SignatureTypeED25519:
		return new(ED25519Signature), nil
	case SignatureTypeETH:
		return new(ETHSignature), nil
	case SignatureTypeForwarded:
		return new(ForwardedSignature), nil
	case SignatureTypeInternal:
		return new(InternalSignature), nil
	case SignatureTypeLegacyED25519:
		return new(LegacyED25519Signature), nil
	case SignatureTypeRCD1:
		return new(RCD1Signature), nil
	case SignatureTypeReceipt:
		return new(ReceiptSignature), nil
	case SignatureTypeSynthetic:
		return new(SyntheticSignature), nil
	default:
		return nil, fmt.Errorf("unknown signature %v", typ)
	}
}

// UnmarshalSignatureType unmarshals the SignatureType from the start of a Signature.
func UnmarshalSignatureType(r io.Reader) (SignatureType, error) {
	var typ SignatureType
	err := encoding.UnmarshalEnumType(r, &typ)
	return typ, err
}

// UnmarshalSignature unmarshals a Signature.
func UnmarshalSignature(data []byte) (Signature, error) {
	typ, err := UnmarshalSignatureType(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	v, err := NewSignature(typ)
	if err != nil {
		return nil, err
	}

	err = v.UnmarshalBinary(data)
	if err != nil {
		return nil, err
	}

	return v, nil
}

// UnmarshalSignatureFrom unmarshals a Signature.
func UnmarshalSignatureFrom(rd io.ReadSeeker) (Signature, error) {
	// Get the reader's current position
	pos, err := rd.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}

	// Read the type code
	typ, err := UnmarshalSignatureType(rd)
	if err != nil {
		return nil, err
	}

	// Reset the reader's position
	_, err = rd.Seek(pos, io.SeekStart)
	if err != nil {
		return nil, err
	}

	// Create a new transaction result
	v, err := NewSignature(SignatureType(typ))
	if err != nil {
		return nil, err
	}

	// Unmarshal the result
	err = v.UnmarshalBinaryFrom(rd)
	if err != nil {
		return nil, err
	}

	return v, nil
}

// UnmarshalSignatureJson unmarshals a Signature.
func UnmarshalSignatureJSON(data []byte) (Signature, error) {
	var typ struct{ Type SignatureType }
	err := json.Unmarshal(data, &typ)
	if err != nil {
		return nil, err
	}

	acnt, err := NewSignature(typ.Type)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, acnt)
	if err != nil {
		return nil, err
	}

	return acnt, nil
}
