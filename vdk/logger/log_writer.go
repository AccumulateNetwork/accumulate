// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package logger

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/vdk/utils"
)

type LogWriterConfig struct {
	LogFile     string
	JsonLogFile string
}

type LogAnnotator func(io.Writer, string, bool) io.Writer
type LogWriter func(string, LogAnnotator) (io.Writer, error)

func NewLogWriter(config LogWriterConfig) (LogWriter, error) {
	// Each log file writer must be created once, otherwise different copies
	// will step on each other
	var logFile *rotateWriter
	if config.LogFile != "" {
		logFile = &rotateWriter{file: config.LogFile}
		err := logFile.Open()
		if err != nil {
			return nil, err
		}
	}

	var jsonLogFile *rotateWriter
	if config.JsonLogFile != "" {
		jsonLogFile = &rotateWriter{file: config.JsonLogFile}
		err := jsonLogFile.Open()
		if err != nil {
			return nil, err
		}
	}

	// Rotate logs on SIGHUP
	utils.OnHUP(func() {
		if logFile != nil {
			logFile.Rotate()
		}
		if jsonLogFile != nil {
			jsonLogFile.Rotate()
		}
	})

	return func(format string, annotate LogAnnotator) (io.Writer, error) {
		var mainWriter io.Writer
		var err error
		mainWriter = os.Stderr
		if annotate != nil {
			mainWriter = annotate(mainWriter, format, true)
		}
		mainWriter, err = logging.NewConsoleWriterWith(mainWriter, format)
		if err != nil {
			return nil, err
		}

		var writers multiWriter
		writers = append(writers, mainWriter)

		if logFile != nil {
			w := io.Writer(logFile)
			if annotate != nil {
				w = annotate(w, string(NodeLogFormatPlain), false)
			}
			writers = append(writers, &zerolog.ConsoleWriter{
				Out:        w,
				NoColor:    true,
				TimeFormat: time.RFC3339,
				FormatLevel: func(i interface{}) string {
					if ll, ok := i.(string); ok {
						return strings.ToUpper(ll)
					}
					return "????"
				},
			})
		}

		if jsonLogFile != nil {
			w := io.Writer(jsonLogFile)
			if annotate != nil {
				w = annotate(w, string(NodeLogFormatJSON), false)
			}
			writers = append(writers, w)
		}

		if len(writers) == 1 {
			return writers[0], nil
		}
		return writers, nil
	}, nil
}
