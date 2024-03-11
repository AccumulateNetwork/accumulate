// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package typegen

import (
	"bytes"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
)

func WriteFile(file string, buf *bytes.Buffer) error {
	switch filepath.Ext(file) {
	case ".go":
		return GoFmt(file, buf)
	default:
		f, err := os.Create(file)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = buf.WriteTo(f)
		return err
	}
}

func GoFmt(filePath string, buf *bytes.Buffer) error {
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, buf, parser.ParseComments)
	if err != nil {
		// If parsing fails, write out the unformatted code. Without this,
		// debugging the generator is a pain.
		_, _ = buf.WriteTo(f)
	}
	if err != nil {
		return err
	}

	err = format.Node(f, fset, file)
	if err != nil {
		return err
	}

	err = exec.Command("go", "run", "github.com/rinchsan/gosimports/cmd/gosimports", "-w", filePath).Run()
	if err != nil {
		return err
	}

	return nil
}
