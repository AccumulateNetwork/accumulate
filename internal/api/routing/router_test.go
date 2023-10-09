// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package routing

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func TestRouteSyntheticMessage(t *testing.T) {
	t.Run("New", func(t *testing.T) {
		router := NewMockRouter(t)
		inner := &messaging.SequencedMessage{Destination: url.MustParse("foo")}
		synth := &messaging.SyntheticMessage{Message: inner}
		router.EXPECT().RouteAccount(inner.Destination).Return("bar", nil)
		_, err := RouteMessages(router, []messaging.Message{synth})
		require.NoError(t, err)
	})

	t.Run("Old", func(t *testing.T) {
		router := NewMockRouter(t)
		inner := &messaging.SequencedMessage{Destination: url.MustParse("foo")}
		synth := &messaging.BadSyntheticMessage{Message: inner}
		router.EXPECT().RouteAccount(inner.Destination).Return("bar", nil)
		_, err := RouteMessages(router, []messaging.Message{synth})
		require.NoError(t, err)
	})
}
