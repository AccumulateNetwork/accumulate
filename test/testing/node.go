// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testing

import (
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const LogConsole = true

func DefaultSlogConfig() logging.SlogConfig {
	// Setup default slogging
	modules := map[string]slog.Level{}
	ll := (config.LogLevel{}).Parse(DefaultLogLevels)
	for _, m := range ll.Modules {
		var l slog.Level
		if err := l.UnmarshalText([]byte(m[1])); err != nil {
			panic(err)
		}
		modules[m[0]] = l
	}

	var def slog.Level
	if err := def.UnmarshalText([]byte(ll.Default)); err != nil {
		panic(err)
	}

	return logging.SlogConfig{
		DefaultLevel: def,
		ModuleLevels: modules,
	}
}

func SlogDebug(modules ...string) {
	c := DefaultSlogConfig()
	for _, m := range modules {
		c.ModuleLevels[m] = slog.LevelDebug
	}
	ConfigureSlog(c)
}

func ConfigureSlog(c logging.SlogConfig) {
	out := logging.ConsoleSlogWriter(os.Stderr, true)
	h, err := logging.NewSlogHandler(c, out)
	if err != nil {
		panic(err)
	}
	slog.SetDefault(slog.New(h))
}

func NewTestLogger(t testing.TB) logging.Logger {
	if !LogConsole {
		return logging.NewTestLogger(t, "plain", DefaultLogLevels, false)
	}

	var w io.Writer = &zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: time.RFC3339,
		// PartsExclude: []string{"time"},
		FormatLevel: func(i interface{}) string {
			if ll, ok := i.(string); ok {
				return strings.ToUpper(ll)
			}
			return "????"
		},
	}

	level, w, err := logging.ParseLogLevel(DefaultLogLevels, w)
	require.NoError(t, err)

	// w = logging.FilterWriter{
	// 	Out: w,
	// 	Predicate: func(level zerolog.Level, event map[string]interface{}) bool {
	// 		n, ok := event["node"].(float64)
	// 		return ok && n == 0
	// 	},
	// }

	logger, err := logging.NewZerologLogger(zerolog.New(w), level, false)
	require.NoError(t, err)
	return logger
}

var DefaultLogLevels = config.LogLevel{}.
	Parse(config.DefaultLogLevels).
	SetModule("restore", "error").
	// SetModule("accumulate", "debug").
	// SetModule("executor", "debug").
	// SetModule("consensus", "info").
	// SetModule("synthetic", "debug").
	// SetModule("anchoring", "debug").
	// SetModule("block", "debug").
	// SetModule("storage", "debug").
	// SetModule("database", "debug").
	// SetModule("fake-node", "debug").
	String()

func BvnIdForTest(t testing.TB) string {
	id := t.Name()
	id = strings.ReplaceAll(id, "/", "-")
	id = strings.ReplaceAll(id, "#", "-")
	return id
}

func BvnUrlForTest(t testing.TB) *url.URL {
	return protocol.PartitionUrl(BvnIdForTest(t))
}
