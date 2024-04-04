// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package harness

import (
	"fmt"

	"github.com/stretchr/testify/assert"
)

// Fails waits until the messages has been delivered and succeeds if the
// messages failed (and fails otherwise).
func (c msgCond) Fails() *failCond {
	d := new(failCond)
	d.Condition = c.make("fails", deliveredThen(func(h *Harness, c *condition, r *msgResult) bool {
		h.TB.Helper()

		// Must be failure
		if !r.Status.Failed() {
			h.TB.Fatal(c.messageReplaceEnd("did not fail 🗴\n"))
		}

		if d.withError != nil && !assert.ErrorIs(h.TB, r.Status.Error, *d.withError) {
			h.TB.Fatal(c.messageReplaceEnd("failed with wrong error 🗴\n"))
		}

		if d.withMessage != nil && !assert.EqualError(h.TB, r.Status.Error, *d.withMessage) {
			h.TB.Fatal(c.messageReplaceEnd("failed with wrong message 🗴\n"))
		}
		return true
	}))
	return d
}

type failCond struct {
	withError   *error
	withMessage *string
	Condition
}

func (c *failCond) WithError(target error) *failCond {
	c.withError = &target
	return c
}

func (c *failCond) WithMessage(message string) *failCond {
	c.withMessage = &message
	return c
}

func (c *failCond) WithMessagef(format string, args ...any) *failCond {
	message := fmt.Sprintf(format, args...)
	c.withMessage = &message
	return c
}
