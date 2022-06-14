//go:build !production
// +build !production

package storage

var debugKeys = false

func EnableKeyNameTracking() {
	debugKeys = true
}

func DisableKeyNameTracking() {
	debugKeys = false
}
