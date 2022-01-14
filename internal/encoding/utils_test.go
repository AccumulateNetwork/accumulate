package encoding

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
