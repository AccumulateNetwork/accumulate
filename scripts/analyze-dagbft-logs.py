#!/usr/bin/env python3
"""Analyze DAG-BFT logs and produce compact summaries."""

import sys
import re
import json
from pathlib import Path
from collections import defaultdict

ANSI_ESCAPE = re.compile(r'\x1b\[[0-9;]*m')

def strip_ansi(text: str) -> str:
    return ANSI_ESCAPE.sub('', text)

def analyze_log(log_path: Path) -> dict:
    """Analyze a single log file and return summary stats."""
    stats = {
        "node": log_path.stem,
        "certificates": 0,
        "max_round": 0,
        "max_signers": 0,
        "min_signers": 999,
        "errors": [],
        "warnings": [],
        "validators_found": 0,
        "genesis_mode": None,
    }

    if not log_path.exists():
        stats["error"] = "file not found"
        return stats

    with open(log_path) as f:
        for raw_line in f:
            line = strip_ansi(raw_line)
            # Certificate creation
            if "Created certificate" in line:
                stats["certificates"] += 1
                if m := re.search(r'round=(\d+)', line):
                    stats["max_round"] = max(stats["max_round"], int(m.group(1)))
                if m := re.search(r'signers=(\d+)', line):
                    signers = int(m.group(1))
                    stats["max_signers"] = max(stats["max_signers"], signers)
                    stats["min_signers"] = min(stats["min_signers"], signers)

            # Validator extraction
            if "Extracted initial validators" in line:
                if m := re.search(r'validators=(\d+)', line):
                    stats["validators_found"] = int(m.group(1))

            # Genesis mode
            if "Multi-validator mode" in line:
                stats["genesis_mode"] = "multi"
            elif "single-validator mode" in line:
                stats["genesis_mode"] = "single"

            # Errors (last 5)
            if "ERROR" in line or "error" in line.lower():
                if len(stats["errors"]) < 5:
                    stats["errors"].append(line.strip()[:100])

            # Warnings (last 3)
            if "WARN" in line:
                if len(stats["warnings"]) < 3:
                    stats["warnings"].append(line.strip()[:100])

    if stats["min_signers"] == 999:
        stats["min_signers"] = 0

    return stats

def main():
    log_dir = Path(sys.argv[1]) if len(sys.argv) > 1 else Path("/tmp/dagbft-stress")

    if not log_dir.exists():
        print(json.dumps({"error": f"Directory not found: {log_dir}"}))
        return

    results = []
    for log_file in sorted(log_dir.glob("*.log")):
        results.append(analyze_log(log_file))

    # Print compact summary
    print("=" * 60)
    print("DAG-BFT Log Analysis Summary")
    print("=" * 60)

    for r in results:
        status = "OK" if r["certificates"] > 0 and r["min_signers"] >= 3 else "ISSUE"
        print(f"\n{r['node']}: {status}")
        print(f"  Certs: {r['certificates']}, MaxRound: {r['max_round']}, Signers: {r['min_signers']}-{r['max_signers']}")
        print(f"  Validators: {r['validators_found']}, Genesis: {r['genesis_mode']}")
        if r["errors"]:
            print(f"  Errors: {len(r['errors'])}")
        if r["warnings"]:
            print(f"  Warnings: {len(r['warnings'])}")

    # Overall health
    print("\n" + "=" * 60)
    healthy = sum(1 for r in results if r["certificates"] > 0 and r["min_signers"] >= 3)
    print(f"Healthy nodes: {healthy}/{len(results)}")

    # Output JSON for programmatic use
    print("\n--- JSON ---")
    print(json.dumps(results, indent=2))

if __name__ == "__main__":
    main()
