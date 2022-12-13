// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package core holds core protocol types and constants
package core

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package core types.yml

// ExecutorV1 represents the executor system used at activation.
const ExecutorV1 = 0

// ExecutorV1V2Transition represents the transition from the original executor
// system to the v2 system.
const ExecutorV1V2Transition = 1

// ExecutorV2 represents the second version of the executor system.
const ExecutorV2 = 2
