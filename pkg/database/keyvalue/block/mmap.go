// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"os"
	"sync/atomic"

	"github.com/edsrzf/mmap-go"
)

type mmapRef struct {
	buf    atomic.Pointer[mmapBuf]
	closed atomic.Bool
}

type mmapBuf struct {
	count atomic.Int32
	data  mmap.MMap
}

func (r *mmapRef) Map(f *os.File, length int) error {
	var err error
	new := new(mmapBuf)
	new.count.Store(1)
	new.data, err = mmap.MapRegion(f, length, mmap.RDWR, 0, 0)
	if err != nil {
		return err
	}

	old := r.buf.Swap(new)
	if old == nil {
		return nil
	}
	return old.release()
}

func (r *mmapRef) Acquire() *mmapBuf {
outer:
	for !r.closed.Load() {
		b := r.buf.Load()
		if b == nil {
			break
		}
		for {
			c := b.count.Load()
			if c == 0 {
				continue outer
			}
			if b.count.CompareAndSwap(c, c+1) {
				break
			}
		}

		return b
	}

	return new(mmapBuf)
}

func (r *mmapRef) Close() error {
	if r.closed.Swap(true) {
		return nil
	}

	b := r.buf.Swap(nil)
	if b == nil {
		return nil
	}
	return b.release()
}

func (b *mmapBuf) release() error {
	if b.count.Add(-1) != 0 {
		return nil
	}
	return b.data.Unmap()
}

func doRelease(ptr *error, b *mmapBuf) {
	err := b.release()
	if err != nil {
		*ptr = err
	}
}
