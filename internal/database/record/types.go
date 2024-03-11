// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package record

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
)

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-model --package record_test --out model_gen_test.go model_test.yml

type Key = database.Key
type Record = database.Record
type WalkOptions = database.WalkOptions
type WalkFunc = database.WalkFunc
type TerminalRecord = database.Value
type Store = database.Store
type ValueReader = database.Value
type ValueWriter = database.Value

func NewKey(v ...any) *Key { return database.NewKey(v...) }
