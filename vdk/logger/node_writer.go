// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package logger

import (
	"fmt"
	"io"
	"strings"

	"github.com/fatih/color"
)

type NodeLogFormat string

const (
	NodeLogFormatJSON  = NodeLogFormat("json")
	NodeLogFormatPlain = NodeLogFormat("plain")
)

var colors = []*color.Color{
	color.New(color.FgRed),
	color.New(color.FgGreen),
	color.New(color.FgYellow),
	color.New(color.FgBlue),
	color.New(color.FgMagenta),
	color.New(color.FgCyan),
}
var fallbackColor = color.New(color.FgHiBlack)

var partitionColor = map[string]*color.Color{}

type NodeWriterConfig struct {
	Format          NodeLogFormat
	PartitionName   string
	NodeIndex       int
	NodeName        string
	NodeNamePadding int
	Colorize        bool
}

func NewNodeWriter(w io.Writer, config NodeWriterConfig) io.Writer {
	switch config.Format {
	case NodeLogFormatPlain:
		id := fmt.Sprintf("%s.%d", config.PartitionName, config.NodeIndex)
		s := fmt.Sprintf("[%s]", id) + strings.Repeat(" ", config.NodeNamePadding+len("bvnxx")-len(config.NodeName)+1)
		if !config.Colorize {
			return &plainNodeWriter{s, w}
		}

		c, ok := partitionColor[config.PartitionName]
		if !ok {
			c = fallbackColor
			if len(colors) > 0 {
				c = colors[0]
				colors = colors[1:]
			}
			partitionColor[config.PartitionName] = c
		}

		s = c.Sprint(s)
		return &plainNodeWriter{s, w}

	case NodeLogFormatJSON:
		s := fmt.Sprintf(`"partition":"%s","node":%d`, config.PartitionName, config.NodeIndex)
		return &jsonNodeWriter{s, w}

	default:
		return w
	}
}

type plainNodeWriter struct {
	s string
	w io.Writer
}

func (w *plainNodeWriter) Write(b []byte) (int, error) {
	c := make([]byte, len(w.s)+len(b))
	n := copy(c, []byte(w.s))
	copy(c[n:], b)

	n, err := w.w.Write(c)
	if n >= len(w.s) {
		n -= len(w.s)
	}
	return n, err
}

type jsonNodeWriter struct {
	s string
	w io.Writer
}

func (w *jsonNodeWriter) Write(b []byte) (int, error) {
	if b[0] != '{' {
		return w.w.Write(b)
	}

	c := make([]byte, 0, len(w.s)+len(b)+1)
	c = append(c, '{')
	c = append(c, []byte(w.s)...)
	c = append(c, ',')
	c = append(c, b[1:]...)

	n, err := w.w.Write(c)
	if n >= len(w.s)+1 {
		n -= len(w.s) + 1
	}
	return n, err
}
