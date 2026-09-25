// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package nodestate

import (
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// joiningRefusal is what a BOOTING node's querier says when it refuses a read.
// The phrase, and not the NotReady code alone, is what marks the refusal: an
// ACTIVE node out of query capacity answers NotReady too, and it is no
// evidence that its partition restarted (#4447).
const joiningRefusal = "is joining and cannot answer for state it has not executed"

// RefuseJoining is the refusal a BOOTING node answers every read with.
func RefuseJoining(partition string) error {
	return errors.NotReady.WithFormat("%s %s", partition, joiningRefusal)
}

// IsJoiningRefusal reports whether err is a peer saying it is joining: it was
// reached and answered, and has no state of its own to serve.
func IsJoiningRefusal(err error) bool {
	return err != nil && errors.Is(err, errors.NotReady) && strings.Contains(err.Error(), joiningRefusal)
}
