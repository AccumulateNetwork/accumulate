// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build !production
// +build !production

package record

var debugKeys = false

func EnableKeyNameTracking() {
	debugKeys = true
}

func DisableKeyNameTracking() {
	debugKeys = false
}
