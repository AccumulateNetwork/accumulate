// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIpOffset(t *testing.T) {
	require.Equal(t, "/ip4/127.0.0.2", listen(nil, "/ip4/127.0.0.1", ipOffset(1)).String())
	require.Equal(t, "/ip4/127.0.1.6", listen(nil, "/ip4/127.0.0.250", ipOffset(10)).String())
	require.Equal(t, "/ip4/127.0.2.12", listen(nil, "/ip4/127.0.0.250", ipOffset(270)).String())
}
