// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package nodestate

import (
	"encoding/json"
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

// JoiningState is what a joining node's refusal carries: the block of its own
// execution, when its store is that (no pull has started), so that a node
// deciding whether to resume after a whole restart knows whether a peer holds
// a higher state than it does (executor spec, "A partition that restarts as a
// whole resumes from its highest signed, validated state"; #4501).
type JoiningState struct {
	Executed *uint64 `json:"joiningExecuted"`
}

// RefuseJoiningAt is RefuseJoining stating the refusing node's own executed
// block.
func RefuseJoiningAt(partition string, executed uint64) error {
	e := errors.NotReady.WithFormat("%s %s", partition, joiningRefusal)
	v := executed
	e.Data, _ = json.Marshal(JoiningState{Executed: &v})
	return e
}

// JoiningExecuted finds the executed block a joining refusal states, the
// outermost first.
func JoiningExecuted(err error) (uint64, bool) {
	var e *errors.Error
	if !errors.As(err, &e) {
		return 0, false
	}
	for ; e != nil; e = e.Cause {
		if len(e.Data) == 0 {
			continue
		}
		d := new(JoiningState)
		if json.Unmarshal(e.Data, d) == nil && d.Executed != nil {
			return *d.Executed, true
		}
	}
	return 0, false
}
