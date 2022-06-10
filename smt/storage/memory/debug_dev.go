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
