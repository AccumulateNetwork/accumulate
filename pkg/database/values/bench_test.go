// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package values_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func BenchmarkNotFound(b *testing.B) {
	db := database.OpenInMemory(nil)
	foo := url.MustParse("foo")

	var err error
	for i := 0; i < b.N; i++ {
		batch := db.Begin(false)
		_, err = batch.Account(foo).Main().Get()
	}
	require.ErrorIs(b, err, errors.NotFound)
}
