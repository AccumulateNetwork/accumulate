package protocol

import (
	"bytes"
	"encoding"
	"encoding/json"
	"fmt"

	accenc "gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

type Account interface {
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
	GetType() AccountType
	Header() *AccountHeader
}

// ParseUrl returns the parsed chain URL
//
// Deprecated: use Url field
func (h *AccountHeader) ParseUrl() (*url.URL, error) {
	return h.Url, nil
}

func NewAccount(typ AccountType) (Account, error) {
	new(Anchor).Header()
	switch typ {
	case AccountTypeAnchor:
		return new(Anchor), nil
	case AccountTypeIdentity:
		return new(ADI), nil
	case AccountTypeTokenIssuer:
		return new(TokenIssuer), nil
	case AccountTypeTokenAccount:
		return new(TokenAccount), nil
	case AccountTypeLiteTokenAccount:
		return new(LiteTokenAccount), nil
	case AccountTypeTransaction:
		return new(TransactionState), nil
	case AccountTypePendingTransaction:
		return new(PendingTransactionState), nil
	case AccountTypeKeyPage:
		return new(KeyPage), nil
	case AccountTypeKeyBook:
		return new(KeyBook), nil
	case AccountTypeDataAccount:
		return new(DataAccount), nil
	case AccountTypeLiteDataAccount:
		return new(LiteDataAccount), nil
	case AccountTypeInternalLedger:
		return new(InternalLedger), nil
	case AccountTypeLiteIdentity:
		return new(LiteIdentity), nil
	default:
		return nil, fmt.Errorf("unknown account type %v", typ)
	}
}

func UnmarshalAccount(data []byte) (Account, error) {
	reader := accenc.NewReader(bytes.NewReader(data))
	typ, ok := reader.ReadUint(1)
	_, err := reader.Reset([]string{"Type"})
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("field Type: missing")
	}

	acnt, err := NewAccount(AccountType(typ))
	if err != nil {
		return nil, err
	}

	err = acnt.UnmarshalBinary(data)
	if err != nil {
		return nil, err
	}

	return acnt, nil
}

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
