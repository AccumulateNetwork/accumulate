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

class MetricsCollector:
    def __init__(self):
        self.lock = threading.Lock()
        self.metrics = {
            'tps': 0,
            'total_submitted': 0,
            'total_succeeded': 0,
            'total_failed': 0,
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
        self.start_time = time.time()
        self.last_submitted = 0
        self.last_time = time.time()

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
                    # Parse: Progress: submitted=123 success=120 failure=3 elapsed=30s actual_tps=4.0
                    match = re.search(r'submitted=(\d+)\s+success=(\d+)\s+failure=(\d+).*actual_tps=([\d.]+)', line)
                    if match:
                        with self.lock:
                            self.metrics['total_submitted'] = int(match.group(1))
                            self.metrics['total_succeeded'] = int(match.group(2))
                            self.metrics['total_failed'] = int(match.group(3))
                            self.metrics['tps'] = float(match.group(4))
                    break
        except Exception as e:
            print(f"Error reading loadtest log: {e}")

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
                        # Get latest timestamp
                        latest_time = rows[-1]['Timestamp']
                        latest_resource_rows = [r for r in rows if r['Timestamp'] == latest_time]

                        # Calculate totals
                        total_cpu = sum(float(r['CPU_Percent']) for r in latest_resource_rows)
                        total_mem = sum(float(r['Memory_MB']) for r in latest_resource_rows)
                        node_count = len(latest_resource_rows)

                        with self.lock:
                            self.metrics['cpu_cores'] = total_cpu / 100
                            self.metrics['avg_cpu_percent'] = total_cpu / node_count if node_count > 0 else 0
                            self.metrics['memory_gb'] = total_mem / 1024
                            self.metrics['avg_memory_percent'] = (total_mem / (node_count * 2048) * 100) if node_count > 0 else 0

            # Read per-node database (latest)
            database_file = self.monitoring_dir / 'per-node-database.csv'
            if database_file.exists():
                with open(database_file, 'r') as f:
                    reader = csv.DictReader(f)
                    rows = list(reader)

                    if rows:
                        # Get latest timestamp
                        latest_time = rows[-1]['Timestamp']
                        latest_db_rows = [r for r in rows if r['Timestamp'] == latest_time]

                        # Calculate total DB size
                        total_db = sum(float(r['DB_Size_MB']) for r in latest_db_rows)

                        # Calculate growth rate (compare first and last)
                        first_time = rows[0]['Timestamp']
                        first_rows = [r for r in rows if r['Timestamp'] == first_time]
                        initial_db = sum(float(r['DB_Size_MB']) for r in first_rows)

                        elapsed_min = (time.time() - self.start_time) / 60
                        growth_rate = (total_db - initial_db) / elapsed_min if elapsed_min > 0 else 0

                        with self.lock:
                            self.metrics['db_size_gb'] = total_db / 1024
                            self.metrics['db_growth_mb_per_min'] = growth_rate

                        # Update per-node metrics
                        nodes = []
                        for db_row in latest_db_rows:
                            # Find corresponding resource row
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
            dashboard_file = Path('/tmp/loadtest-workspace/dashboard.html')
            if dashboard_file.exists():
                self.send_response(200)
                self.send_header('Content-Type', 'text/html')
                self.end_headers()
                with open(dashboard_file, 'rb') as f:
                    self.wfile.write(f.read())
            else:
                self.send_error(404, 'Dashboard not found')

        else:
            self.send_error(404, 'Not found')

    def log_message(self, format, *args):
        # Suppress request logging
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

    loadtest_log = sys.argv[1] if len(sys.argv) > 1 else '/tmp/loadtest-workspace/12k-test.log'
    monitoring_dir = sys.argv[2] if len(sys.argv) > 2 else '/tmp/loadtest-workspace/12k-monitoring'
    port = int(sys.argv[3]) if len(sys.argv) > 3 else 8888

    collector = MetricsCollector()
    collector.set_paths(loadtest_log, monitoring_dir)

    print("Starting metrics server...")
    print(f"Load test log: {loadtest_log}")
    print(f"Monitoring directory: {monitoring_dir}")
    print()

    run_server(collector, port)
