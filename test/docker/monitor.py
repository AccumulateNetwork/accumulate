#!/usr/bin/env python3
"""
Comprehensive monitoring for Accumulate load testing
Captures per-node metrics: CPU, memory, disk, database size, network I/O
"""

import subprocess
import time
import csv
import os
import sys
from datetime import datetime
from pathlib import Path

NODES = [
    "acc-bvn1-val1", "acc-bvn1-val2", "acc-bvn1-val3", "acc-bvn1-val4",
    "acc-bvn2-val1", "acc-bvn2-val2", "acc-bvn2-val3", "acc-bvn2-val4",
    "acc-bvn3-val1", "acc-bvn3-val2", "acc-bvn3-val3", "acc-bvn3-val4",
]

def parse_size(size_str):
    """Convert size string (e.g., '1.5GiB', '512MiB') to MB"""
    if not size_str or size_str == '0B':
        return 0.0

    size_str = size_str.strip()

    # Extract number and unit
    import re
    match = re.match(r'([\d.]+)([A-Za-z]+)', size_str)
    if not match:
        return 0.0

    value = float(match.group(1))
    unit = match.group(2).upper()

    # Convert to MB
    conversions = {
        'B': 1 / (1024 * 1024),
        'KB': 1 / 1024,
        'KIB': 1 / 1024,
        'MB': 1,
        'MIB': 1,
        'GB': 1024,
        'GIB': 1024,
        'TB': 1024 * 1024,
        'TIB': 1024 * 1024,
    }

    return value * conversions.get(unit, 1)

def get_container_stats(node):
    """Get Docker stats for a node"""
    try:
        cmd = [
            'docker', 'stats', node, '--no-stream',
            '--format', '{{.CPUPerc}},{{.MemUsage}},{{.NetIO}},{{.BlockIO}}'
        ]
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=5)
        if result.returncode != 0:
            return None

        parts = result.stdout.strip().split(',')
        if len(parts) != 4:
            return None

        cpu_pct = float(parts[0].rstrip('%'))

        # Parse memory (format: "150MiB / 2GiB")
        mem_parts = parts[1].split('/')
        mem_used_mb = parse_size(mem_parts[0].strip())
        mem_limit_mb = parse_size(mem_parts[1].strip()) if len(mem_parts) > 1 else 2048

        # Parse network I/O (format: "1.5MB / 2.3MB")
        net_parts = parts[2].split('/')
        net_in_mb = parse_size(net_parts[0].strip())
        net_out_mb = parse_size(net_parts[1].strip()) if len(net_parts) > 1 else 0

        # Parse block I/O (format: "10MB / 50MB")
        block_parts = parts[3].split('/')
        disk_read_mb = parse_size(block_parts[0].strip())
        disk_write_mb = parse_size(block_parts[1].strip()) if len(block_parts) > 1 else 0

        return {
            'cpu_percent': cpu_pct,
            'memory_mb': mem_used_mb,
            'memory_limit_mb': mem_limit_mb,
            'memory_percent': (mem_used_mb / mem_limit_mb * 100) if mem_limit_mb > 0 else 0,
            'disk_read_mb': disk_read_mb,
            'disk_write_mb': disk_write_mb,
            'net_in_mb': net_in_mb,
            'net_out_mb': net_out_mb,
        }
    except Exception as e:
        print(f"Error getting stats for {node}: {e}", file=sys.stderr)
        return None

def get_database_stats(node):
    """Get database size from container"""
    try:
        # Get total size of .accumulate directory (in KB for precision)
        cmd = [
            'docker', 'exec', node, 'sh', '-c',
            'du -sk /root/.accumulate 2>/dev/null | awk "{print \\$1}" || echo 0'
        ]
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=5)
        if result.returncode != 0:
            return {'db_size_mb': 0, 'db_files': 0}

        db_size_kb = int(result.stdout.strip() or '0')
        db_size_mb = db_size_kb / 1024.0  # Convert KB to MB

        # Count files
        cmd_files = [
            'docker', 'exec', node, 'sh', '-c',
            'find /root/.accumulate/*/data -type f 2>/dev/null | wc -l || echo 0'
        ]
        result_files = subprocess.run(cmd_files, capture_output=True, text=True, timeout=5)
        db_files = int(result_files.stdout.strip() or '0')

        return {
            'db_size_mb': db_size_mb,
            'db_files': db_files,
        }
    except Exception as e:
        print(f"Error getting DB stats for {node}: {e}", file=sys.stderr)
        return {'db_size_mb': 0, 'db_files': 0}

def monitor(output_dir, duration=300, interval=10):
    """Run monitoring for specified duration"""
    output_dir = Path(output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)

    print("Comprehensive Monitoring Started")
    print("=" * 50)
    print(f"Output: {output_dir}")
    print(f"Duration: {duration}s")
    print(f"Interval: {interval}s")
    print()

    # Open CSV files
    resources_file = output_dir / 'per-node-resources.csv'
    database_file = output_dir / 'per-node-database.csv'
    summary_file = output_dir / 'cluster-summary.csv'

    with open(resources_file, 'w', newline='') as rf, \
         open(database_file, 'w', newline='') as df, \
         open(summary_file, 'w', newline='') as sf:

        resources_writer = csv.writer(rf)
        database_writer = csv.writer(df)
        summary_writer = csv.writer(sf)

        # Write headers
        resources_writer.writerow([
            'Timestamp', 'Node', 'CPU_Percent', 'Memory_MB', 'Memory_Limit_MB',
            'Memory_Percent', 'Disk_Read_MB', 'Disk_Write_MB', 'Net_In_MB', 'Net_Out_MB'
        ])
        database_writer.writerow(['Timestamp', 'Node', 'DB_Size_MB', 'DB_Files'])
        summary_writer.writerow([
            'Timestamp', 'Total_CPU_Cores', 'Total_Memory_GB', 'Total_DB_Size_GB',
            'Avg_CPU_Percent', 'Avg_Memory_Percent'
        ])

        start_time = time.time()
        iteration = 0

        while True:
            elapsed = int(time.time() - start_time)
            if elapsed >= duration:
                print("\nMonitoring duration reached, stopping...")
                break

            timestamp = datetime.now().strftime('%Y-%m-%d %H:%M:%S')

            # Collect per-node metrics
            total_cpu = 0
            total_memory_mb = 0
            total_db_size_mb = 0
            node_count = 0

            for node in NODES:
                stats = get_container_stats(node)
                if not stats:
                    continue

                db_stats = get_database_stats(node)

                node_count += 1
                total_cpu += stats['cpu_percent']
                total_memory_mb += stats['memory_mb']
                total_db_size_mb += db_stats['db_size_mb']

                # Write per-node resources
                resources_writer.writerow([
                    timestamp, node, stats['cpu_percent'], stats['memory_mb'],
                    stats['memory_limit_mb'], stats['memory_percent'],
                    stats['disk_read_mb'], stats['disk_write_mb'],
                    stats['net_in_mb'], stats['net_out_mb']
                ])

                # Write per-node database
                database_writer.writerow([
                    timestamp, node, db_stats['db_size_mb'], db_stats['db_files']
                ])

            # Calculate and write cluster summary
            if node_count > 0:
                avg_cpu = total_cpu / node_count
                total_cpu_cores = total_cpu / 100
                total_memory_gb = total_memory_mb / 1024
                total_db_size_gb = total_db_size_mb / 1024
                avg_memory_pct = (total_memory_mb / (node_count * 2048)) * 100

                summary_writer.writerow([
                    timestamp, f'{total_cpu_cores:.2f}', f'{total_memory_gb:.2f}',
                    f'{total_db_size_gb:.2f}', f'{avg_cpu:.1f}', f'{avg_memory_pct:.1f}'
                ])

                # Display progress
                print(f"[{elapsed:4d}s] Nodes: {node_count:2d} | "
                      f"CPU: {total_cpu_cores:5.1f} cores ({avg_cpu:5.1f}% avg) | "
                      f"Memory: {total_memory_gb:6.2f} GB ({avg_memory_pct:5.1f}% avg) | "
                      f"DB: {total_db_size_gb:6.2f} GB")

            # Flush files
            rf.flush()
            df.flush()
            sf.flush()

            iteration += 1
            time.sleep(interval)

    print(f"\nMonitoring complete! Results saved to {output_dir}")
    generate_report(output_dir, duration, interval, iteration)

def generate_report(output_dir, duration, interval, iterations):
    """Generate summary report"""
    print("\nGenerating summary report...")

    report_file = output_dir / 'summary-report.txt'

    with open(report_file, 'w') as f:
        f.write("Accumulate Load Test - Monitoring Summary\n")
        f.write("=" * 50 + "\n")
        f.write(f"Duration: {duration}s\n")
        f.write(f"Samples: {iterations} (every {interval}s)\n\n")

        # Analyze per-node CPU
        f.write("CPU Usage (per node):\n")
        resources_file = output_dir / 'per-node-resources.csv'
        with open(resources_file, 'r') as rf:
            reader = csv.DictReader(rf)
            node_cpu = {}
            for row in reader:
                node = row['Node']
                if node not in node_cpu:
                    node_cpu[node] = []
                node_cpu[node].append(float(row['CPU_Percent']))

            for node in sorted(node_cpu.keys()):
                avg = sum(node_cpu[node]) / len(node_cpu[node])
                f.write(f"  {node}: {avg:.2f}% average\n")
        f.write("\n")

        # Analyze per-node memory
        f.write("Memory Usage (per node):\n")
        with open(resources_file, 'r') as rf:
            reader = csv.DictReader(rf)
            node_mem = {}
            for row in reader:
                node = row['Node']
                if node not in node_mem:
                    node_mem[node] = []
                node_mem[node].append(float(row['Memory_MB']))

            for node in sorted(node_mem.keys()):
                avg = sum(node_mem[node]) / len(node_mem[node])
                f.write(f"  {node}: {avg:.0f} MB average\n")
        f.write("\n")

        # Analyze database size and growth
        f.write("Database Size (per node):\n")
        database_file = output_dir / 'per-node-database.csv'
        with open(database_file, 'r') as df:
            reader = csv.DictReader(df)
            node_db = {}
            for row in reader:
                node = row['Node']
                if node not in node_db:
                    node_db[node] = []
                node_db[node].append(float(row['DB_Size_MB']))

            total_growth = 0
            for node in sorted(node_db.keys()):
                if len(node_db[node]) > 0:
                    initial = node_db[node][0]
                    final = node_db[node][-1]
                    growth = final - initial
                    total_growth += growth
                    f.write(f"  {node}: {final:.0f} MB (grew {growth:.0f} MB)\n")
        f.write("\n")

        # Database growth rate
        growth_rate_per_min = total_growth / (duration / 60)
        f.write(f"Database Growth Rate: {growth_rate_per_min:.2f} MB/minute\n")
        f.write(f"Projected hourly growth: {growth_rate_per_min * 60:.2f} MB/hour\n")
        f.write(f"Projected daily growth: {growth_rate_per_min * 60 * 24 / 1024:.2f} GB/day\n")

    # Display report
    with open(report_file, 'r') as f:
        print(f.read())

if __name__ == '__main__':
    output_dir = sys.argv[1] if len(sys.argv) > 1 else '/tmp/loadtest-workspace/monitoring-results'
    duration = int(sys.argv[2]) if len(sys.argv) > 2 else 300
    interval = int(sys.argv[3]) if len(sys.argv) > 3 else 10

    try:
        monitor(output_dir, duration, interval)
    except KeyboardInterrupt:
        print("\n\nMonitoring interrupted by user")
        sys.exit(0)
