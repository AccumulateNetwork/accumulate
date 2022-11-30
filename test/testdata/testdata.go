// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testdata

import _ "embed"

//go:embed test_factom_addresses
var FactomAddresses string

//go:embed merkle.yaml
var Merkle string
