//go:build !production
// +build !production

package errors

var trackLocation = false

func EnableLocationTracking(v bool) {
	trackLocation = v
}
