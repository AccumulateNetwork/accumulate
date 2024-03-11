// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulate

import (
	_ "embed"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
)

//go:embed checkpoint-mainnet.snap
var checkpointMainnetSnap []byte

var checkpointMainnetHash = [32]byte{
	0xdc, 0x62, 0x3c, 0x43, 0xce, 0x73, 0xde, 0xfb, 0xb9, 0x90, 0x99, 0xf8, 0xae, 0xea, 0x29, 0xe2, 0xfe, 0x7f, 0x35, 0xd7, 0xa4, 0xcd, 0x87, 0xf2, 0x54, 0xa8, 0x83, 0x58, 0xb7, 0xd8, 0xf0, 0x2f,
}

func MainNetCheckpoint() (ioutil.SectionReader, [32]byte) {
	return ioutil.NewBuffer(checkpointMainnetSnap), checkpointMainnetHash
}
