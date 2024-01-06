// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum --package bpt enums.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package bpt types.yml --union-skip-type
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package bpt --language go-union --out unions_gen.go types.yml --union-skip-new
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-model --package bpt model.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-model --package bpt --out model_gen_test.go model_test.yml
