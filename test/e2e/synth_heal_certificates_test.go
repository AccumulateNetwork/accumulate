// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
)

// TestSyntheticHealingUnderCertificateConsensus runs the healing tests with
// their bodies unchanged, in a simulator whose blocks are executed through
// ExecutorBridge.ProduceBlock in certificate batches, as DAG-BFT executes
// them, and not through the lockstep Process loop (#4142, part b).
func TestSyntheticHealingUnderCertificateConsensus(t *testing.T) {
	t.Setenv(CertificateConsensusEnv, "1")

	t.Run("TestSyntheticHealing", TestSyntheticHealing)
	t.Run("TestSyntheticHealingSignatureRequest", TestSyntheticHealingSignatureRequest)
	t.Run("TestSyntheticHealingLostPrefix", TestSyntheticHealingLostPrefix)
	t.Run("TestReconcileDoesNotRaceNormalDelivery", TestReconcileDoesNotRaceNormalDelivery)
}
