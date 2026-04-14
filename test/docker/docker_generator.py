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

    # Base docker-compose structure
    compose = f"""version: '3.8'

services:
  bootstrap:
    image: docker.io/library/alpine:latest
    container_name: acc-bootstrap-{config.validators}v-{config.bvns}b
    environment:
      BVNS: {config.bvns}
      VALIDATORS: {config.validators}
    ports:
      - "16593:16593"
    networks:
      - acc-network
    healthcheck:
      test: ["CMD", "test", "-f", "/tmp/bootstrap-ready"]
      interval: 5s
      timeout: 3s
      retries: 20
      start_period: 10s
    entrypoint: /bin/sh -c "touch /tmp/bootstrap-ready && sleep infinity"
"""

    # Generate validator services for each BVN
    for bvn_idx in range(1, config.bvns + 1):
        for val_idx in range(1, config.validators + 1):
            service_name = f"bvn{bvn_idx}-val{val_idx}"
            container_name = f"acc-bvn{bvn_idx}-val{val_idx}-{config.validators}v-{config.bvns}b"
            port = 26660 + (bvn_idx - 1) * config.validators + (val_idx - 1)

            compose += f"""
  {service_name}:
    image: docker.io/library/alpine:latest
    container_name: {container_name}
    depends_on:
      bootstrap:
        condition: service_healthy
    environment:
      BVNS: {config.bvns}
      VALIDATORS: {config.validators}
      BVN_INDEX: {bvn_idx}
      VAL_INDEX: {val_idx}
    ports:
      - "{port}:26660"
    networks:
      - acc-network
    healthcheck:
      test: ["CMD", "test", "-f", "/tmp/validator-ready"]
      interval: 5s
      timeout: 3s
      retries: 20
      start_period: 10s
    entrypoint: /bin/sh -c "touch /tmp/validator-ready && sleep infinity"
"""

    # Network definition
    compose += """
networks:
  acc-network:
    driver: bridge
"""

    # Write to file
    output_file.write_text(compose)
    logger.info(f"Generated: {output_file}")

    return output_file
