"""Generate dynamic Docker Compose files for different validator/BVN configurations."""

import logging
from pathlib import Path
from test_config import TestConfig

logger = logging.getLogger(__name__)


def generate_docker_compose(config: TestConfig, output_dir: Path) -> Path:
    """
    Generate a docker-compose.yml for the given validator/BVN configuration.

    Returns the path to the generated file.
    """
    output_file = output_dir / f"docker-compose-{config.validators}-val-{config.bvns}-bvn.yml"

    logger.info(f"Generating Docker Compose for {config.validators} validators, {config.bvns} BVN(s)")

    # Generate docker-network.yml for this configuration
    network_config = generate_network_config(config, output_dir)

    # Base docker-compose structure
    compose = f"""version: '3.8'

services:
  # CRITICAL: Bootstrap server is REQUIRED for peer discovery
  bootstrap:
    build:
      context: ../..
      dockerfile: Dockerfile
    container_name: acc-bootstrap-{config.validators}v-{config.bvns}b
    mem_limit: 1g
    ports:
      - "16593:16593"
    entrypoint: ["/bin/accumulated-bootstrap"]
    command:
      - --listen
      - /ip4/0.0.0.0/tcp/16593
      - --key
      - "626f6f7473747261702d6e6f64652d6b65792d736565642d7631000000000000"
    restart: unless-stopped
    healthcheck:
      test: ["CMD-SHELL", "netstat -tuln | grep -q ':16593 ' || exit 1"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s

  # Initialize network configuration
  init:
    build:
      context: ../..
      dockerfile: Dockerfile
    container_name: acc-init-{config.validators}v-{config.bvns}b
    volumes:
      - network-config:/root/.accumulate
      - ../..:/workspace
    working_dir: /workspace
    command:
      - init
      - network
      - {network_config}
      - --reset
      - --faucet-seed=FAUCET
"""

    # Generate validator services for each BVN
    service_count = 0
    for bvn_idx in range(1, config.bvns + 1):
        for val_idx in range(1, config.validators + 1):
            service_name = f"bvn{bvn_idx}-val{val_idx}"
            container_name = f"acc-bvn{bvn_idx}-val{val_idx}-{config.validators}v-{config.bvns}b"
            port = 26660 + service_count
            service_count += 1

            compose += f"""
  {service_name}:
    build:
      context: ../..
      dockerfile: Dockerfile
    container_name: {container_name}
    mem_limit: 1g
    volumes:
      - network-config:/root/.accumulate
    command:
      - -w=/root/.accumulate/bvn{bvn_idx}-{val_idx}
      - /root/.accumulate/bvn{bvn_idx}-{val_idx}/accumulate.toml
    ports:
      - "{port}:26660"
    depends_on:
      init:
        condition: service_completed_successfully
      bootstrap:
        condition: service_started
"""

    # Volume definition
    compose += """
volumes:
  network-config:
"""

    # Write to file
    output_file.write_text(compose)
    logger.info(f"Generated: {output_file}")

    return output_file


def generate_network_config(config: TestConfig, output_dir: Path) -> str:
    """Generate docker-network.yml for this configuration."""
    network_file = output_dir / f"docker-network-{config.validators}-val-{config.bvns}-bvn.yml"

    # Build BVN configurations
    bvns_section = ""
    for bvn_idx in range(1, config.bvns + 1):
        nodes_section = ""
        for val_idx in range(1, config.validators + 1):
            nodes_section += f"""      - listenAddress: "0.0.0.0"
        peerAddress: "acc-bvn{bvn_idx}-val{val_idx}-{config.validators}v-{config.bvns}b"
        advertizeAddress: "acc-bvn{bvn_idx}-val{val_idx}-{config.validators}v-{config.bvns}b"
        basePort: 26656
        dnnType: "validator"
        bvnnType: "validator"
"""
        bvns_section += f"""  - id: "BVN{bvn_idx}"
    nodes:
{nodes_section}"""

    network_config = f"""id: "DAGBFTTest-{config.validators}v-{config.bvns}b"

# Bootstrap node configuration for Docker peer discovery
bootstrap:
  peerAddress: "acc-bootstrap-{config.validators}v-{config.bvns}b"
  advertizeAddress: "acc-bootstrap-{config.validators}v-{config.bvns}b"
  listenAddress: "0.0.0.0"
  basePort: 16593

# Global network configuration
globals:
  executorVersion: "v2"
  oracle:
    price: 10000000  # 1000 * AcmeOraclePrecision (devnet default: 0.10 USD per ACME)
  globals:
    majorBlockSchedule: "0 */12 * * *"  # Major blocks every 12 hours

bvns:
{bvns_section}
"""

    network_file.write_text(network_config)
    logger.info(f"Generated network config: {network_file}")

    return f"test/docker/docker-network-{config.validators}-val-{config.bvns}-bvn.yml"
