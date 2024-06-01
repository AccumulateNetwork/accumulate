// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulate

//go:generate go run github.com/vektra/mockery/v2
//go:generate go run github.com/rinchsan/gosimports/cmd/gosimports -w test/mocks pkg/api/v3/p2p/dial pkg/api/v3/message internal/api/routing

const unknownVersion = "version unknown"

var Version = unknownVersion
var Commit string

func IsVersionKnown() bool {
	return Version != unknownVersion
}
