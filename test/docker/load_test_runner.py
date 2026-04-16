"""Run parallel-loadtest.go and parse metrics output."""

import logging
import subprocess
import re
from typing import Dict, Optional, Tuple
from pathlib import Path

logger = logging.getLogger(__name__)


class LoadTestMetrics:
    """Metrics from a single load test run."""

    def __init__(self):
        self.tps_target: int = 0
        self.submitted: int = 0
        self.success: int = 0
        self.failed: int = 0
        self.error_rate: float = 0.0
        self.actual_tps: float = 0.0
        self.p50_latency: Optional[float] = None
        self.p99_latency: Optional[float] = None

    def __repr__(self):
        return f"LoadTestMetrics(target={self.tps_target}tps, actual={self.actual_tps:.0f}tps, error={self.error_rate*100:.2f}%, submitted={self.submitted}, success={self.success}, failed={self.failed})"


class LoadTestRunner:
    """Runs load tests via parallel-loadtest.go."""

    def __init__(self, docker_dir: Path):
        self.docker_dir = docker_dir

    def run(self, target_tps: int, duration_seconds: int) -> Tuple[bool, Optional[LoadTestMetrics]]:
        """
        Run a load test at the given TPS target for the specified duration.

        Returns (success, metrics).
        """
        logger.info(f"Running load test: {target_tps} TPS for {duration_seconds}s")

        try:
            # Build command - parallel-loadtest uses adaptive ramping:
            # -start-tps: starting TPS
            # -min-tps: minimum TPS (ramp down to this first)
            # -max-tps: maximum TPS (ramp up limit)
            # For fixed-rate testing, set start=min=max=target to hold steady
            cmd = [
                "go", "run", "parallel-loadtest.go",
                "-start-tps", str(target_tps),
                "-min-tps", str(target_tps),
                "-max-tps", str(target_tps),
                "-ramp-down-duration", "0s",  # No ramp-down for fixed rate
                "-ramp-up-duration", "0s",    # No ramp-up for fixed rate
                "-error-threshold", "0.05",   # 5% error threshold for pushback
                "-duration", f"{duration_seconds}s",
            ]

            result = subprocess.run(
                cmd,
                cwd=self.docker_dir,
                capture_output=True,
                text=True,
                timeout=duration_seconds + 60,  # Add buffer for startup/shutdown
            )

            if result.returncode != 0:
                logger.error(f"Load test failed with exit code {result.returncode}")
                logger.error(f"stderr: {result.stderr}")
                return False, None

            # Parse output
            metrics = self._parse_output(target_tps, result.stdout)
            if metrics:
                logger.info(f"✓ {metrics}")
                return True, metrics
            else:
                logger.error("Failed to parse load test output")
                return False, None

        except subprocess.TimeoutExpired:
            logger.error(f"Load test timed out after {duration_seconds}s")
            return False, None
        except Exception as e:
            logger.error(f"Load test execution failed: {e}")
            return False, None

    def _parse_output(self, target_tps: int, output: str) -> Optional[LoadTestMetrics]:
        """Parse metrics from load test output."""
        metrics = LoadTestMetrics()
        metrics.tps_target = target_tps

        # Parse key metrics from output
        # Expected output format (from parallel-loadtest.go):
        # "Submitted: 12345"
        # "Success: 12300"
        # "Failed: 45"
        # "Average TPS: 1234.56"
        # "Error rate: 0.36%"

        try:
            # Extract submitted
            match = re.search(r"Submitted:\s*(\d+)", output)
            if match:
                metrics.submitted = int(match.group(1))

            # Extract success
            match = re.search(r"Success:\s*(\d+)", output)
            if match:
                metrics.success = int(match.group(1))

            # Extract failed
            match = re.search(r"Failed:\s*(\d+)", output)
            if match:
                metrics.failed = int(match.group(1))

            # Calculate error rate if we have the data
            if metrics.submitted > 0:
                metrics.error_rate = metrics.failed / metrics.submitted
            else:
                return None

            # Extract actual TPS
            match = re.search(r"Average TPS:\s*([\d.]+)", output)
            if match:
                metrics.actual_tps = float(match.group(1))
            else:
                # If no average TPS, calculate from submitted/duration
                # For now, estimate from target
                metrics.actual_tps = target_tps * (1.0 - metrics.error_rate)

            # Extract P50 latency (optional)
            match = re.search(r"P50.*?:\s*([\d.]+)\s*ms", output)
            if match:
                metrics.p50_latency = float(match.group(1))

            # Extract P99 latency (optional)
            match = re.search(r"P99.*?:\s*([\d.]+)\s*ms", output)
            if match:
                metrics.p99_latency = float(match.group(1))

            return metrics

        except Exception as e:
            logger.error(f"Error parsing load test output: {e}")
            return None
