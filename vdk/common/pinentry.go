package vdk

import (
	"errors"
	"fmt"
	"github.com/twpayne/go-pinentry"
	"io"
	"os"
	"runtime/debug"
	"strings"

	"github.com/fatih/color"
	"github.com/howeyc/gopass"
)

var PinEntryMode string

func init() {
	// The pinentry library is used to read secret data. Therefore it must be
	// audited. If the pinentry library is updated without updating this block
	// of code, a warning will be printed out. This makes it harder to
	// accidentally update the library without auditing it.

	// I wanted to use log.Fatal, but build info is missing from `go test`
	// builds and I couldn't find a reliable way to detect tests.

	info, ok := debug.ReadBuildInfo()
	if !ok {
		color.Red("WARNING: cannot verify pinentry: binary does not include build info")
		return
	}
	ok = false
	for _, dep := range info.Deps {
		if dep.Path == "github.com/twpayne/go-pinentry" {
			switch dep.Version {
			case "v0.2.0":
				// Audited
			default:
				// Unknown
				color.Red("WARNING: cannot verify pinentry: %s@%s has not been audited", dep.Path, dep.Version)
				return
			}
			ok = true
		}
	}
	if !ok {
		color.Red("WARNING: cannot verify pinentry: could not locate dependency info")
		return
	}
}

var ErrPasswdCanceled = errors.New("password request was canceled")

func GetPassword(long, short string, rd gopass.FdReader, errw io.Writer, mask bool) ([]byte, error) {
	opts := []pinentry.ClientOption{
		pinentry.WithTitle("Accumulate CLI Wallet"),
		pinentry.WithDesc(long),
		pinentry.WithPrompt(""),
		pinentry.WithGPGTTY(),
	}

	switch strings.ToLower(PinEntryMode) {
	case "", "enable", "enabled":
		opts = append(opts, pinentry.WithBinaryNameFromGnuPGAgentConf())
	case "none", "disable", "disabled", "loopback":
		b, err := gopass.GetPasswdPrompt(short, true, rd, errw)
		return b, err
		// loopback
	default:
		opts = append(opts, pinentry.WithBinaryName(PinEntryMode))
	}

	c, err := pinentry.NewClient(opts...)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	pin, fromCache, err := c.GetPIN()
	if err != nil {
		if pinentry.IsCancelled(err) {
			return nil, ErrPasswdCanceled
		}
		return nil, err
	}
	if fromCache {
		fmt.Fprintf(os.Stderr, "Got password from cache\n")
	}

	return []byte(pin), nil
}
