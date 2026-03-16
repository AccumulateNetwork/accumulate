#!/usr/bin/env python3
"""
Consensus Testnet Monitor

Monitors Docker container logs for consensus testnet nodes.
Tracks submitted and committed transactions, reports aggregated stats.
Kills all nodes if system memory usage exceeds threshold.

Usage:
    python monitor.py [--memory-threshold 50] [--interval 5]
"""

import argparse
import re
import subprocess
import sys
import threading
import time
from collections import defaultdict
from dataclasses import dataclass, field
from typing import Dict


@dataclass
class NodeMetrics:
    """Metrics for a single node."""
    submitted: int = 0
    dropped: int = 0
    committed: int = 0
    blocks: int = 0
    last_update: float = field(default_factory=time.time)


class ConsensusMonitor:
    """Monitors consensus testnet Docker containers."""

    CONTAINER_PREFIX = "consensus-node"
    METRICS_PATTERN = re.compile(
        r"METRICS\s+submitted=(\d+)\s+dropped=(\d+)\s+committed=(\d+)\s+blocks=(\d+)"
    )
    BLOCK_PATTERN = re.compile(
        r"METRICS\s+block_height=(\d+)\s+block_txns=(\d+)\s+total_committed=(\d+)"
    )

    def __init__(self, memory_threshold: float = 50.0, report_interval: float = 5.0):
        self.memory_threshold = memory_threshold
        self.report_interval = report_interval
        self.nodes: Dict[str, NodeMetrics] = defaultdict(NodeMetrics)
        self.running = False
        self.lock = threading.Lock()
        self.log_threads: list = []

    def get_container_names(self) -> list:
        """Get list of running consensus node containers."""
        try:
            result = subprocess.run(
                ["docker", "ps", "--format", "{{.Names}}", "--filter", f"name={self.CONTAINER_PREFIX}"],
                capture_output=True,
                text=True,
                timeout=10
            )
            if result.returncode != 0:
                return []
            return [name.strip() for name in result.stdout.strip().split("\n") if name.strip()]
        except Exception as e:
            print(f"Error getting containers: {e}", file=sys.stderr)
            return []

    def get_memory_usage(self) -> float:
        """Get system memory usage percentage."""
        try:
            with open("/proc/meminfo", "r") as f:
                meminfo = {}
                for line in f:
                    parts = line.split()
                    if len(parts) >= 2:
                        key = parts[0].rstrip(":")
                        value = int(parts[1])
                        meminfo[key] = value

            total = meminfo.get("MemTotal", 1)
            available = meminfo.get("MemAvailable", meminfo.get("MemFree", 0))
            used_percent = ((total - available) / total) * 100
            return used_percent
        except Exception as e:
            print(f"Error reading memory: {e}", file=sys.stderr)
            return 0.0

    def kill_all_nodes(self):
        """Stop all consensus node containers."""
        print("\n*** MEMORY THRESHOLD EXCEEDED - KILLING ALL NODES ***")
        containers = self.get_container_names()
        for container in containers:
            try:
                subprocess.run(["docker", "kill", container], capture_output=True, timeout=30)
                print(f"Killed {container}")
            except Exception as e:
                print(f"Error killing {container}: {e}", file=sys.stderr)

    def follow_container_logs(self, container: str):
        """Follow logs from a container and parse metrics."""
        try:
            process = subprocess.Popen(
                ["docker", "logs", "-f", "--tail", "0", container],
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                text=True,
                bufsize=1
            )

            while self.running and process.poll() is None:
                line = process.stdout.readline()
                if not line:
                    continue

                # Parse metrics line
                match = self.METRICS_PATTERN.search(line)
                if match:
                    with self.lock:
                        self.nodes[container].submitted = int(match.group(1))
                        self.nodes[container].dropped = int(match.group(2))
                        self.nodes[container].committed = int(match.group(3))
                        self.nodes[container].blocks = int(match.group(4))
                        self.nodes[container].last_update = time.time()
                    continue

                # Parse block metrics
                match = self.BLOCK_PATTERN.search(line)
                if match:
                    with self.lock:
                        self.nodes[container].blocks = int(match.group(1))
                        self.nodes[container].committed = int(match.group(3))
                        self.nodes[container].last_update = time.time()

            process.terminate()
        except Exception as e:
            if self.running:
                print(f"Error following {container}: {e}", file=sys.stderr)

    def print_report(self):
        """Print aggregated metrics report."""
        with self.lock:
            if not self.nodes:
                print("No nodes reporting yet...")
                return

            total_submitted = 0
            total_dropped = 0
            total_committed = 0
            block_counts = []

            print("\n" + "=" * 70)
            print(f"{'Node':<20} {'Submitted':>12} {'Dropped':>10} {'Committed':>12} {'Blocks':>8}")
            print("-" * 70)

            for name in sorted(self.nodes.keys()):
                metrics = self.nodes[name]
                age = time.time() - metrics.last_update
                stale = " (stale)" if age > 30 else ""

                print(f"{name:<20} {metrics.submitted:>12,} {metrics.dropped:>10,} "
                      f"{metrics.committed:>12,} {metrics.blocks:>8}{stale}")

                total_submitted += metrics.submitted
                total_dropped += metrics.dropped
                total_committed += metrics.committed
                block_counts.append(metrics.blocks)

            print("-" * 70)
            print(f"{'TOTAL':<20} {total_submitted:>12,} {total_dropped:>10,} "
                  f"{total_committed:>12,}")

            # Check block consistency
            if block_counts:
                min_blocks = min(block_counts)
                max_blocks = max(block_counts)
                if min_blocks != max_blocks:
                    print(f"\n*** WARNING: Block count mismatch! Range: {min_blocks} - {max_blocks} ***")
                else:
                    print(f"\nAll nodes at block height: {max_blocks}")

            # Memory status
            mem_usage = self.get_memory_usage()
            print(f"Memory usage: {mem_usage:.1f}% (threshold: {self.memory_threshold}%)")
            print("=" * 70)

    def monitor_memory(self):
        """Monitor memory and kill nodes if threshold exceeded."""
        while self.running:
            mem_usage = self.get_memory_usage()
            if mem_usage >= self.memory_threshold:
                self.kill_all_nodes()
                self.running = False
                break
            time.sleep(1)

    def run(self):
        """Main monitoring loop."""
        self.running = True

        # Get initial containers
        containers = self.get_container_names()
        if not containers:
            print("No consensus nodes found. Make sure Docker containers are running.")
            print("Start with: docker compose up -d")
            return

        print(f"Monitoring {len(containers)} nodes: {', '.join(containers)}")
        print(f"Memory threshold: {self.memory_threshold}%")
        print(f"Report interval: {self.report_interval}s")
        print("Press Ctrl+C to stop\n")

        # Start log followers for each container
        for container in containers:
            thread = threading.Thread(target=self.follow_container_logs, args=(container,), daemon=True)
            thread.start()
            self.log_threads.append(thread)

        # Start memory monitor
        mem_thread = threading.Thread(target=self.monitor_memory, daemon=True)
        mem_thread.start()

        # Main reporting loop
        try:
            while self.running:
                self.print_report()
                time.sleep(self.report_interval)
        except KeyboardInterrupt:
            print("\nShutting down monitor...")
            self.running = False


def main():
    parser = argparse.ArgumentParser(description="Monitor consensus testnet Docker containers")
    parser.add_argument(
        "--memory-threshold", "-m",
        type=float,
        default=50.0,
        help="Memory usage percentage threshold to kill nodes (default: 50)"
    )
    parser.add_argument(
        "--interval", "-i",
        type=float,
        default=5.0,
        help="Report interval in seconds (default: 5)"
    )
    args = parser.parse_args()

    monitor = ConsensusMonitor(
        memory_threshold=args.memory_threshold,
        report_interval=args.interval
    )
    monitor.run()


if __name__ == "__main__":
    main()
