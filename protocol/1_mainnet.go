// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build !testnet
// +build !testnet

package protocol

const IsTestNet = false

// InitialAcmeOracle is $0.50, the oracle value at activation.
const InitialAcmeOracle = 0.50
