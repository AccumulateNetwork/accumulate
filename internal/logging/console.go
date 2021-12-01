package logging

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/tendermint/tendermint/libs/log"
)

// NewConsoleWriter parses the log format and creates an appropriate writer.
// It is based on part of Tendermint's NewTendermintLogger.
func NewConsoleWriter(format string) (io.Writer, error) {
	switch strings.ToLower(format) {
	case log.LogFormatPlain, log.LogFormatText:
		return newConsoleWriter(os.Stderr), nil

	case log.LogFormatJSON:
		return os.Stderr, nil

	default:
		return nil, fmt.Errorf("unsupported log format: %s", format)
	}
}

// newConsoleWriter creates a zerolog console writer that formats log messages
// as plain text for the console. It is based on part of Tendermint's NewTendermintLogger.
func newConsoleWriter(w io.Writer) *zerolog.ConsoleWriter {
	return &zerolog.ConsoleWriter{
		Out: w,
		// NoColor:    true,
		TimeFormat: time.RFC3339,
		FormatLevel: func(i interface{}) string {
			if ll, ok := i.(string); ok {
				return strings.ToUpper(ll)
			}
			return "????"
		},
	}
}
