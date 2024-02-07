// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package logging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/rs/zerolog"
)

func ParseLogLevel(s string, w io.Writer) (string, io.Writer, error) {
	if !strings.Contains(s, "=") {
		return s, w, nil
	}

	var lowestLevel = zerolog.Disabled
	var defaultLevel = zerolog.Disabled
	levels := map[string]zerolog.Level{}
	modules := strings.FieldsFunc(s, func(r rune) bool { return r == ';' })
	for _, module := range modules {
		parts := strings.Split(module, "=")
		level, err := zerolog.ParseLevel(parts[len(parts)-1])
		if err != nil {
			return "", nil, err
		}

		if level < lowestLevel {
			lowestLevel = level
		}

		if len(parts) == 1 || parts[0] == "*" {
			defaultLevel = level
		} else {
			levels[parts[0]] = level
		}
	}

	w = FilterWriter{
		Out: w,
		Predicate: func(level zerolog.Level, event map[string]interface{}) bool {
			m, _ := event["module"].(string)

			// This is a hack to hide annoying cache messages
			if m == "mempool" {
				e, _ := event["err"].(string)
				if e == "tx already exists in cache" {
					return false
				}
			}

			l, ok := levels[m]
			if !ok {
				l = defaultLevel
			}

			return level >= l
		},
	}

	return lowestLevel.String(), w, nil
}

type FilterWriter struct {
	Out       io.Writer
	Predicate func(zerolog.Level, map[string]interface{}) bool
}

var _ zerolog.LevelWriter = FilterWriter{}

func (w FilterWriter) Write(p []byte) (n int, err error) {
	return w.WriteLevel(zerolog.NoLevel, p)
}

func (w FilterWriter) WriteLevel(level zerolog.Level, p []byte) (n int, err error) {
	var evt map[string]interface{}
	// WARNING If zerolog is compiled with binary_log, this will not work
	d := json.NewDecoder(bytes.NewReader(p))
	err = d.Decode(&evt)
	if err != nil {
		return 0, fmt.Errorf("cannot decode event: %s", err)
	}

	if level == zerolog.NoLevel {
		s, ok := evt[zerolog.LevelFieldName].(string)
		if ok {
			level, _ = zerolog.ParseLevel(s)
		}
	}

	n = len(p)
	if p := w.Predicate; p != nil && !p(level, evt) {
		return n, nil
	}

	return w.Out.Write(p)
}
