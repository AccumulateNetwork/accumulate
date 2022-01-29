package protocol

import (
	"fmt"
	"reflect"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/go-playground/validator/v10"
)

// ChainType is the type of a chain belonging to an account.
type ChainType uint64

// ObjectType is the type of an object in the database.
type ObjectType uint64

// KeyPageOperation is the operation type of an UpdateKeyPage transaction.
type KeyPageOperation uint8

// TransactionType is the type for transaction types
type TransactionType uint64

// TransactionMax defines the max point for transaction types
type TransactionMax uint64

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

//go:generate go run ../tools/cmd/gentypes accounts.yml general.yml internal.yml query.yml transactions.yml
//go:generate go run ../tools/cmd/gentypes2 --out enums_gen.go enums.yml

///go:generate go run ../tools/cmd/gentypes accounts.yml general.yml internal.yml query.yml transactions.yml --out ../export/c --language c
///go:generate go run ../tools/cmd/gentypes2 --out enums_gen.go enums.yml --out ../export/c --language c

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
