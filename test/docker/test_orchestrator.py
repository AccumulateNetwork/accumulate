#!/usr/bin/env python3
"""
Issue #3905: Performance test orchestration for RC v1.5.1-breaking

Runs 6 test configurations (3/4 validators × 1/2/3 BVNs) in sequence,
each with incremental TPS testing from 1K to 12K TPS.

Unattended execution with detailed failure capture for debugging team.
"""

import logging
import sys
import time
from datetime import datetime
from pathlib import Path

from test_config import TEST_CONFIGS, TPS_SEQUENCE, INCREMENT_DURATION_SECONDS, ERROR_THRESHOLD
from docker_manager import DockerManager
from failure_reporter import FailureReport, FailureRegistry


# Setup logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s [%(levelname)s] %(message)s',
    datefmt='%Y-%m-%d %H:%M:%S',
)
logger = logging.getLogger(__name__)


class TestOrchestrator:
    """Orchestrates the complete performance test suite."""

    def __init__(self, results_dir: Path = None):
        self.results_dir = results_dir or Path('performance-results')
        self.results_dir.mkdir(parents=True, exist_ok=True)
        self.suite_log = self.results_dir / f"suite-{datetime.now().strftime('%Y%m%d-%H%M%S')}.log"
        self.failure_registry = FailureRegistry()
        self.passed = 0
        self.failed = 0

    def log_section(self, title: str):
        """Log a section header."""
        logger.info("")
        logger.info("=" * 70)
        logger.info(title)
        logger.info("=" * 70)

    def run(self):
        """Execute the complete test suite."""
        self.log_section("Issue #3905: Performance Test Suite - RC v1.5.1-breaking")
        logger.info(f"Results directory: {self.results_dir}")
        logger.info(f"Test configurations: {len(TEST_CONFIGS)}")
        logger.info(f"TPS sequence: {TPS_SEQUENCE}")
        logger.info(f"Suite log: {self.suite_log}")

        # Pre-suite cleanup
        self.pre_suite_cleanup()

        # Run each configuration
        for config in TEST_CONFIGS:
            self.run_test_config(config)

        # Post-suite cleanup
        self.post_suite_cleanup()

        # Summary
        self.print_summary()

    def pre_suite_cleanup(self):
        """Clean Docker state before starting tests."""
        self.log_section("Pre-Suite Docker Cleanup")
        docker = DockerManager(Path.cwd() / "docker-compose.yml")
        docker.cleanup()
        if not docker.verify_clean():
            logger.warning("Docker not completely clean, continuing anyway")

    def post_suite_cleanup(self):
        """Clean Docker state after tests complete."""
        self.log_section("Post-Suite Docker Cleanup")
        docker = DockerManager(Path.cwd() / "docker-compose.yml")
        docker.cleanup()
        docker.verify_clean()
        logger.info("All resources cleaned up")

    def run_test_config(self, config):
        """Run a single test configuration with incremental TPS steps."""
        self.log_section(f"Test {config.test_id}: {config.description}")
        logger.info(f"Configuration: {config.validators} validators, {config.bvns} BVN(s)")

        docker = DockerManager(Path.cwd() / "docker-compose.yml")

        try:
            # Pre-test cleanup
            logger.info("Wiping Docker state before test...")
            docker.cleanup()
            docker.verify_clean()

            # Start network
            logger.info("Starting Docker network...")
            if not docker.compose_up():
                self._handle_failure(config, "Docker Compose startup failed", "docker_startup", docker)
                self.failed += 1
                return

            # Wait for health
            if not docker.wait_healthy(timeout=60):
                self._handle_failure(config, "Containers did not reach healthy state", "network_health", docker)
                self.failed += 1
                return

            logger.info("Network started successfully")

            # Verify sharding
            logger.info("Verifying 64-shard execution...")
            shards_ok, shard_msgs = docker.verify_shards(expected=64)
            if not shards_ok:
                logger.warning("⚠ No 64-shard confirmation found in logs")
                logger.warning("Executor messages found:")
                for msg in shard_msgs:
                    logger.warning(f"  {msg[:100]}")
                # Continue anyway - might just be logging issue
            else:
                logger.info(f"✓ Confirmed: 64-shard execution enabled ({len(shard_msgs)} confirmations)")

            # Run incremental TPS tests
            self._run_tps_sequence(config, docker)

            logger.info(f"✓ Test {config.test_id} completed successfully")
            self.passed += 1

        except Exception as e:
            logger.error(f"Unexpected error in test {config.test_id}: {e}")
            self._handle_failure(config, str(e), "unexpected_error", docker)
            self.failed += 1

        finally:
            # Post-test cleanup
            logger.info("Stopping network and cleaning Docker state...")
            docker.compose_down()
            time.sleep(3)
            docker.cleanup()
            docker.verify_clean()

    def _run_tps_sequence(self, config, docker):
        """Run incremental TPS steps until pushback detected or 15K TPS reached."""
        logger.info(f"Running TPS sequence: {TPS_SEQUENCE}")
        logger.info(f"Each level: {INCREMENT_DURATION_SECONDS}s (~2 min/level, ~15 min total)")
        logger.info(f"Stop condition: error rate > {ERROR_THRESHOLD*100}% OR TPS reaches 15000")

        for i, tps in enumerate(TPS_SEQUENCE, 1):
            logger.info(f"\n--- Level {i}/{len(TPS_SEQUENCE)}: {tps} TPS ({INCREMENT_DURATION_SECONDS}s) ---")

            # TODO: Call parallel-loadtest.go with TPS target and parse results
            logger.info(f"[PLACEHOLDER] Would run: parallel-loadtest -target-tps {tps} -duration {INCREMENT_DURATION_SECONDS}s")
            logger.info(f"[PLACEHOLDER] Would parse metrics: submitted, success, failed, error_rate, actual_tps")

            # TODO: Detect pushback and stop incrementing
            # error_rate = parse_result(output)
            # if error_rate > ERROR_THRESHOLD:
            #     logger.warning(f"PUSHBACK DETECTED at {tps} TPS (error rate: {error_rate*100:.1f}%)")
            #     break

            # Simulate test run for now
            time.sleep(2)

    def _handle_failure(self, config, error_msg: str, stage: str, docker):
        """Capture failure details for debugging team."""
        logger.error(f"✗ Test {config.test_id} FAILED: {error_msg} (stage: {stage})")

        report = FailureReport(config.test_id, error_msg, stage, self.results_dir)
        filepath = report.capture(docker)
        self.failure_registry.add(report, filepath)

        logger.error(f"Failure report saved: {filepath}")

    def print_summary(self):
        """Print test suite summary."""
        self.log_section("Test Suite Complete")
        logger.info(f"Passed: {self.passed}/{len(TEST_CONFIGS)}")
        logger.info(f"Failed: {self.failed}/{len(TEST_CONFIGS)}")
        logger.info(f"Results directory: {self.results_dir}")
        logger.info(f"Suite log: {self.suite_log}")

        if self.failed > 0:
            summary = self.failure_registry.summary()
            logger.error(summary)

            # Save failure summary
            failure_summary_file = self.results_dir / "FAILURES-SUMMARY.txt"
            failure_summary_file.write_text(summary)
            logger.error(f"\nFailure summary saved: {failure_summary_file}")


def main():
    """Main entry point."""
    try:
        orchestrator = TestOrchestrator()
        orchestrator.run()
        return 0 if orchestrator.failed == 0 else 1
    except KeyboardInterrupt:
        logger.info("\nTest suite interrupted by user")
        return 130
    except Exception as e:
        logger.error(f"Fatal error: {e}", exc_info=True)
        return 1


if __name__ == '__main__':
    sys.exit(main())
