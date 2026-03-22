#!/bin/bash
# Copyright 2025 The Accumulate Authors
#
# Use of this source code is governed by an MIT-style
# license that can be found in the LICENSE file or at
# https://opensource.org/licenses/MIT.

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Script configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT_DIR="${OUTPUT_DIR:-./comparison-results}"

# Usage information
usage() {
    cat << EOF
Usage: $0 [OPTIONS] REPORT1 REPORT2 [REPORT3 ...]

Compare multiple performance test reports and generate comparative analysis.

OPTIONS:
    -o, --output DIR        Output directory for comparison results (default: ./comparison-results)
    -f, --format FORMAT     Output format: table, json, csv (default: table)
    -b, --baseline FILE     Baseline report for comparison
    -s, --sort FIELD        Sort by field: tps, latency, errors (default: tps)
    -h, --help              Show this help message

ARGUMENTS:
    REPORT1 REPORT2 ...     Performance report JSON files to compare

EXAMPLES:
    # Compare two reports
    $0 report1.json report2.json

    # Compare multiple iterations
    $0 baseline.json iter1.json iter2.json iter3.json

    # Compare with specific baseline
    $0 --baseline baseline.json current.json

    # Output as JSON
    $0 --format json report1.json report2.json -o ./results

EOF
    exit 1
}

# Parse command line arguments
FORMAT="table"
BASELINE=""
SORT_FIELD="tps"
REPORTS=()

while [[ $# -gt 0 ]]; do
    case $1 in
        -o|--output)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        -f|--format)
            FORMAT="$2"
            shift 2
            ;;
        -b|--baseline)
            BASELINE="$2"
            shift 2
            ;;
        -s|--sort)
            SORT_FIELD="$2"
            shift 2
            ;;
        -h|--help)
            usage
            ;;
        -*)
            echo "Unknown option: $1"
            usage
            ;;
        *)
            REPORTS+=("$1")
            shift
            ;;
    esac
done

# Validate inputs
if [ ${#REPORTS[@]} -lt 2 ]; then
    echo -e "${RED}Error: At least 2 report files are required${NC}"
    usage
fi

for report in "${REPORTS[@]}"; do
    if [ ! -f "$report" ]; then
        echo -e "${RED}Error: Report file not found: $report${NC}"
        exit 1
    fi
done

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Extract metrics from a report
extract_metrics() {
    local report_file="$1"
    local metrics_json=$(cat "$report_file")

    echo "$metrics_json" | jq -r '{
        file: input_filename,
        target_tps: .target_tps,
        achieved_tps: .achieved_tps,
        submitted: .submitted_total,
        success: .success_total,
        errors: .error_total,
        success_rate: .success_rate_percent,
        latency_mean: .latency_mean_ms,
        latency_p50: .latency_p50_ms,
        latency_p95: .latency_p95_ms,
        latency_p99: .latency_p99_ms,
        block_count: .block_count,
        block_rate: .blocks_per_minute,
        duration: .duration_seconds
    }' <<< "$metrics_json"
}

# Calculate percentage change
calc_percent_change() {
    local old_val="$1"
    local new_val="$2"

    if [ "$old_val" == "0" ] || [ "$old_val" == "null" ]; then
        echo "N/A"
    else
        echo "scale=2; (($new_val - $old_val) / $old_val) * 100" | bc
    fi
}

# Format change with color
format_change() {
    local change="$1"
    local reverse="${2:-false}"  # true for latency (lower is better)

    if [ "$change" == "N/A" ]; then
        echo "N/A"
        return
    fi

    local abs_change=$(echo "$change" | sed 's/-//')
    if (( $(echo "$abs_change < 1" | bc -l) )); then
        echo "${change}%"
        return
    fi

    if [ "$reverse" == "true" ]; then
        if (( $(echo "$change < 0" | bc -l) )); then
            echo -e "${GREEN}${change}%${NC}"
        else
            echo -e "${RED}+${change}%${NC}"
        fi
    else
        if (( $(echo "$change > 0" | bc -l) )); then
            echo -e "${GREEN}+${change}%${NC}"
        else
            echo -e "${RED}${change}%${NC}"
        fi
    fi
}

# Print table header
print_table_header() {
    echo "================================================================================"
    echo "PERFORMANCE COMPARISON REPORT"
    echo "================================================================================"
    echo ""
}

# Print table comparison
print_table_comparison() {
    local -n reports_ref=$1

    print_table_header

    # Print summary table
    printf "%-30s %10s %10s %10s %10s %10s\n" \
        "Report" "Target" "Achieved" "Success%" "P99(ms)" "Blocks/m"
    echo "--------------------------------------------------------------------------------"

    for report in "${reports_ref[@]}"; do
        local filename=$(basename "$report")
        local target=$(jq -r '.target_tps' "$report")
        local achieved=$(jq -r '.achieved_tps' "$report")
        local success=$(jq -r '.success_rate_percent' "$report")
        local p99=$(jq -r '.latency_p99_ms' "$report")
        local blocks=$(jq -r '.blocks_per_minute' "$report")

        printf "%-30s %10d %10.2f %10.2f %10.2f %10.2f\n" \
            "${filename:0:30}" "$target" "$achieved" "$success" "$p99" "$blocks"
    done

    echo ""

    # Detailed comparison if baseline is provided
    if [ -n "$BASELINE" ]; then
        echo "BASELINE COMPARISON"
        echo "--------------------------------------------------------------------------------"

        local base_tps=$(jq -r '.achieved_tps' "$BASELINE")
        local base_latency=$(jq -r '.latency_p99_ms' "$BASELINE")
        local base_success=$(jq -r '.success_rate_percent' "$BASELINE")

        printf "%-30s %15s %15s %15s\n" "Report" "TPS Change" "Latency Chg" "Success Chg"
        echo "--------------------------------------------------------------------------------"

        for report in "${reports_ref[@]}"; do
            if [ "$report" == "$BASELINE" ]; then
                continue
            fi

            local filename=$(basename "$report")
            local curr_tps=$(jq -r '.achieved_tps' "$report")
            local curr_latency=$(jq -r '.latency_p99_ms' "$report")
            local curr_success=$(jq -r '.success_rate_percent' "$report")

            local tps_change=$(calc_percent_change "$base_tps" "$curr_tps")
            local lat_change=$(calc_percent_change "$base_latency" "$curr_latency")
            local suc_change=$(calc_percent_change "$base_success" "$curr_success")

            local tps_colored=$(format_change "$tps_change" "false")
            local lat_colored=$(format_change "$lat_change" "true")
            local suc_colored=$(format_change "$suc_change" "false")

            printf "%-30s %15s %15s %15s\n" \
                "${filename:0:30}" "$tps_colored" "$lat_colored" "$suc_colored"
        done

        echo ""
    fi

    # Statistical summary
    echo "STATISTICAL SUMMARY"
    echo "--------------------------------------------------------------------------------"

    local all_tps=$(for r in "${reports_ref[@]}"; do jq -r '.achieved_tps' "$r"; done)
    local all_latency=$(for r in "${reports_ref[@]}"; do jq -r '.latency_p99_ms' "$r"; done)

    local min_tps=$(echo "$all_tps" | sort -n | head -1)
    local max_tps=$(echo "$all_tps" | sort -n | tail -1)
    local avg_tps=$(echo "$all_tps" | awk '{s+=$1} END {printf "%.2f", s/NR}')

    local min_lat=$(echo "$all_latency" | sort -n | head -1)
    local max_lat=$(echo "$all_latency" | sort -n | tail -1)
    local avg_lat=$(echo "$all_latency" | awk '{s+=$1} END {printf "%.2f", s/NR}')

    echo "TPS:     Min: $min_tps | Max: $max_tps | Avg: $avg_tps"
    echo "P99 Latency: Min: $min_lat ms | Max: $max_lat ms | Avg: $avg_lat ms"
    echo ""

    echo "================================================================================"
}

# Generate JSON comparison
generate_json_comparison() {
    local -n reports_ref=$1
    local output_file="$OUTPUT_DIR/comparison.json"

    echo "{" > "$output_file"
    echo "  \"comparison_time\": \"$(date -Iseconds)\"," >> "$output_file"
    echo "  \"baseline\": \"$BASELINE\"," >> "$output_file"
    echo "  \"reports\": [" >> "$output_file"

    local first=true
    for report in "${reports_ref[@]}"; do
        if [ "$first" = true ]; then
            first=false
        else
            echo "," >> "$output_file"
        fi

        echo "    {" >> "$output_file"
        echo "      \"file\": \"$report\"," >> "$output_file"
        cat "$report" | jq -c '.' | sed 's/^/      "metrics": /' >> "$output_file"
        echo "    }" | tr -d '\n' >> "$output_file"
    done

    echo "" >> "$output_file"
    echo "  ]" >> "$output_file"
    echo "}" >> "$output_file"

    echo -e "${GREEN}JSON comparison saved to: $output_file${NC}"
}

# Generate CSV comparison
generate_csv_comparison() {
    local -n reports_ref=$1
    local output_file="$OUTPUT_DIR/comparison.csv"

    echo "file,target_tps,achieved_tps,submitted,success,errors,success_rate,latency_mean,latency_p50,latency_p95,latency_p99,block_count,block_rate,duration" > "$output_file"

    for report in "${reports_ref[@]}"; do
        local metrics=$(cat "$report" | jq -r '[
            input_filename,
            .target_tps,
            .achieved_tps,
            .submitted_total,
            .success_total,
            .error_total,
            .success_rate_percent,
            .latency_mean_ms,
            .latency_p50_ms,
            .latency_p95_ms,
            .latency_p99_ms,
            .block_count,
            .blocks_per_minute,
            .duration_seconds
        ] | @csv')

        echo "$report,$metrics" | sed 's/"//g' >> "$output_file"
    done

    echo -e "${GREEN}CSV comparison saved to: $output_file${NC}"
}

# Main execution
main() {
    echo -e "${BLUE}Comparing ${#REPORTS[@]} performance reports...${NC}"
    echo ""

    case $FORMAT in
        table)
            print_table_comparison REPORTS
            ;;
        json)
            generate_json_comparison REPORTS
            print_table_comparison REPORTS
            ;;
        csv)
            generate_csv_comparison REPORTS
            print_table_comparison REPORTS
            ;;
        *)
            echo -e "${RED}Unknown format: $FORMAT${NC}"
            usage
            ;;
    esac

    echo -e "${GREEN}Comparison complete!${NC}"
}

# Run main function
main
