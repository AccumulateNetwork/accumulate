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

//go:generate go run ../tools/cmd/gentypes accounts.yml general.yml internal.yml query.yml transactions.yml
//go:generate go run ../tools/cmd/gentypes2 --out chains_gen.go chains.yml

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
