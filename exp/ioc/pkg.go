// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package ioc provides Inversion of Control tools.
package ioc

import "reflect"

type Descriptor interface {
	Type() reflect.Type
	Namespace() string
}
