// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

func (WillBeginBlock) IsEvent()  {}
func (WillCommitBlock) IsEvent() {}

type WillBeginBlock struct {
	BlockParams
}

type WillCommitBlock struct {
	Block BlockState
}
