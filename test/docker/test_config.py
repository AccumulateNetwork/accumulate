"""Test configuration for Issue #3905 performance validation."""

from dataclasses import dataclass
from typing import List


@dataclass
class TestConfig:
    """Single test configuration (validators, BVNs)."""
    validators: int
    bvns: int
    test_id: str
    description: str

    def __str__(self):
        return f"{self.test_id}: {self.description} ({self.validators} validators, {self.bvns} BVN(s))"


# All 6 test configurations
TEST_CONFIGS: List[TestConfig] = [
    TestConfig(3, 1, "A1", "Single-BVN-3-Validators"),
    TestConfig(4, 1, "A2", "Single-BVN-4-Validators"),
    TestConfig(3, 2, "B1", "Dual-BVN-3-Validators"),
    TestConfig(4, 2, "B2", "Dual-BVN-4-Validators"),
    TestConfig(3, 3, "C1", "Triple-BVN-3-Validators"),
    TestConfig(4, 3, "C2", "Triple-BVN-4-Validators"),
]

# TPS sequence: start low, increment until pushback detected
TPS_SEQUENCE: List[int] = [1000, 2000, 3000, 5000, 7000, 10000, 12000]

# Test parameters
INCREMENT_DURATION_SECONDS = 60
ERROR_THRESHOLD = 0.05  # 5% error rate triggers pushback
