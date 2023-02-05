// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

func TestDurationFromJSON(t *testing.T) {
	assert.Equal(t, time.Second+time.Nanosecond, mustDurationFromJSON(t, (time.Second+time.Nanosecond).Seconds()))
	assert.Equal(t, time.Second, mustDurationFromJSON(t, 1))
	assert.Equal(t, time.Second+time.Nanosecond, mustDurationFromJSON(t, (time.Second+time.Nanosecond).String()))
	assert.Equal(t, time.Second+time.Nanosecond, mustDurationFromJSON(t, DurationFields{Seconds: 1, Nanoseconds: 1}))
	assert.Equal(t, time.Second+time.Nanosecond, mustDurationFromJSON(t, map[string]interface{}{"seconds": 1, "nanoseconds": 1}))
}

func mustDurationFromJSON(t *testing.T, v interface{}) time.Duration {
	d, err := DurationFromJSON(v)
	require.NoError(t, err)
	return d
}

func TestSetPtrUnwrap(t *testing.T) {
	var target messaging.MessageWithTransaction
	value := new(messaging.SequencedMessage)
	value.Message = new(messaging.UserTransaction)

	require.NoError(t, SetPtr(value, &target))
	require.True(t, value.Message == target)
}
