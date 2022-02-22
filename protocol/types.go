package protocol

import (
	"fmt"
	"reflect"

	"github.com/go-playground/validator/v10"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

// ChainType is the type of a chain belonging to an account.
type ChainType uint64

// ObjectType is the type of an object in the database.
type ObjectType uint64

// KeyPageOperation is the operation type of an UpdateKeyPage transaction.
type KeyPageOperation uint8

// TransactionType is the type for transaction types.
type TransactionType uint64

// TransactionMax defines the max point for transaction types.
type TransactionMax uint64

// AccountType is the type of an account.
type AccountType uint64

// IsUser returns true if the transaction type is user.
func (t TransactionType) IsUser() bool {
	return TransactionTypeUnknown < t && t.ID() <= TransactionMaxUser.ID()
}

// IsSynthetic returns true if the transaction type is synthetic.
func (t TransactionType) IsSynthetic() bool {
	return TransactionMaxUser.ID() < t.ID() && t.ID() <= TransactionMaxSynthetic.ID()
}

// IsInternal returns true if the transaction type is internal.
func (t TransactionType) IsInternal() bool {
	return TransactionMaxSynthetic.ID() < t.ID() && t.ID() <= TransactionMaxInternal.ID()
}

// IsTransaction returns true if the account type is a transaction.
func (t AccountType) IsTransaction() bool {
	switch t {
	case AccountTypeTransaction, AccountTypePendingTransaction:
		return true
	default:
		return false
	}
}

//go:generate go run ../tools/cmd/gen-types accounts.yml general.yml internal.yml query.yml transactions.yml
//go:generate go run ../tools/cmd/gen-enum --out enums_gen.go enums.yml errors.yml

///go:generate go run ../tools/cmd/gen-types general.yml -i AnchorMetadata,ChainMetadata

///intentionally disabled for now
///go:generate go run ../tools/cmd/gen-types accounts.yml general.yml internal.yml query.yml transactions.yml --out ../export/sdk/c --language c
///go:generate go run ../tools/cmd/gen-enum enums.yml --out ../export/sdk/c --language c

func NewValidator() (*validator.Validate, error) {
	v := validator.New()
	err := v.RegisterValidation("acc-url", func(fl validator.FieldLevel) bool {
		if fl.Field().Kind() != reflect.String {
			panic(fmt.Errorf("%q is not a string", fl.FieldName()))
		}

		s := fl.Field().String()
		if len(s) == 0 {
			// allow empty
			return true
		}

		_, err := url.Parse(s)
		return err == nil
	})
	return v, err
}
