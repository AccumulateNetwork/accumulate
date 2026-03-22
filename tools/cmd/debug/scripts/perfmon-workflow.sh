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
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Script configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORKFLOW_DIR="${WORKFLOW_DIR:-./perfmon-workflow}"

# Default values
SERVER=""
TARGET_TPS=""
DURATION=""
ITERATIONS=3
THRESHOLD=0.85
AUTO_APPLY=false
WAIT_BETWEEN=30
DEBUG_CMD="debug"

# Usage information
usage() {
    cat << EOF
Usage: $0 [OPTIONS] SERVER TPS DURATION [ITERATIONS]

Automated iterative performance tuning workflow.

This script automates the process of:
1. Running baseline performance test
2. Analyzing results and generating recommendations
3. (Optionally) Applying recommended tuning
4. Running follow-up tests
5. Comparing results with baseline
6. Repeating until target performance is achieved

OPTIONS:
    -w, --workflow-dir DIR  Working directory for all outputs (default: ./perfmon-workflow)
    -t, --threshold FLOAT   TPS achievement threshold (0.0-1.0, default: 0.85)
    -a, --auto-apply        Automatically apply tuning recommendations (USE WITH CAUTION)
    -d, --delay SECONDS     Wait time between iterations (default: 30)
    -c, --cmd COMMAND       Debug command to use (default: debug)
    -h, --help              Show this help message

ARGUMENTS:
    SERVER          Server endpoint (e.g., localhost:16695)
    TPS             Target transactions per second
    DURATION        Test duration (e.g., 5m, 10m, 1h)
    ITERATIONS      Number of tuning iterations (default: 3)

EXAMPLES:
    # Basic workflow with 3 iterations
    $0 localhost:16695 100 5m

    # Extended workflow with 5 iterations
    $0 localhost:16695 500 10m 5

    # High threshold, manual tuning
    $0 --threshold 0.95 localhost:16695 1000 15m

    # Automated tuning (requires write access to config)
    $0 --auto-apply localhost:16695 100 5m

WORKFLOW OUTPUTS:
    {workflow-dir}/
    ├── baseline/
    │   ├── report_*.json
    │   ├── metrics_*.csv
    │   └── recommendations.json
    ├── iteration-1/
    │   ├── report_*.json
    │   ├── metrics_*.csv
    │   ├── analysis.json
    │   └── recommendations.json
    ├── iteration-2/
    │   └── ...
    └── summary.json

EOF
    exit 1
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -w|--workflow-dir)
            WORKFLOW_DIR="$2"
            shift 2
            ;;
        -t|--threshold)
            THRESHOLD="$2"
            shift 2
            ;;
        -a|--auto-apply)
            AUTO_APPLY=true
            shift
            ;;
        -d|--delay)
            WAIT_BETWEEN="$2"
            shift 2
            ;;
        -c|--cmd)
            DEBUG_CMD="$2"
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
            if [ -z "$SERVER" ]; then
                SERVER="$1"
            elif [ -z "$TARGET_TPS" ]; then
                TARGET_TPS="$1"
            elif [ -z "$DURATION" ]; then
                DURATION="$1"
            elif [ -z "$ITERATIONS" ]; then
                ITERATIONS="$1"
            else
                echo "Too many arguments"
                usage
            fi
            shift
            ;;
    esac
done

# Validate inputs
if [ -z "$SERVER" ] || [ -z "$TARGET_TPS" ] || [ -z "$DURATION" ]; then
    echo -e "${RED}Error: SERVER, TPS, and DURATION are required${NC}"
    usage
fi

# Create workflow directory
mkdir -p "$WORKFLOW_DIR"

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_step() {
    echo ""
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${CYAN}$1${NC}"
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
}

# Run performance test
run_perfmon() {
    local output_dir="$1"
    local iteration_name="$2"

    log_info "Running performance test: $iteration_name"
    log_info "Target: $TARGET_TPS TPS for $DURATION"
    log_info "Output: $output_dir"

    "$DEBUG_CMD" perfmon "$SERVER" "$TARGET_TPS" "$DURATION" \
        --output "$output_dir" \
        --interval 10s

    log_success "Performance test completed"

    # Find the generated report file
    local report_file=$(ls -t "$output_dir"/report_*.json 2>/dev/null | head -1)
    if [ -z "$report_file" ]; then
        log_error "Report file not found in $output_dir"
        exit 1
    fi

    echo "$report_file"
}

# Analyze performance report
analyze_report() {
    local report_file="$1"
    local output_file="$2"
    local baseline_file="$3"

    log_info "Analyzing performance report"

    local cmd="$DEBUG_CMD perfmon-tuner \"$report_file\" --output \"$output_file\" --threshold $THRESHOLD"

    if [ -n "$baseline_file" ]; then
        cmd="$cmd --baseline \"$baseline_file\""
        log_info "Comparing with baseline: $baseline_file"
    fi

    eval "$cmd"

    log_success "Analysis completed: $output_file"
}

# Extract key metrics from report
extract_metrics() {
    local report_file="$1"

    local achieved_tps=$(jq -r '.achieved_tps' "$report_file")
    local target_tps=$(jq -r '.target_tps' "$report_file")
    local success_rate=$(jq -r '.success_rate_percent' "$report_file")
    local p99_latency=$(jq -r '.latency_p99_ms' "$report_file")

    local achievement=0
    if [ "$target_tps" != "0" ] && [ "$target_tps" != "null" ]; then
        achievement=$(echo "scale=4; $achieved_tps / $target_tps" | bc)
    fi

    echo "$achievement $achieved_tps $success_rate $p99_latency"
}

# Check if target is met
check_target_met() {
    local achievement="$1"

    local threshold_met=$(echo "$achievement >= $THRESHOLD" | bc -l)
    [ "$threshold_met" -eq 1 ]
}

# Display recommendations
display_recommendations() {
    local recommendations_file="$1"

    log_info "Top recommendations:"
    echo ""

    # Extract and display top 5 high-priority recommendations
    jq -r '.recommendations[] | select(.priority == "HIGH") |
        "  [\(.priority)] \(.category).\(.parameter)\n" +
        "    Suggested: \(.suggested_value)\n" +
        "    Reason: \(.reason)\n" +
        "    Impact: \(.expected_impact)\n"' \
        "$recommendations_file" | head -n 20

    echo ""
}

# Apply tuning recommendations (placeholder)
apply_tuning() {
    local recommendations_file="$1"

    if [ "$AUTO_APPLY" != true ]; then
        log_warning "Auto-apply is disabled. Please review recommendations and apply manually."
        log_info "Recommendations file: $recommendations_file"
        echo ""
        read -p "Press Enter after applying tuning to continue, or Ctrl+C to exit..."
        return
    fi

    log_warning "Auto-apply is enabled"
    log_warning "This feature requires implementation specific to your configuration system"
    log_info "For now, please apply recommendations manually"
    echo ""
    read -p "Press Enter after applying tuning to continue, or Ctrl+C to exit..."
}

# Generate workflow summary
generate_summary() {
    local workflow_dir="$1"

    log_info "Generating workflow summary"

    local summary_file="$workflow_dir/summary.json"
    local baseline_report=$(ls -t "$workflow_dir"/baseline/report_*.json 2>/dev/null | head -1)

    if [ -z "$baseline_report" ]; then
        log_error "Baseline report not found"
        return
    fi

    echo "{" > "$summary_file"
    echo "  \"workflow_start\": \"$(date -Iseconds)\"," >> "$summary_file"
    echo "  \"configuration\": {" >> "$summary_file"
    echo "    \"server\": \"$SERVER\"," >> "$summary_file"
    echo "    \"target_tps\": $TARGET_TPS," >> "$summary_file"
    echo "    \"duration\": \"$DURATION\"," >> "$summary_file"
    echo "    \"threshold\": $THRESHOLD," >> "$summary_file"
    echo "    \"iterations\": $ITERATIONS" >> "$summary_file"
    echo "  }," >> "$summary_file"
    echo "  \"iterations\": [" >> "$summary_file"

    # Add baseline
    echo "    {" >> "$summary_file"
    echo "      \"name\": \"baseline\"," >> "$summary_file"
    cat "$baseline_report" | jq -c '.' | sed 's/^/      "results": /' >> "$summary_file"
    echo "    }" >> "$summary_file"

    # Add iterations
    for i in $(seq 1 $ITERATIONS); do
        local iter_report=$(ls -t "$workflow_dir"/iteration-$i/report_*.json 2>/dev/null | head -1)
        if [ -n "$iter_report" ]; then
            echo "," >> "$summary_file"
            echo "    {" >> "$summary_file"
            echo "      \"name\": \"iteration-$i\"," >> "$summary_file"
            cat "$iter_report" | jq -c '.' | sed 's/^/      "results": /' >> "$summary_file"

            local analysis_file="$workflow_dir/iteration-$i/analysis.json"
            if [ -f "$analysis_file" ]; then
                echo "," >> "$summary_file"
                cat "$analysis_file" | jq -c '.baseline_comparison' | sed 's/^/      "comparison": /' >> "$summary_file"
            fi

            echo "    }" | tr -d '\n' >> "$summary_file"
        fi
    done

    echo "" >> "$summary_file"
    echo "  ]" >> "$summary_file"
    echo "}" >> "$summary_file"

    log_success "Workflow summary saved to: $summary_file"
}

# Print final summary
print_final_summary() {
    local workflow_dir="$1"

    log_step "WORKFLOW SUMMARY"

    echo "Configuration:"
    echo "  Server: $SERVER"
    echo "  Target TPS: $TARGET_TPS"
    echo "  Duration: $DURATION"
    echo "  Threshold: $THRESHOLD"
    echo ""

    echo "Results by Iteration:"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    printf "%-15s %12s %12s %12s %12s\n" "Iteration" "Achieved TPS" "Target %" "Success %" "P99 Lat (ms)"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

    # Baseline
    local baseline_report=$(ls -t "$workflow_dir"/baseline/report_*.json 2>/dev/null | head -1)
    if [ -n "$baseline_report" ]; then
        local metrics=$(extract_metrics "$baseline_report")
        read achievement tps success latency <<< "$metrics"
        local target_pct=$(echo "scale=2; $achievement * 100" | bc)
        printf "%-15s %12.2f %11.2f%% %11.2f%% %12.2f\n" "Baseline" "$tps" "$target_pct" "$success" "$latency"
    fi

    # Iterations
    for i in $(seq 1 $ITERATIONS); do
        local iter_report=$(ls -t "$workflow_dir"/iteration-$i/report_*.json 2>/dev/null | head -1)
        if [ -n "$iter_report" ]; then
            local metrics=$(extract_metrics "$iter_report")
            read achievement tps success latency <<< "$metrics"
            local target_pct=$(echo "scale=2; $achievement * 100" | bc)

            local verdict=""
            local iter_analysis="$workflow_dir/iteration-$i/analysis.json"
            if [ -f "$iter_analysis" ]; then
                verdict=$(jq -r '.baseline_comparison.verdict // ""' "$iter_analysis")
            fi

            local name="Iteration $i"
            if [ -n "$verdict" ]; then
                case $verdict in
                    IMPROVEMENT)
                        name="$name ✓"
                        ;;
                    REGRESSION)
                        name="$name ✗"
                        ;;
                    *)
                        name="$name ~"
                        ;;
                esac
            fi

            printf "%-15s %12.2f %11.2f%% %11.2f%% %12.2f\n" "$name" "$tps" "$target_pct" "$success" "$latency"
        fi
    done

    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""

    echo "Legend: ✓ = Improvement, ✗ = Regression, ~ = Neutral"
    echo ""

    echo "All results saved to: $workflow_dir"
    echo ""
}

# Main workflow
main() {
    log_step "STARTING AUTOMATED PERFORMANCE TUNING WORKFLOW"

    log_info "Configuration:"
    echo "  Server: $SERVER"
    echo "  Target TPS: $TARGET_TPS"
    echo "  Duration: $DURATION"
    echo "  Iterations: $ITERATIONS"
    echo "  Threshold: $THRESHOLD"
    echo "  Workflow Directory: $WORKFLOW_DIR"
    echo "  Auto-apply: $AUTO_APPLY"
    echo ""

    # Phase 1: Baseline
    log_step "PHASE 1: BASELINE MEASUREMENT"

    local baseline_dir="$WORKFLOW_DIR/baseline"
    mkdir -p "$baseline_dir"

    local baseline_report=$(run_perfmon "$baseline_dir" "Baseline")
    local baseline_recommendations="$baseline_dir/recommendations.json"

    analyze_report "$baseline_report" "$baseline_recommendations" ""

    local baseline_metrics=$(extract_metrics "$baseline_report")
    read baseline_achievement baseline_tps baseline_success baseline_latency <<< "$baseline_metrics"

    log_info "Baseline Results:"
    echo "  Achieved TPS: $baseline_tps"
    echo "  Target Achievement: $(echo "scale=2; $baseline_achievement * 100" | bc)%"
    echo "  Success Rate: $baseline_success%"
    echo "  P99 Latency: $baseline_latency ms"
    echo ""

    if check_target_met "$baseline_achievement"; then
        log_success "Baseline already meets target threshold!"
        log_info "Consider increasing target TPS or threshold for further optimization"
        generate_summary "$WORKFLOW_DIR"
        print_final_summary "$WORKFLOW_DIR"
        exit 0
    fi

    display_recommendations "$baseline_recommendations"

    # Phase 2: Iterative Tuning
    local current_achievement=$baseline_achievement
    local best_achievement=$baseline_achievement
    local best_iteration=0

    for iteration in $(seq 1 $ITERATIONS); do
        log_step "PHASE 2: ITERATION $iteration/$ITERATIONS"

        # Get recommendations from previous iteration
        local prev_recommendations
        if [ $iteration -eq 1 ]; then
            prev_recommendations="$baseline_recommendations"
        else
            prev_recommendations="$WORKFLOW_DIR/iteration-$((iteration-1))/recommendations.json"
        fi

        display_recommendations "$prev_recommendations"
        apply_tuning "$prev_recommendations"

        # Wait for system to stabilize
        if [ $iteration -gt 1 ]; then
            log_info "Waiting $WAIT_BETWEEN seconds for system to stabilize..."
            sleep "$WAIT_BETWEEN"
        fi

        # Run test
        local iter_dir="$WORKFLOW_DIR/iteration-$iteration"
        mkdir -p "$iter_dir"

        local iter_report=$(run_perfmon "$iter_dir" "Iteration $iteration")
        local iter_analysis="$iter_dir/analysis.json"
        local iter_recommendations="$iter_dir/recommendations.json"

        # Analyze with baseline comparison
        analyze_report "$iter_report" "$iter_analysis" "$baseline_report"

        # Extract recommendations for next iteration
        "$DEBUG_CMD" perfmon-tuner "$iter_report" --output "$iter_recommendations" --threshold "$THRESHOLD"

        # Get metrics
        local iter_metrics=$(extract_metrics "$iter_report")
        read current_achievement iter_tps iter_success iter_latency <<< "$iter_metrics"

        log_info "Iteration $iteration Results:"
        echo "  Achieved TPS: $iter_tps"
        echo "  Target Achievement: $(echo "scale=2; $current_achievement * 100" | bc)%"
        echo "  Success Rate: $iter_success%"
        echo "  P99 Latency: $iter_latency ms"
        echo ""

        # Track best iteration
        if (( $(echo "$current_achievement > $best_achievement" | bc -l) )); then
            best_achievement=$current_achievement
            best_iteration=$iteration
            log_success "New best performance achieved!"
        fi

        # Check if target is met
        if check_target_met "$current_achievement"; then
            log_success "Target threshold met in iteration $iteration!"
            log_success "Achievement: $(echo "scale=2; $current_achievement * 100" | bc)%"
            break
        fi

        # Check for regression
        local verdict=$(jq -r '.baseline_comparison.verdict // ""' "$iter_analysis")
        if [ "$verdict" == "REGRESSION" ]; then
            log_warning "Performance regression detected"
            log_warning "Consider reverting last changes"
        fi
    done

    # Phase 3: Summary
    log_step "PHASE 3: FINAL SUMMARY"

    generate_summary "$WORKFLOW_DIR"
    print_final_summary "$WORKFLOW_DIR"

    if check_target_met "$current_achievement"; then
        log_success "Workflow completed successfully!"
        log_success "Target threshold of $(echo "scale=0; $THRESHOLD * 100" | bc)% has been met"
    else
        log_warning "Workflow completed but target threshold not met"
        log_info "Best performance: $(echo "scale=2; $best_achievement * 100" | bc)% (Iteration $best_iteration)"
        log_info "Consider:"
        echo "  - Running additional iterations"
        echo "  - Reviewing hardware resources"
        echo "  - Adjusting target TPS"
        echo "  - Lowering threshold"
    fi
}

# Execute main workflow
main
