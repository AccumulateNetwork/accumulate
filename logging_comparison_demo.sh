#!/bin/bash

# Demo: JSON vs Line-Oriented Logs for DevNet

echo "=== Logging Format Comparison ==="
echo

echo "📊 JSON Format (structured, requires jq):"
echo '{"timestamp":"2025-01-15T10:30:45Z","component":"devnet.conductor","partition":"BVN1","node":2,"source":"acc://bvn0","destination":"acc://bvn1","sequence":1234,"message_type":"synthetic","gap_size":5}'
echo

echo "🔍 Filtering JSON (complex):"
echo "jq 'select(.component==\"devnet.conductor\" and .gap_size > 0)'"
echo

echo "📝 Plain Format (grep-friendly):"
echo "2025-01-15 10:30:45 INFO devnet.conductor msg_processing src=acc://bvn0 dst=acc://bvn1 seq=1234 type=synthetic partition=BVN1 node=2"
echo "2025-01-15 10:30:46 WARN devnet.recovery gap_detected dst=acc://bvn1 expected=1240 last_known=1235 gap_size=5 action=reset_and_resend partition=BVN1"
echo

echo "🔍 Filtering Plain (simple):"
echo "grep 'devnet.conductor.*seq=1234'               # Find specific sequence"
echo "grep 'gap_size=' | grep -v 'gap_size=0'         # Find actual gaps"
echo "awk -F' ' '/msg_processing/ {print \$5, \$6, \$7}'  # Extract src, dst, seq"
echo "grep 'partition=BVN1' | tail -10                # Last 10 BVN1 events"
echo

echo "🚀 Performance Analysis Examples:"
echo 
echo "# Count messages per partition:"
echo "grep 'devnet.conductor' devnet.log | grep -o 'partition=[^[:space:]]*' | sort | uniq -c"
echo
echo "# Find gaps by partition:"  
echo "grep 'gap_detected.*partition=BVN1' devnet.log | grep -o 'gap_size=[0-9]*' | sort -t= -k2 -n"
echo
echo "# Monitor real-time conductor activity:"
echo "tail -f devnet.log | grep --line-buffered 'devnet.conductor'"
echo
echo "# Extract performance metrics:"
echo "grep 'req/s' devnet.log | awk '{print \$1, \$2, \$(NF-1), \$NF}' | tail -5"

echo
echo "✅ Recommendation: Use PLAIN format for devnet (default)"
echo "   - Faster grep/awk processing"
echo "   - Human readable for debugging"
echo "   - Familiar Unix tools"
echo "   - Still structured with key=value pairs"