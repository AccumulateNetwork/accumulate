// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bcdb

import (
	stderrors "errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// TestAViewBegunDuringACommitSeesAWholeVersion — #4434 NEW-5. A commit made
// while no reader is open takes no pre-images, and a view that begins while
// that commit is being written through is at the previous version with no
// overlay to hold it there: it read whatever keys had landed. Run
// 20260924T111811Z: a joining node pulled <partition>/synthetic and every peer
// served a local delivery queue naming a message it answered NotFound for --
// the queue (dynamic) had landed and the message (permanent) had not.
//
// Here the view begins as the commit writes its first key. It must see the
// version before the commit or the version after it, never a mixture.
func TestAViewBegunDuringACommitSeesAWholeVersion(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "db"))
	require.NoError(t, err)
	defer func() { require.NoError(t, d.Close()) }()

	queue := record.NewKey("Account", "bvn-BVN1.acme/synthetic", "LocalDeliveryQueue")
	message := record.NewKey("Message", [32]byte{1}, "Main")
	put(t, d, queue, "empty")

	type seen struct {
		queue   string
		message string
	}
	read := make(chan seen, 1)
	var once bool
	d.putHook = func() {
		if once {
			return
		}
		once = true
		go func() {
			v := d.Begin(nil, false)
			defer v.Discard()
			var s seen
			q, err := v.Get(queue)
			if err != nil {
				t.Error(err)
			}
			s.queue = string(q)
			m, err := v.Get(message)
			switch {
			case err == nil:
				s.message = string(m)
			case isNotFound(err):
				s.message = "absent"
			default:
				t.Error(err)
			}
			read <- s
		}()
		// Give the view the chance to read inside the write-through. A view
		// that waits for the commit instead is not held up by this.
		select {
		case s := <-read:
			read <- s
		case <-time.After(200 * time.Millisecond):
		}
	}

	b := d.Begin(nil, true)
	require.NoError(t, b.Put(queue, []byte("queued")))
	require.NoError(t, b.Put(message, []byte("the message")))
	require.NoError(t, b.Commit())

	s := <-read
	t.Logf("the view read the queue %q and the message %q", s.queue, s.message)
	whole := s == seen{"empty", "absent"} || s == seen{"queued", "the message"}
	require.True(t, whole, "a view begun during a commit read part of it: the queue %q and the message %q", s.queue, s.message)
}

func isNotFound(err error) bool {
	var nf *database.NotFoundError
	return stderrors.As(err, &nf)
}
