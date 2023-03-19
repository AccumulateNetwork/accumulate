// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package core

import "gitlab.com/accumulatenetwork/accumulate/pkg/types/network"

type GlobalValues = network.GlobalValues
type ValidatorUpdate = network.ValidatorUpdate

const ValidatorUpdateAdd = network.ValidatorUpdateAdd
const ValidatorUpdateRemove = network.ValidatorUpdateRemove

func NewGlobals(g *GlobalValues) *GlobalValues { return network.NewGlobals(g) }
