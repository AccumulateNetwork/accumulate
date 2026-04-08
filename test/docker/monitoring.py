#!/usr/bin/env python3
"""
Per-node monitoring for Accumulate validators.
Collects CPU, memory, and database metrics from running containers.
"""

import csv
import json
import os
import subprocess
import time
from pathlib import Path
from datetime import datetime

# Validator container names (must match docker-compose.yml)
VALIDATORS = [
    "acc-bvn1-val1", "acc-bvn1-val2", "acc-bvn1-val3", "acc-bvn1-val4",
    "acc-bvn2-val1", "acc-bvn2-val2", "acc-bvn2-val3", "acc-bvn2-val4",
    "acc-bvn3-val1", "acc-bvn3-val2", "acc-bvn3-val3", "acc-bvn3-val4",
]

# Data directory mapping (container_name -> bvn-N path)
CONTAINER_TO_PATH = {
    "acc-bvn1-val1": "bvn1-1", "acc-bvn1-val2": "bvn1-2",
    "acc-bvn1-val3": "bvn1-3", "acc-bvn1-val4": "bvn1-4",
    "acc-bvn2-val1": "bvn2-1", "acc-bvn2-val2": "bvn2-2",
    "acc-bvn2-val3": "bvn2-3", "acc-bvn2-val4": "bvn2-4",
    "acc-bvn3-val1": "bvn3-1", "acc-bvn3-val2": "bvn3-2",
    "acc-bvn3-val3": "bvn3-3", "acc-bvn3-val4": "bvn3-4",
}

def get_docker_stats():
    """Get CPU and memory stats for all validators using docker stats."""
    stats = {}

    try:
        # Get stats in JSON format for all running containers
        result = subprocess.run(
            ["docker", "stats", "--no-stream", "--format", "json"],
            capture_output=True,
            text=True,
            timeout=5
        )

        if result.returncode == 0:
            for line in result.stdout.strip().split('\n'):
                if not line:
                    continue
                try:
                    data = json.loads(line)
                    container_name = data.get('Name', '')

                    if container_name in VALIDATORS:
                        # Parse CPU percentage (e.g., "12.34%")
                        cpu_str = data.get('CPUPerc', '0%').replace('%', '')
                        cpu = float(cpu_str) if cpu_str else 0

                        # Parse memory (e.g., "256.5MiB / 1GiB")
                        mem_str = data.get('MemUsage', '0MiB').split('/')[0].strip()
                        mem_mb = parse_memory(mem_str)

                        stats[container_name] = {
                            'cpu': cpu,
                            'memory_mb': mem_mb
                        }
                except (json.JSONDecodeError, ValueError, KeyError) as e:
                    print(f"Error parsing stats for line: {e}")
    except subprocess.TimeoutExpired:
        print("docker stats timed out")
    except Exception as e:
        print(f"Error getting docker stats: {e}")

    return stats

def parse_memory(mem_str):
    """Parse memory string (e.g., '256.5MiB') to MB."""
    try:
        mem_str = mem_str.strip()
        if 'GiB' in mem_str:
            return float(mem_str.replace('GiB', '')) * 1024
        elif 'MiB' in mem_str:
            return float(mem_str.replace('MiB', ''))
        elif 'KiB' in mem_str:
            return float(mem_str.replace('KiB', '')) / 1024
        else:
            return 0
    except:
        return 0

def get_db_sizes(network_config_path):
    """Get database sizes for each validator."""
    db_sizes = {}

    network_path = Path(network_config_path)
    if not network_path.exists():
        return db_sizes

    for container_name, bvn_path in CONTAINER_TO_PATH.items():
        db_path = network_path / bvn_path / "blockdb"

        if db_path.exists():
            try:
                # Sum all files in the database directory
                total_size = sum(
                    f.stat().st_size for f in db_path.rglob('*') if f.is_file()
                )
                db_sizes[container_name] = total_size / (1024 * 1024)  # Convert to MB
            except Exception as e:
                print(f"Error getting DB size for {bvn_path}: {e}")
                db_sizes[container_name] = 0
        else:
            db_sizes[container_name] = 0

    return db_sizes

def write_resource_csv(stats, output_file):
    """Write per-node resource metrics to CSV."""
    timestamp = datetime.now().isoformat()

    try:
        file_exists = Path(output_file).exists()

        with open(output_file, 'a', newline='') as f:
            writer = csv.DictWriter(f, fieldnames=['Timestamp', 'Node', 'CPU_Percent', 'Memory_MB'])

            # Write header if new file
            if not file_exists:
                writer.writeheader()

            # Write one row per validator
            for container_name, metrics in sorted(stats.items()):
                writer.writerow({
                    'Timestamp': timestamp,
                    'Node': container_name,
                    'CPU_Percent': f"{metrics['cpu']:.1f}",
                    'Memory_MB': f"{metrics['memory_mb']:.1f}"
                })
    except Exception as e:
        print(f"Error writing resource CSV: {e}")

def write_database_csv(db_sizes, output_file):
    """Write per-node database metrics to CSV."""
    timestamp = datetime.now().isoformat()

    try:
        file_exists = Path(output_file).exists()

        with open(output_file, 'a', newline='') as f:
            writer = csv.DictWriter(f, fieldnames=['Timestamp', 'Node', 'DB_Size_MB'])

            # Write header if new file
            if not file_exists:
                writer.writeheader()

            # Write one row per validator
            for container_name, size_mb in sorted(db_sizes.items()):
                writer.writerow({
                    'Timestamp': timestamp,
                    'Node': container_name,
                    'DB_Size_MB': f"{size_mb:.1f}"
                })
    except Exception as e:
        print(f"Error writing database CSV: {e}")

def main():
    """Main monitoring loop."""
    # Create monitoring directory if it doesn't exist
    monitoring_dir = Path('/tmp/monitoring')
    monitoring_dir.mkdir(parents=True, exist_ok=True)

    # Network config path (same as used by docker-compose)
    network_config = Path('/root/.accumulate')

    print("Starting per-node monitoring...")
    print(f"Monitoring directory: {monitoring_dir}")
    print(f"Network config path: {network_config}")
    print(f"Monitoring {len(VALIDATORS)} validators\n")

    # Initial collection
    interval = 10  # Collect every 10 seconds

    while True:
        try:
            # Get current metrics
            stats = get_docker_stats()
            db_sizes = get_db_sizes(network_config)

            # Write to CSV files
            write_resource_csv(stats, monitoring_dir / 'per-node-resources.csv')
            write_database_csv(db_sizes, monitoring_dir / 'per-node-database.csv')

            # Log summary
            timestamp = datetime.now().strftime('%H:%M:%S')
            print(f"[{timestamp}] Collected metrics for {len(stats)} validators")

            time.sleep(interval)
        except KeyboardInterrupt:
            print("\nMonitoring stopped")
            break
        except Exception as e:
            print(f"Error in monitoring loop: {e}")
            time.sleep(interval)

if __name__ == '__main__':
    main()
