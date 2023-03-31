// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package logger

import (
	"fmt"
	"os"
	"sync"
)

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
