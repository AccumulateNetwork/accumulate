// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package errors

import "gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum --package errors --short-names status.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package errors --skip-generic-as error.yml

type Error = ErrorBase[Status]

// Status is a request status code.
type Status uint64

type statusType interface {
	~int | ~int16 | ~int32 | ~int64 | ~uint | ~uint16 | ~uint32 | ~uint64
	comparable
	error
	String() string
	IsKnownError() bool
	encoding.EnumValueGetter
}
