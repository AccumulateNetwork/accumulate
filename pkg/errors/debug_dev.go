// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build !production
// +build !production

package errors

var trackLocation = false

func EnableLocationTracking() {
	trackLocation = true
}

func DisableLocationTracking() {
	trackLocation = false
}
