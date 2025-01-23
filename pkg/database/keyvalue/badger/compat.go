// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package badger

type Database = DatabaseV1

func New(filepath string, o ...Option) (*Database, error) {
	return OpenV1(filepath, o...)
}
