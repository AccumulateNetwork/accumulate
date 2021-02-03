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

// +build ignore

package main

import (
	"log"
	"os"
	. "text/template"
)

var idKeys = []struct {
	ID       int
	IDPrefix string
	SKPrefix string
	IDStr    string
	SKStr    string
}{{
	ID:       1,
	IDPrefix: "0x3f, 0xbe, 0xba",
	SKPrefix: "0x4d, 0xb6, 0xc9",
	IDStr:    "id12K4tCXKcJJYxJmZ1UY9EuKPvtGVAjo32xySMKNUahbmRcsqFgW",
	SKStr:    "sk13iLKJfxNQg8vpSmjacEgEQAnXkn7rbjd5ewexc1Un5wVPa7KTk",
}, {
	ID:       2,
	IDPrefix: "0x3f, 0xbe, 0xd8",
	SKPrefix: "0x4d, 0xb6, 0xe7",
	IDStr:    "id22pNvsaMWf9qxWFrmfQpwFJiKQoWfKmBwVgQtdvqVZuqzGmrFNY",
	SKStr:    "sk22UaDys2Mzg2pUCsToo9aKgxubJFnZN5Bc2LXfV59VxMvXXKwXa",
}, {
	ID:       3,
	IDPrefix: "0x3f, 0xbe, 0xf6",
	SKPrefix: "0x4d, 0xb7, 0x05",
	IDStr:    "id33pRgpm8ufXNGxtW7n5FgdGP6afXKjU4LfVmgfC8Yaq6LyYq2wA",
	SKStr:    "sk32Xyo9kmjtNqRUfRd3ZhU56NZd8M1nR61tdBaCLSQRdhUCk4yiM",
}, {
	ID:       4,
	IDPrefix: "0x3f, 0xbf, 0x14",
	SKPrefix: "0x4d, 0xb7, 0x23",
	IDStr:    "id42vYqBB63eoSz8DHozEwtCaLbEwvBTG9pWgD3D5CCaHWy1gCjF5",
	SKStr:    "sk43eMusQuvvChoGNn1VZZwbAH8BtKJSZNC7ZWoz1Vc4Y3greLA45",
}}

func main() {
	idKeyGoTmplt := Must(ParseFiles("./idkey.tmpl"))
	idKeyTestGoTmplt := Must(ParseFiles("./idkey_test.tmpl"))

	idKeyGoFile, err := os.OpenFile("./idkey_gen.go",
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer idKeyGoFile.Close()

	idKeyTestGoFile, err := os.OpenFile("./idkey_gen_test.go",
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer idKeyTestGoFile.Close()

	err = idKeyGoTmplt.Execute(idKeyGoFile, idKeys)
	if err != nil {
		log.Fatal(err)
	}

	err = idKeyTestGoTmplt.Execute(idKeyTestGoFile, idKeys)
	if err != nil {
		log.Fatal(err)
	}
}
