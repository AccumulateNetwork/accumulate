// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package vdk

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kardianos/service"
	"github.com/rs/zerolog"
	"github.com/tendermint/tendermint/config"
	"gitlab.com/accumulatenetwork/accumulate/vdk/utils"
	"gitlab.com/accumulatenetwork/core/wallet/cmd/accumulate/logging"
)

type logAnnotator func(io.Writer, string, bool) io.Writer

var _ = NewLogWriter
var _ = Interrupt

func NewLogWriter(s service.Service, logFilename, jsonLogFilename string) func(string, logAnnotator) (io.Writer, error) {
	// Each log file writer must be created once, otherwise different copies
	// will step on each other
	var logFile *rotateWriter
	if logFilename != "" {
		logFile = &rotateWriter{file: logFilename}
		check(logFile.Open())
	}

	var jsonLogFile *rotateWriter
	if jsonLogFilename != "" {
		jsonLogFile = &rotateWriter{file: jsonLogFilename}
		check(jsonLogFile.Open())
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

	return func(format string, annotate logAnnotator) (io.Writer, error) {
		var mainWriter io.Writer
		var err error
		if !service.Interactive() && s != nil {
			mainWriter, err = logging.NewServiceLogger(s, format)
			check(err)
		} else {
			mainWriter = os.Stderr
			if annotate != nil {
				mainWriter = annotate(mainWriter, format, true)
			}
			mainWriter, err = logging.NewConsoleWriterWith(mainWriter, format)
			check(err)
		}

		var writers multiWriter
		writers = append(writers, mainWriter)

		if logFile != nil {
			w := io.Writer(logFile)
			if annotate != nil {
				w = annotate(w, config.LogFormatPlain, false)
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
				w = annotate(w, config.LogFormatJSON, false)
			}
			writers = append(writers, w)
		}

		if len(writers) == 1 {
			return writers[0], nil
		}
		return writers, nil
	}
}

func Interrupt(pid int) {
	utils.Interrupt(pid)
}

type multiWriter []io.Writer

func (w multiWriter) Write(b []byte) (int, error) {
	for _, w := range w {
		n, err := w.Write(b)
		if err != nil {
			return n, err
		}
		if n != len(b) {
			return n, io.ErrShortWrite
		}
	}
	return len(b), nil
}

func (w multiWriter) Close() error {
	var errs []error
	for _, w := range w {
		c, ok := w.(io.Closer)
		if !ok {
			continue
		}
		err := c.Close()
		if err != nil {
			errs = append(errs, err)
		}
	}
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return nil
	default:
		return errors.New(fmt.Sprint(errs))
	}
}

type rotateWriter struct {
	file string
	w    *os.File
	mu   sync.Mutex
}

func (w *rotateWriter) Open() error {
	if w.w != nil {
		return nil
	}

	var err error
	w.w, err = os.Create(w.file)
	return err
}

func (w *rotateWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	err := w.Open()
	if err != nil {
		return 0, err
	}

	return w.w.Write(b)
}

func (w *rotateWriter) Rotate() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.w == nil {
		return
	}

	err := w.w.Close()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "rotate writer failed to close file: %v\n", err)
	}

	w.w = nil
}

func fatalf(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

func check(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}
