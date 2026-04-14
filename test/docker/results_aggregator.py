"""Aggregate test results and generate performance reports."""

import logging
from pathlib import Path
from typing import List, Dict
from dataclasses import dataclass
from datetime import datetime

logger = logging.getLogger(__name__)


@dataclass
class ConfigResult:
    """Results for a single test configuration."""
    test_id: str
    description: str
    validators: int
    bvns: int
    metrics_by_tps: Dict[int, Dict]  # tps -> {submitted, success, failed, error_rate, actual_tps, ...}
    max_sustained_tps: int
    pushback_detected_at: int = 0
    stable_range_tps: tuple = (0, 0)  # (min, max) where error_rate < 1%


class ResultsAggregator:
    """Aggregate and report test results."""

    def __init__(self, results_dir: Path):
        self.results_dir = results_dir
        self.config_results: List[ConfigResult] = []

    def add_config_result(self, result: ConfigResult):
        """Add results from a test configuration."""
        self.config_results.append(result)
        logger.info(f"Added results for {result.test_id}: {result.description}")

    def generate_csv(self, config_result: ConfigResult) -> Path:
        """Generate CSV file for a test configuration."""
        csv_file = self.results_dir / f"{config_result.test_id}-results.csv"

        lines = [
            "TPS_TARGET,SUBMITTED,SUCCESS,FAILED,ERROR_RATE_PCT,ACTUAL_TPS,P50_LATENCY_MS,P99_LATENCY_MS,PUSHBACK"
        ]

        for tps in sorted(config_result.metrics_by_tps.keys()):
            m = config_result.metrics_by_tps[tps]
            pushback = "TRUE" if tps == config_result.pushback_detected_at else "FALSE"
            lines.append(
                f"{tps},"
                f"{m.get('submitted', 0)},"
                f"{m.get('success', 0)},"
                f"{m.get('failed', 0)},"
                f"{m.get('error_rate', 0) * 100:.2f},"
                f"{m.get('actual_tps', 0):.1f},"
                f"{m.get('p50_latency', 'N/A')},"
                f"{m.get('p99_latency', 'N/A')},"
                f"{pushback}"
            )

        csv_content = "\n".join(lines)
        csv_file.write_text(csv_content)
        logger.info(f"Generated CSV: {csv_file}")
        return csv_file

    def generate_summary_report(self) -> Path:
        """Generate comprehensive summary report."""
        report_file = self.results_dir / "PERFORMANCE-RESULTS-RC-v1.5.1.md"

        lines = [
            "# Performance Test Results - RC v1.5.1-breaking (Issue #3905)",
            "",
            f"**Test Date**: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}",
            "**Methodology**: Incremental TPS testing (1K → 15K) with pushback detection (error > 5%)",
            "**Configurations**: 6 (3/4 validators × 1/2/3 BVNs)",
            "",
            "---",
            "",
            "## Executive Summary",
            "",
        ]

        # Calculate overall statistics
        if self.config_results:
            total_configs = len(self.config_results)
            avg_max_tps = sum(r.max_sustained_tps for r in self.config_results) / total_configs if total_configs else 0

            lines.append(f"- **Total Configurations Tested**: {total_configs}")
            lines.append(f"- **Average Per-BVN Throughput**: {avg_max_tps:.0f} TPS")
            lines.append(f"- **Test Duration**: ~{total_configs * 15} minutes")
            lines.append("")

        # Per-config summary table
        lines.append("## Per-Configuration Results")
        lines.append("")
        lines.append("| Test ID | Config | Max Sustained TPS | Per-BVN Limit | Total Network | Status |")
        lines.append("|---------|--------|------------------|---------------|----------------|--------|")

        for result in self.config_results:
            total_network_tps = result.max_sustained_tps * result.bvns
            status = "⚠ Pushback" if result.pushback_detected_at else "✓ No pushback"
            lines.append(
                f"| {result.test_id} | "
                f"{result.validators}v,{result.bvns}b | "
                f"{result.max_sustained_tps} TPS | "
                f"~{result.max_sustained_tps} TPS | "
                f"~{total_network_tps} TPS | "
                f"{status} |"
            )

        lines.append("")

        # Detailed per-config tables
        lines.append("## Detailed Results")
        lines.append("")

        for result in self.config_results:
            lines.append(f"### Test {result.test_id}: {result.description}")
            lines.append("")
            lines.append(f"**Configuration**: {result.validators} validators, {result.bvns} BVN(s)")
            lines.append(f"**Max Sustained TPS**: {result.max_sustained_tps}")
            lines.append(f"**Stable Range**: {result.stable_range_tps[0]}-{result.stable_range_tps[1]} TPS (<1% error)")
            if result.pushback_detected_at:
                lines.append(f"**Pushback Detected At**: {result.pushback_detected_at} TPS")
            lines.append("")

            # Metrics table
            lines.append("| TPS Target | Submitted | Success | Failed | Error % | Actual TPS | Status |")
            lines.append("|-----------|-----------|---------|--------|---------|-----------|--------|")

            for tps in sorted(result.metrics_by_tps.keys()):
                m = result.metrics_by_tps[tps]
                status_mark = "⚠ PUSHBACK" if tps == result.pushback_detected_at else "✓"
                lines.append(
                    f"| {tps} | "
                    f"{m.get('submitted', 0)} | "
                    f"{m.get('success', 0)} | "
                    f"{m.get('failed', 0)} | "
                    f"{m.get('error_rate', 0) * 100:.2f}% | "
                    f"{m.get('actual_tps', 0):.1f} | "
                    f"{status_mark} |"
                )

            lines.append("")

        # Analysis section
        lines.append("## Analysis")
        lines.append("")
        lines.append("### Per-BVN Scaling")
        lines.append("")

        # Find best single-BVN config
        single_bvn = [r for r in self.config_results if r.bvns == 1]
        if single_bvn:
            best = max(single_bvn, key=lambda r: r.max_sustained_tps)
            lines.append(f"- **Best single-BVN**: {best.test_id} ({best.validators}v) = {best.max_sustained_tps} TPS")

        # Calculate multi-BVN impact
        three_bvn = [r for r in self.config_results if r.bvns == 3]
        if three_bvn:
            lines.append(f"- **3-BVN configurations**:")
            for r in three_bvn:
                per_bvn = r.max_sustained_tps / r.bvns if r.bvns else 0
                lines.append(f"  - {r.test_id}: {r.max_sustained_tps} total ({per_bvn:.0f} per-BVN)")

        lines.append("")

        # RC Readiness
        lines.append("### RC v1.5.1-breaking Readiness")
        lines.append("")

        single_bvn_ready = any(r.max_sustained_tps >= 8000 for r in single_bvn) if single_bvn else False
        multi_bvn_ready = any(r.max_sustained_tps >= 9000 for r in three_bvn) if three_bvn else False

        if single_bvn_ready:
            lines.append("✓ **Single-BVN deployment ready**: ≥8K TPS sustained")
        else:
            lines.append("✗ **Single-BVN needs work**: <8K TPS")

        if multi_bvn_ready:
            lines.append("✓ **Multi-BVN deployment ready**: ≥3K TPS per-BVN")
        else:
            lines.append("✗ **Multi-BVN needs optimization**: <3K TPS per-BVN")

        lines.append("")
        lines.append("---")
        lines.append(f"Generated: {datetime.now().isoformat()}")

        # Write report
        report_content = "\n".join(lines)
        report_file.write_text(report_content)
        logger.info(f"Generated report: {report_file}")

        return report_file
