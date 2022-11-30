// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"fmt"
	"reflect"

	"github.com/go-playground/validator/v10"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/managed"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// ChainType is the type of a chain belonging to an account.
type ChainType = managed.ChainType

const ChainTypeUnknown = managed.ChainTypeUnknown
const ChainTypeTransaction = managed.ChainTypeTransaction
const ChainTypeAnchor = managed.ChainTypeAnchor
const ChainTypeIndex = managed.ChainTypeIndex

// BookType is the type of a key book.
type BookType uint64

// ObjectType is the type of an object in the database.
type ObjectType uint64

// KeyPageOperationType is the operation type of an UpdateKeyPage operation.
type KeyPageOperationType uint8

// AccountAuthOperationType is the operation type of an UpdateAccountAuth operation.
type AccountAuthOperationType uint8

type ErrorCode int

type PartitionType int

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types account_auth_operations.yml accounts.yml general.yml system.yml key_page_operations.yml query.yml signatures.yml synthetic_transactions.yml transaction.yml transaction_results.yml user_transactions.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum --out enums_gen.go enums.yml errors.yml

///intentionally disabled for now
///go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --out ../export/sdk/c --language c account_auth_operations.yml accounts.yml general.yml system.yml key_page_operations.yml query.yml signatures.yml synthetic_transactions.yml transaction.yml transaction_results.yml user_transactions.yml
///go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum enums.yml --out ../export/sdk/c --language c

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
