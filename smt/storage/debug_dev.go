//go:build !production
// +build !production

package storage

var debugKeys = false

func EnableKeyNameTracking(v bool) {
	debugKeys = v
}
