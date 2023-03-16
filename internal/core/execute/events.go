// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

func (WillCommitBlock) IsEvent() {}

type WillCommitBlock struct {
	Block BlockState
}
