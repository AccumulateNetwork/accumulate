// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type NotFoundError Key

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%v not found", (*Key)(e))
}

func (e *NotFoundError) Is(target error) bool {
	return errors.Is(target, errors.NotFound)
}

func (e *NotFoundError) As(target any) bool {
	switch target := target.(type) {
	case **errors.Error:
		*target = errors.NotFound.With(e.Error())
	default:
		return false
	}
	return true
}
