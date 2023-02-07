// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package abci implements the Accumulate ABCI applications.
//
// # Transaction Processing
//
// Tendermint processes transactions in the following phases:
//
//   - BeginBlock
//   - [CheckTx]
//   - [DeliverTx]
//   - EndBlock
//   - Commit
package abci

// Version is the version of the ABCI applications.
const Version uint64 = 0x1
