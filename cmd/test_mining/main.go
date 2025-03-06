// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/mining"
)

func main() {
	fmt.Println("Testing LXR Mining Integration")
	fmt.Println("=============================")
	
	// Run the integration tests
	mining.RunIntegrationTests()
}
