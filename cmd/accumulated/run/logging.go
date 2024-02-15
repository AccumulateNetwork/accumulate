// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"io"
	"os"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"golang.org/x/exp/slog"
)

func (l *Logging) newHandler(out io.Writer) (slog.Handler, error) {
	setDefaultPtr(&l.Color, true)

	defaultLevel := slog.LevelWarn
	modules := map[string]slog.Level{}
	for _, r := range l.Rules {
		if len(r.Modules) == 0 {
			defaultLevel = r.Level
			continue
		}
		for _, m := range r.Modules {
			modules[strings.ToLower(m)] = r.Level
		}
	}

	return logging.NewSlogHandler(logging.SlogConfig{
		Format:       l.Format,
		NoColor:      !*l.Color,
		DefaultLevel: defaultLevel,
		ModuleLevels: modules,
	}, out)
}

func (l *Logging) start(inst *Instance) error {
	if l == nil {
		l = new(Logging)
	}

	h, err := l.newHandler(os.Stderr)
	if err != nil {
		return err
	}

	inst.logger = slog.New(h)
	slog.SetDefault(inst.logger)

	return nil
}
