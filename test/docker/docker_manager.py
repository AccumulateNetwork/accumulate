"""Docker operations for performance testing."""

import subprocess
import time
import logging
from typing import List, Tuple
from pathlib import Path
import json


logger = logging.getLogger(__name__)


class DockerManager:
    """Manages Docker containers, networks, and cleanup."""

    def __init__(self, compose_file: Path):
        self.compose_file = compose_file
        self.compose_dir = compose_file.parent

    def run(self, cmd: List[str], check: bool = True, timeout: int = 60) -> Tuple[int, str, str]:
        """Run a command and return (returncode, stdout, stderr)."""
        try:
            result = subprocess.run(
                cmd,
                cwd=self.compose_dir,
                capture_output=True,
                text=True,
                timeout=timeout
            )
            return result.returncode, result.stdout, result.stderr
        except subprocess.TimeoutExpired:
            logger.error(f"Command timed out: {' '.join(cmd)}")
            return -1, "", "Timeout"
        except Exception as e:
            logger.error(f"Command failed: {e}")
            return -1, "", str(e)

    def cleanup(self) -> bool:
        """Aggressively clean up all Docker resources."""
        logger.info("Starting aggressive Docker cleanup...")

        # Stop accumulate containers by pattern
        rc, out, err = self.run(
            ["sh", "-c", "docker ps -a 2>/dev/null | grep -E 'acc-|accumulate' | awk '{print $1}' | xargs -r docker stop 2>/dev/null || true"]
        )

        # Remove accumulate containers
        rc, out, err = self.run(
            ["sh", "-c", "docker ps -a 2>/dev/null | grep -E 'acc-|accumulate' | awk '{print $1}' | xargs -r docker rm -f 2>/dev/null || true"]
        )

        # Remove networks
        rc, out, err = self.run(
            ["sh", "-c", "docker network ls 2>/dev/null | grep -E 'acc-network|accumulate' | awk '{print $1}' | xargs -r docker network rm 2>/dev/null || true"]
        )

        # Remove volumes
        rc, out, err = self.run(
            ["sh", "-c", "docker volume ls 2>/dev/null | grep -E 'acc-|accumulate' | awk '{print $2}' | xargs -r docker volume rm -f 2>/dev/null || true"]
        )

        # Prune dangling resources
        self.run(["docker", "volume", "prune", "-f", "--filter", "label!=keep"])
        self.run(["docker", "system", "prune", "-f"])

        # Clear local temp directories
        self.run(["sh", "-c", "rm -rf /tmp/accumulate-* /tmp/perf-test-* 2>/dev/null || true"])

        time.sleep(3)  # Let cleanup settle
        logger.info("Docker cleanup complete")
        return True

    def verify_clean(self) -> bool:
        """Verify Docker state is clean."""
        logger.info("Verifying Docker clean state...")

        # Check for lingering containers
        rc, out, err = self.run(["sh", "-c", "docker ps -a 2>/dev/null | grep -c -E 'acc-|accumulate' || echo 0"])
        try:
            containers = int(out.strip().split('\n')[0]) if out.strip() else 0
        except (ValueError, IndexError):
            containers = 0

        if containers > 0:
            logger.warning(f"Found {containers} lingering accumulate containers")
            self.run(["docker", "ps", "-a"])
            return False

        # Check for lingering volumes
        rc, out, err = self.run(["sh", "-c", "docker volume ls 2>/dev/null | grep -c -E 'acc-|accumulate' || echo 0"])
        try:
            volumes = int(out.strip().split('\n')[0]) if out.strip() else 0
        except (ValueError, IndexError):
            volumes = 0

        if volumes > 0:
            logger.warning(f"Found {volumes} lingering accumulate volumes")
            self.run(["docker", "volume", "ls"])
            return False

        logger.info("Docker clean state verified")
        return True

    def compose_up(self) -> bool:
        """Start Docker Compose network."""
        logger.info("Starting Docker Compose network...")

        # Build (with explicit compose file)
        rc, out, err = self.run(["docker", "compose", "-f", str(self.compose_file), "build"], timeout=600)
        if rc != 0:
            logger.error(f"Docker build failed: {err}")
            return False

        # Up (with explicit compose file)
        rc, out, err = self.run(["docker", "compose", "-f", str(self.compose_file), "up", "-d"])
        if rc != 0:
            logger.error(f"Docker up failed: {err}")
            return False

        logger.info("Docker Compose started")
        return True

    def compose_down(self) -> bool:
        """Stop and remove Docker Compose network."""
        logger.info("Stopping Docker Compose network...")
        self.run(["docker", "compose", "-f", str(self.compose_file), "down", "-v"])
        time.sleep(2)
        return True

    def wait_healthy(self, timeout: int = 120) -> bool:
        """Wait for all containers to be healthy and nodes to respond to JSON-RPC."""
        logger.info(f"Waiting for containers to be healthy (timeout {timeout}s)...")
        start = time.time()

        # First, wait for all containers to be running
        while time.time() - start < timeout:
            rc, out, err = self.run(["docker", "compose", "-f", str(self.compose_file), "ps"])
            if "running" in out and "Exit" not in out:
                logger.info("All containers running")
                break
            time.sleep(2)

        # Then, wait for JSON-RPC to respond on validator ports
        logger.info("Waiting for JSON-RPC to be responsive...")
        remaining = timeout - (time.time() - start)
        json_rpc_start = time.time()

        while time.time() - json_rpc_start < remaining:
            try:
                # Try to reach a validator on standard port 26660 via JSON-RPC
                # /v3 endpoint requires POST with valid JSON-RPC request
                result = subprocess.run(
                    ["curl", "-s", "-X", "POST", "http://localhost:26660/v3",
                     "-H", "Content-Type: application/json",
                     "-d", '{"jsonrpc":"2.0","method":"jrpc_methods","params":[],"id":1}'],
                    capture_output=True,
                    text=True,
                    timeout=5
                )
                if result.returncode == 0 and "jsonrpc" in result.stdout:
                    logger.info("✓ JSON-RPC is responding")
                    return True
            except:
                pass

            time.sleep(3)

        logger.error("Containers or JSON-RPC did not become healthy")
        return False

    def get_logs(self, max_lines: int = 500) -> str:
        """Get Docker Compose logs."""
        rc, out, err = self.run(["docker", "compose", "-f", str(self.compose_file), "logs"])
        lines = out.split('\n')
        return '\n'.join(lines[-max_lines:])

    def verify_shards(self, expected: int = 64) -> Tuple[bool, List[str]]:
        """
        Verify that sharding is enabled with expected shard count.
        Returns (success, list of shard confirmation messages found).
        """
        logger.info(f"Verifying {expected}-shard execution...")

        logs = self.get_logs()
        shard_msgs = [
            line for line in logs.split('\n')
            if f'shard-count={expected}' in line and 'sharding=ENABLED' in line
        ]

        if shard_msgs:
            logger.info(f"Found {len(shard_msgs)} shard confirmation messages")
            return True, shard_msgs
        else:
            logger.warning(f"No messages found confirming {expected}-shard execution")
            # Find any executor initialization messages for debugging
            exec_msgs = [
                line for line in logs.split('\n')
                if 'Executor initialized' in line
            ]
            return False, exec_msgs

    def ps(self) -> str:
        """Get docker compose ps output."""
        rc, out, err = self.run(["docker", "compose", "-f", str(self.compose_file), "ps"])
        return out
