// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build !production
// +build !production

package memory

var debugLogWrites = false

func EnableLogWrites() {
	debugLogWrites = true
}

func DisableLogWrites() {
	debugLogWrites = false
}
