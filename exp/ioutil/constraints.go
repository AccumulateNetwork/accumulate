// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package ioutil

import "gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"

// enumGet is a type constraint for an enum type that is comparable and
// implements EnumValueGetter.
type enumGet interface {
	comparable
	String() string
	encoding.EnumValueGetter
}

// enumSet is a type constraint for an enum type that implements EnumValueSetter
// with a pointer receiver.
type enumSet[V any] interface {
	*V
	encoding.EnumValueSetter
}
