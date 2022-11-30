// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package errors

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum --package errors --short-names status.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package errors error.yml

// Status is a request status code.
type Status uint64
