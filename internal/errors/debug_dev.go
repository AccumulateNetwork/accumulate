//go:build !production
// +build !production

package errors

var trackLocation = false

func EnableLocationTracking() {
	trackLocation = true
}
