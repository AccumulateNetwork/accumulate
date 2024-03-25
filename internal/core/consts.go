// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package core

import (
	"regexp"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
)

const SnapshotMajorFormat = "snapshot-major-block-%09d.bpt"

var SnapshotMajorRegexp = regexp.MustCompile(`snapshot-major-block-\d+.bpt`)

var Cron = network.CronFormat
