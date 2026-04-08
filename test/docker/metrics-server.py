#!/usr/bin/env python3
"""
Metrics server for Accumulate load testing dashboard
Serves real-time metrics via HTTP and hosts the dashboard
"""

import json
import subprocess
import threading
import time
from http.server import HTTPServer, BaseHTTPRequestHandler
from pathlib import Path
import csv
import re
from collections import deque

class MetricsCollector:
    def __init__(self):
        self.lock = threading.Lock()
        self.metrics = {
            'tps_1min': 0,
            'tps_5min': 0,
            'tps_15min': 0,
            'tps_total': 0,
            'target_tps': 10,
            'total_submitted': 0,
            'total_succeeded': 0,
            'total_failed': 0,
            'active_accounts': 0,
            'cpu_cores': 0,
            'avg_cpu_percent': 0,
            'memory_gb': 0,
            'avg_memory_percent': 0,
            'db_size_gb': 0,
            'db_growth_mb_per_min': 0,
            'nodes': []
        }
        self.loadtest_log = None
        self.monitoring_dir = None
        self.start_time = None  # Will be set from first loadtest progress entry
        self.last_submitted = 0  # Track previous count to avoid duplicate samples
        self.last_progress_time = None  # Track timestamp of last progress update

        # Sample history for TPS calculations
        # Each sample is (timestamp, submitted_count)
        self.samples = deque(maxlen=900)  # 15 minutes at 1 sample/sec

    def set_paths(self, loadtest_log, monitoring_dir):
        self.loadtest_log = Path(loadtest_log)
        self.monitoring_dir = Path(monitoring_dir)

    def update_from_loadtest(self):
        """Read load test log for TPS and transaction counts"""
        if not self.loadtest_log or not self.loadtest_log.exists():
            return

        try:
            with open(self.loadtest_log, 'r') as f:
                lines = f.readlines()

            # Find latest progress line
            for line in reversed(lines):
                if 'Progress:' in line:
                    # Parse: Progress: submitted=123 success=120 failure=3 elapsed=30s tps_1min=100 tps_total=95 target=2000
                    match = re.search(r'submitted=(\d+)\s+success=(\d+)\s+failure=(\d+)', line)
                    if match:
                        submitted = int(match.group(1))
                        succeeded = int(match.group(2))
                        failed = int(match.group(3))

                        # Get target TPS
                        target_match = re.search(r'target=(\d+)', line)
                        target = int(target_match.group(1)) if target_match else 10

                        # Get active accounts if present
                        accounts_match = re.search(r'accounts=(\d+)', line)
                        accounts = int(accounts_match.group(1)) if accounts_match else 0

                        # Initialize start time from first progress update
                        if self.start_time is None:
                            self.start_time = time.time()

                        # Record sample only if submitted count changed (avoid duplicate samples)
                        if submitted != self.last_submitted:
                            now = time.time()
                            self.samples.append((now, submitted))
                            self.last_submitted = submitted

                        # Calculate TPS for different time windows
                        tps_1min = self._calc_tps(60)
                        tps_5min = self._calc_tps(300)
                        tps_15min = self._calc_tps(900)
                        elapsed = now - self.start_time
                        tps_total = submitted / elapsed if elapsed > 0 else 0

                        with self.lock:
                            self.metrics['total_submitted'] = submitted
                            self.metrics['total_succeeded'] = succeeded
                            self.metrics['total_failed'] = failed
                            self.metrics['target_tps'] = target
                            self.metrics['active_accounts'] = accounts
                            self.metrics['tps_1min'] = tps_1min
                            self.metrics['tps_5min'] = tps_5min
                            self.metrics['tps_15min'] = tps_15min
                            self.metrics['tps_total'] = tps_total
                    break
        except Exception as e:
            print(f"Error reading loadtest log: {e}")

    def _calc_tps(self, window_seconds):
        """Calculate TPS over the given time window"""
        if len(self.samples) < 2:
            return 0

        now = time.time()
        cutoff = now - window_seconds

        # Find oldest sample within window
        oldest_in_window = None
        for ts, count in self.samples:
            if ts >= cutoff:
                oldest_in_window = (ts, count)
                break

        if oldest_in_window is None:
            return 0

        newest = self.samples[-1]
        duration = newest[0] - oldest_in_window[0]
        if duration <= 0:
            return 0

        return (newest[1] - oldest_in_window[1]) / duration

    def update_from_monitoring(self):
        """Read monitoring CSVs for node metrics"""
        if not self.monitoring_dir:
            return

        latest_resource_rows = []

        try:
            # Read per-node resources (latest)
            resources_file = self.monitoring_dir / 'per-node-resources.csv'
            if resources_file.exists():
                with open(resources_file, 'r') as f:
                    reader = csv.DictReader(f)
                    rows = list(reader)

                    if rows:
                        latest_time = rows[-1]['Timestamp']
                        latest_resource_rows = [r for r in rows if r['Timestamp'] == latest_time]

                        total_cpu = sum(float(r['CPU_Percent']) for r in latest_resource_rows)
                        total_mem = sum(float(r['Memory_MB']) for r in latest_resource_rows)
                        node_count = len(latest_resource_rows)

                        with self.lock:
                            self.metrics['cpu_cores'] = total_cpu / 100
                            self.metrics['avg_cpu_percent'] = total_cpu / node_count if node_count > 0 else 0
                            self.metrics['memory_gb'] = total_mem / 1024
                            # Updated for 1GB limit per container
                            self.metrics['avg_memory_percent'] = (total_mem / (node_count * 1024) * 100) if node_count > 0 else 0

            # Read per-node database (latest)
            database_file = self.monitoring_dir / 'per-node-database.csv'
            if database_file.exists():
                with open(database_file, 'r') as f:
                    reader = csv.DictReader(f)
                    rows = list(reader)

                    if rows:
                        latest_time = rows[-1]['Timestamp']
                        latest_db_rows = [r for r in rows if r['Timestamp'] == latest_time]

                        total_db = sum(float(r['DB_Size_MB']) for r in latest_db_rows)

                        first_time = rows[0]['Timestamp']
                        first_rows = [r for r in rows if r['Timestamp'] == first_time]
                        initial_db = sum(float(r['DB_Size_MB']) for r in first_rows)

                        elapsed_min = (time.time() - self.start_time) / 60
                        growth_rate = (total_db - initial_db) / elapsed_min if elapsed_min > 0 else 0

                        with self.lock:
                            self.metrics['db_size_gb'] = total_db / 1024
                            self.metrics['db_growth_mb_per_min'] = growth_rate

                        nodes = []
                        for db_row in latest_db_rows:
                            resource_row = next((r for r in latest_resource_rows if r['Node'] == db_row['Node']), None)
                            if resource_row:
                                nodes.append({
                                    'name': db_row['Node'],
                                    'cpu': float(resource_row.get('CPU_Percent', 0)),
                                    'memory_mb': float(resource_row.get('Memory_MB', 0)),
                                    'db_size_mb': float(db_row['DB_Size_MB'])
                                })

                        with self.lock:
                            self.metrics['nodes'] = nodes

        except Exception as e:
            print(f"Error reading monitoring files: {e}")

    def get_metrics(self):
        """Get current metrics as JSON"""
        self.update_from_loadtest()
        self.update_from_monitoring()

        with self.lock:
            return dict(self.metrics)

class MetricsHandler(BaseHTTPRequestHandler):
    collector = None

    def do_GET(self):
        if self.path == '/metrics':
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.send_header('Access-Control-Allow-Origin', '*')
            self.end_headers()

            metrics = self.collector.get_metrics()
            self.wfile.write(json.dumps(metrics).encode())

        elif self.path == '/' or self.path == '/dashboard':
            # Try local dashboard first, then workspace
            dashboard_paths = [
                Path(__file__).parent / 'dashboard.html',
                Path('/tmp/loadtest-workspace/dashboard.html')
            ]

            for dashboard_file in dashboard_paths:
                if dashboard_file.exists():
                    self.send_response(200)
                    self.send_header('Content-Type', 'text/html')
                    self.end_headers()
                    with open(dashboard_file, 'rb') as f:
                        self.wfile.write(f.read())
                    return

            self.send_error(404, 'Dashboard not found')

        else:
            self.send_error(404, 'Not found')

    def log_message(self, format, *args):
        pass

def run_server(collector, port=8888):
    MetricsHandler.collector = collector
    server = HTTPServer(('0.0.0.0', port), MetricsHandler)
    print(f"Metrics server running on http://0.0.0.0:{port}")
    print(f"Dashboard: http://localhost:{port}/")
    print(f"Metrics API: http://localhost:{port}/metrics")
    server.serve_forever()

if __name__ == '__main__':
    import sys

    loadtest_log = sys.argv[1] if len(sys.argv) > 1 else '/tmp/loadtest-workspace/loadtest.log'
    monitoring_dir = sys.argv[2] if len(sys.argv) > 2 else '/tmp/loadtest-workspace/monitoring'
    port = int(sys.argv[3]) if len(sys.argv) > 3 else 8888

    collector = MetricsCollector()
    collector.set_paths(loadtest_log, monitoring_dir)

    print("Starting metrics server...")
    print(f"Load test log: {loadtest_log}")
    print(f"Monitoring directory: {monitoring_dir}")
    print()

    run_server(collector, port)
