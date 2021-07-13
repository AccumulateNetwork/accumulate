// MIT License
//
// Copyright 2018 Canonical Ledgers, LLC
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal in the Software without restriction, including without limitation the
// rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
// sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS
// IN THE SOFTWARE.

package fat107

import (
	"bytes"
	"crypto/sha256"
	"testing"

	"github.com/Factom-Asset-Tokens/factom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetadata(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	chainID := factom.NewBytes32(
		"1c9e1fd5603bd4cf54f8e186ebb8c9d68ae68ded0484fa6543e5915b72ce8990")
	dataHash := factom.NewBytes32(
		"1e3fcf78089dd840d132e4f30c1f56072e35cd33e06de879de6e8e859bb00d29")
	m, err := Lookup(nil, c, &chainID)
	require.NoError(err)

	assert.Equal(dataHash, *m.DataHash)
	assert.EqualValues(10250240, int(m.Size))
	if assert.NotNil(m.Compression) {
		assert.EqualValues(10253393, int(m.Compression.Size))
	}

	dataBuf := bytes.NewBuffer(make([]byte, 0, m.Size))
	require.NoError(m.Download(nil, c, dataBuf))

	hash := sha256.Sum256(dataBuf.Bytes())
	hash = sha256.Sum256(hash[:])

	assert.EqualValues(dataHash, hash)
}
