// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build !ci
// +build !ci

package run

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestStartDelayAvoidsTheMinuteBoundary checks that a DevNet whose genesis
// would land in the last seconds before a scheduled major block is held back
// until the minute has turned (#4585, #4610).
func TestStartDelayAvoidsTheMinuteBoundary(t *testing.T) {
	at := func(sec int, ms int) time.Time {
		return time.Date(2026, 10, 26, 9, 24, sec, ms*int(time.Millisecond), time.UTC)
	}

	cases := map[string]struct {
		Now  time.Time
		Want time.Duration
	}{
		"Early in the minute":     {at(5, 0), 0},
		"Middle of the minute":    {at(30, 0), 0},
		"Just before the margin":  {at(49, 999), 0},
		"At the margin":           {at(50, 0), 10*time.Second + time.Second},
		"Last second":             {at(59, 200), 800*time.Millisecond + time.Second},
		"Just after the boundary": {at(0, 100), 0},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, c.Want, startDelay(c.Now))
		})
	}
}
