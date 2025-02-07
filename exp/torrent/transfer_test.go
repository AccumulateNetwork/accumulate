// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package torrent

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"golang.org/x/sync/errgroup"
)

func TestTransfer(t *testing.T) {
	// Make a random 1 MiB 'file'
	const fileSize = 1 << 20
	file := make([]byte, fileSize)
	_, err := io.ReadFull(rand.Reader, file)
	require.NoError(t, err)

	// Build the metadata (32 KiB blocks)
	const chunkSize = 1 << 15
	md := new(FileMetadata)
	md.Chunks, _, err = ChunksBySize(bytes.NewBuffer(file), chunkSize)
	require.NoError(t, err)

	// Start a job
	job, err := NewDownloadJob(md)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, job.Reset()) })

	// Transfer chunks
	errg := new(errgroup.Group)
	for i := range md.Chunks {
		i := i
		start := i * chunkSize
		errg.Go(func() error {
			return job.RecordChunk(i, file[start:start+chunkSize])
		})
	}
	require.NoError(t, errg.Wait())

	// Concatenate the chunks
	require.True(t, job.Done())
	out := new(ioutil.Buffer)
	err = job.WriteTo(out)
	require.NoError(t, err)

	// Verify the input and output are equal
	h1 := sha256.Sum256(file)
	h2 := sha256.Sum256(out.Bytes())
	require.Equal(t, h1, h2)
}
