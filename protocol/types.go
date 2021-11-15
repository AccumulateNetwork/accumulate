package protocol

import (
	"fmt"
	"reflect"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/go-playground/validator/v10"
)

//go:generate go run ../internal/cmd/gentypes types.yml

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
