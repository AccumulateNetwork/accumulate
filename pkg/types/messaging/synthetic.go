// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package messaging

func (s *BadSyntheticMessage) Data() *SynthFields {
	d := new(SynthFields)
	d.Message = s.Message
	d.Signature = s.Signature
	d.Proof = s.Proof
	return d
}

func (s *SyntheticMessage) Data() *SynthFields {
	d := new(SynthFields)
	d.Message = s.Message
	d.Signature = s.Signature
	d.Proof = s.Proof
	return d
}
