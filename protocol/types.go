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

//go:generate go run ../tools/cmd/gen-types accounts.yml general.yml internal.yml query.yml transactions.yml
//go:generate go run ../tools/cmd/gen-enum --out enums_gen.go enums.yml errors.yml

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
