#!/usr/bin/env python3
"""
Example: Python client for Accumulate MCP

This shows how to invoke Accumulate MCP tools from Python scripts.
"""

import json
import subprocess
import sys
from typing import Dict, Any


class AccumulateMCPClient:
    """Client for interacting with Accumulate MCP server."""

    def __init__(self, mcp_server_path: str = "./mcp-server"):
        """
        Initialize MCP client.

        Args:
            mcp_server_path: Path to the MCP server executable
        """
        self.mcp_server_path = mcp_server_path
        self.request_id = 0

    def call_tool(self, tool_name: str, arguments: Dict[str, Any]) -> Dict[str, Any]:
        """
        Call an MCP tool.

        Args:
            tool_name: Name of the tool to call (e.g., "accumulate_get_genesis_files")
            arguments: Dictionary of tool arguments

        Returns:
            Tool response as dictionary

        Raises:
            RuntimeError: If the tool call fails
        """
        self.request_id += 1

        request = {
            "jsonrpc": "2.0",
            "id": self.request_id,
            "method": "tools/call",
            "params": {
                "name": tool_name,
                "arguments": arguments
            }
        }

        # Invoke MCP server via stdio
        try:
            result = subprocess.run(
                [self.mcp_server_path],
                input=json.dumps(request),
                capture_output=True,
                text=True,
                check=True
            )

            response = json.loads(result.stdout)

            if "error" in response:
                raise RuntimeError(f"MCP tool error: {response['error']}")

            return response.get("result", {})

        except subprocess.CalledProcessError as e:
            raise RuntimeError(f"Failed to call MCP server: {e.stderr}")
        except json.JSONDecodeError as e:
            raise RuntimeError(f"Invalid JSON response from MCP server: {e}")


# Example usage
def main():
    client = AccumulateMCPClient()

    # Example 1: Get genesis files
    print("Getting genesis file locations...")
    genesis_info = client.call_tool("accumulate_get_genesis_files", {
        "network": "mainnet",
        "bvn": "1"
    })
    print(json.dumps(genesis_info, indent=2))
    print()

    # Example 2: Check follower status
    print("Checking follower status...")
    try:
        status = client.call_tool("accumulate_follower_status", {
            "container_name": "accumulate-follower"
        })
        print(json.dumps(status, indent=2))
    except RuntimeError as e:
        print(f"Note: {e}")
    print()

    # Example 3: Initialize follower
    if len(sys.argv) > 2:
        dn_db = sys.argv[1]
        bvn_db = sys.argv[2]

        print(f"Initializing follower with:")
        print(f"  DN:  {dn_db}")
        print(f"  BVN: {bvn_db}")

        result = client.call_tool("accumulate_init_follower", {
            "dn_database": dn_db,
            "bvn_database": bvn_db,
            "work_dir": "/tmp/test-follower",
            "network": "MainNet"
        })

        print("Initialization result:")
        print(json.dumps(result, indent=2))


if __name__ == "__main__":
    main()
