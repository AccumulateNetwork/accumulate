// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package logging

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	tmconfig "github.com/cometbft/cometbft/config"
	"github.com/rs/zerolog"
)

// NewConsoleWriter parses the log format and creates an appropriate writer.
// It is based on part of Tendermint's NewTendermintLogger.
func NewConsoleWriter(format string) (io.Writer, error) {
	return NewConsoleWriterWith(os.Stderr, format)
}

func NewConsoleWriterWith(w io.Writer, format string) (io.Writer, error) {
	switch strings.ToLower(format) {
	case tmconfig.LogFormatPlain:
		return newConsoleWriter(w), nil

	case tmconfig.LogFormatJSON:
		return w, nil

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
