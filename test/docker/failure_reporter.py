"""Capture and report test failures for debugging team."""

import json
from datetime import datetime
from pathlib import Path
from typing import Optional
import subprocess


class FailureReport:
    """Structured failure report for investigation."""

    def __init__(self, test_id: str, error_msg: str, stage: str, results_dir: Path):
        self.test_id = test_id
        self.error_msg = error_msg
        self.stage = stage  # network_startup, docker_build, shard_verification, load_test, etc.
        self.timestamp = datetime.utcnow().isoformat() + 'Z'
        self.results_dir = results_dir

    def capture(self, docker_manager) -> str:
        """
        Capture detailed failure diagnostics and save to file.
        Returns the path to the failure report.
        """
        filename = f"FAILED-{self.test_id}-{datetime.now().strftime('%Y%m%d-%H%M%S')}.txt"
        filepath = self.results_dir / filename

        report_lines = [
            "=" * 70,
            "TEST FAILURE REPORT",
            "=" * 70,
            "",
            f"Test ID:       {self.test_id}",
            f"Timestamp:     {self.timestamp}",
            f"Stage:         {self.stage}",
            f"Error Message: {self.error_msg}",
            "",
            "=" * 70,
            "DOCKER STATE",
            "=" * 70,
            "",
        ]

        # Docker ps
        report_lines.append("=== docker ps -a ===")
        try:
            result = subprocess.run(["docker", "ps", "-a"], capture_output=True, text=True, timeout=10)
            report_lines.append(result.stdout)
        except Exception as e:
            report_lines.append(f"Error: {e}")

        # Docker volumes
        report_lines.append("\n=== docker volume ls ===")
        try:
            result = subprocess.run(["docker", "volume", "ls"], capture_output=True, text=True, timeout=10)
            report_lines.append(result.stdout)
        except Exception as e:
            report_lines.append(f"Error: {e}")

        # Docker networks
        report_lines.append("\n=== docker network ls ===")
        try:
            result = subprocess.run(["docker", "network", "ls"], capture_output=True, text=True, timeout=10)
            report_lines.append(result.stdout)
        except Exception as e:
            report_lines.append(f"Error: {e}")

        # Docker Compose ps
        report_lines.append("\n=== docker compose ps ===")
        try:
            ps_output = docker_manager.ps()
            report_lines.append(ps_output)
        except Exception as e:
            report_lines.append(f"Error: {e}")

        # Docker Compose logs
        report_lines.append("\n" + "=" * 70)
        report_lines.append("CONTAINER LOGS (last 300 lines)")
        report_lines.append("=" * 70)
        report_lines.append("")
        try:
            logs = docker_manager.get_logs(max_lines=300)
            report_lines.append(logs)
        except Exception as e:
            report_lines.append(f"Error getting logs: {e}")

        # System state
        report_lines.append("\n" + "=" * 70)
        report_lines.append("SYSTEM STATE")
        report_lines.append("=" * 70)
        report_lines.append("")

        try:
            result = subprocess.run(["df", "-h", "/"], capture_output=True, text=True, timeout=5)
            report_lines.append("=== Disk ===")
            report_lines.append(result.stdout)
        except Exception as e:
            report_lines.append(f"Disk check error: {e}")

        try:
            result = subprocess.run(["free", "-h"], capture_output=True, text=True, timeout=5)
            report_lines.append("\n=== Memory ===")
            report_lines.append(result.stdout)
        except Exception as e:
            report_lines.append(f"Memory check error: {e}")

        try:
            result = subprocess.run(["uptime"], capture_output=True, text=True, timeout=5)
            report_lines.append("\n=== Load ===")
            report_lines.append(result.stdout)
        except Exception as e:
            report_lines.append(f"Load check error: {e}")

        # Write to file
        report_text = '\n'.join(report_lines)
        filepath.write_text(report_text)

        return str(filepath)


class FailureRegistry:
    """Track all failures for summary reporting."""

    def __init__(self):
        self.failures = []

    def add(self, report: FailureReport, filepath: str):
        """Add a failure report to the registry."""
        self.failures.append({
            'test_id': report.test_id,
            'timestamp': report.timestamp,
            'stage': report.stage,
            'error_msg': report.error_msg,
            'report_file': filepath,
        })

    def to_json(self) -> str:
        """Export failures as JSON."""
        return json.dumps(self.failures, indent=2)

    def summary(self) -> str:
        """Generate summary of all failures."""
        if not self.failures:
            return "No failures detected"

        lines = [
            f"\n{'=' * 70}",
            "TEST FAILURES SUMMARY",
            f"{'=' * 70}",
            f"\nTotal Failures: {len(self.failures)}\n",
        ]

        by_stage = {}
        for f in self.failures:
            stage = f['stage']
            if stage not in by_stage:
                by_stage[stage] = []
            by_stage[stage].append(f)

        for stage in sorted(by_stage.keys()):
            lines.append(f"\n{stage.upper()} ({len(by_stage[stage])} failures):")
            for f in by_stage[stage]:
                lines.append(f"  - {f['test_id']}: {f['error_msg'][:60]}")
                lines.append(f"    Report: {f['report_file']}")

        lines.append("\n" + "=" * 70)
        lines.append("NEXT STEPS FOR DEBUGGING TEAM")
        lines.append("=" * 70)
        lines.append("""
1. Create team: teams create v1.5.1-rc-debugging

2. Review failure reports:
   - Categorize by failure type (network, sharding, consensus, etc.)
   - Assign to appropriate specialist

3. Specialists:
   - network-debugger: Docker startup, networking issues
   - sharding-debugger: Missing shard logs, executor config
   - load-test-debugger: Transaction submission failures
   - consensus-debugger: TPS plateaus, block production
   - memory-debugger: Memory runaway, goroutine leaks
   - cleanup-debugger: Docker cleanup failures

4. Follow debugging plan: .claude/plans/issue-3905-debugging-team-coordination.md
""")

        return '\n'.join(lines)
