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

package fat

import (
	"fmt"
	"strconv"
)

type Type uint

const (
	TypeFAT0 Type = iota
	TypeFAT1
)

func (t *Type) Set(s string) error {
	return t.UnmarshalText([]byte(s))
}

const format = `FAT-`

func (t *Type) UnmarshalText(text []byte) error {
	if string(text[0:len(format)]) != format {
		return fmt.Errorf("%T: invalid format", t)
	}
	i, err := strconv.ParseUint(string(text[len(format):]), 10, 64)
	if err != nil {
		return fmt.Errorf("%T: %w", t, err)
	}
	*t = Type(i)
	return nil
}

func (t Type) MarshalText() ([]byte, error) {
	return []byte(fmt.Sprintf("%v%v", format, uint(t))), nil
}

func (t Type) String() string {
	text, err := t.MarshalText()
	if err != nil {
		return err.Error()
	}
	return string(text)
}

func (t Type) IsValid() bool {
	switch t {
	case TypeFAT0:
		fallthrough
	case TypeFAT1:
		return true
	}
	return false
}
