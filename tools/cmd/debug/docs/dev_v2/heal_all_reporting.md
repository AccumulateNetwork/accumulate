# Heal-All Reporting Design

## Overview

This document outlines the design for the reporting functionality in the `heal-all` command. For a high-level overview of all enhancements, see the [Developer Guide](./developer_guide.md). For implementation details, see the [Implementation Guide](./heal_all_implementation.md).

**Implementation Status**: ✅ Fully implemented with enhancements

## Goals

1. **Comprehensive Status Tracking**: Track the healing status of all partition pairs in the network ✅
2. **Clear Visualization**: Present the data in a well-formatted table for easy interpretation ✅
3. **Actionable Insights**: Highlight areas that require attention ✅
4. **Integration with Existing Workflow**: Maintain compatibility with the controlled pacing mechanism ✅
5. **Minimal Performance Impact**: Ensure the reporting doesn't significantly impact the healing process ✅

## Reporting-Specific Enhancements

The reporting system includes the following key features:

1. **Comprehensive Status Tracking**: Track the healing status of all partition pairs in the network, as described in the [Data Structure](#data-structure) section below
2. **Pre-initialization of Report**: The report is pre-initialized with all possible partition pairs, ensuring all pairs appear in the report even if they don't need healing (see the [Collection Process](#collection-process) section)
3. **Enhanced Error Tracking**: Separate tracking for anchor and synthetic transaction errors through the `AnchorErrors` and `SynthErrors` fields
4. **Visual Status Indicators**: Color-coded status indicators for quick assessment, as detailed in the [Table Formatting](#table-formatting) section

For information about other enhancements like the Stable Node Submission approach, see the [Developer Guide](./developer_guide.md#recent-enhancements).

## Data Structure

### Healing Status Tracking

We will introduce a new data structure to track the healing status:

```go
// partitionPairStatus tracks the healing status between two partitions
type partitionPairStatus struct {
    Source      string // Source partition ID
    Destination string // Destination partition ID
    
    // Anchor healing status
    AnchorGaps  int    // Number of anchor gaps detected
    AnchorHealed int   // Number of anchors healed in this cycle
    
    // Synthetic transaction healing status
    SynthGaps   int    // Number of synthetic transaction gaps detected
    SynthHealed int    // Number of synthetic transactions healed in this cycle
    
    // Error tracking
    AnchorErrors int   // Number of errors encountered during anchor healing
    SynthErrors  int   // Number of errors encountered during synthetic healing
    
    // Last processed sequence numbers (for tracking progress)
    LastAnchorSequence uint64 // Last anchor sequence number processed
    LastSynthSequence  uint64 // Last synthetic sequence number processed
}

// healingReport contains the status for all partition pairs
type healingReport struct {
    Cycle            int                               // Current healing cycle number
    StartTime        time.Time                         // When this cycle started
    Duration         time.Duration                     // How long this cycle took
    PairStatuses     map[string]*partitionPairStatus   // Status for each partition pair (key is "source:destination")
    TotalHealed      int                               // Total number of transactions healed in this cycle
    TotalAnchorHealed int                              // Total number of anchors healed since start
    TotalSynthHealed  int                              // Total number of synthetic transactions healed since start
}
```

## Collection Process

The collection process is integrated into the main healing loop as described in the [Implementation Guide](./heal_all_implementation.md#healing-process):

1. Track cycle count and cumulative healing statistics across all cycles
2. At the start of each cycle, initialize a new `healingReport` with current cumulative totals
3. Pre-initialize the report with all possible partition pairs to ensure complete reporting
4. For each partition pair:
   - Process anchor gaps first, prioritizing the lowest sequence numbers
   - When checking for anchor gaps, update the `AnchorGaps` count
   - When healing an anchor, update the `AnchorHealed` count and increment the cumulative anchor count
   - After processing anchors, check for synthetic gaps
   - When checking for synthetic gaps, update the `SynthGaps` count
   - When healing a synthetic transaction, update the `SynthHealed` count and increment the cumulative synthetic count
   - Track any errors encountered during both anchor and synthetic processing
5. At the end of the cycle, calculate the total duration and generate the report
6. Pause before the next cycle based on whether healing was performed, using the controlled pacing mechanism described in the [Developer Guide](./developer_guide.md#controlled-pacing-implementation)

## Reporting Format

The reporting will use a table format with the following columns:

1. **Source**: Source partition ID
2. **Destination**: Destination partition ID
3. **Anchor Gaps**: Number of anchor gaps detected
4. **Anchor Healed**: Number of anchors healed in this cycle
5. **Synth Gaps**: Number of synthetic transaction gaps detected
6. **Synth Healed**: Number of synthetic transactions healed in this cycle
7. **Status**: Specific healing needs (OK, Anchors, Synth, Anchors/Synth, Error)

The report header will include:
1. **Cycle Number**: The current healing cycle
2. **Duration**: How long the cycle took to complete
3. **Cycle Healed**: Number of transactions healed in this cycle
4. **Total Healed**: Cumulative number of transactions healed since start
5. **Breakdown by Type**: Number of anchors and synthetic transactions healed

## Table Formatting

All tables in reports use the `github.com/olekukonko/tablewriter` package to ensure proper formatting and alignment, as recommended in the [Developer Guide](./developer_guide.md#tablewriter-usage-guidelines). This provides several advantages:

1. **Consistent Alignment**: All columns are properly aligned (centered by default)
2. **Clear Borders**: Table borders and separators make the data easy to read
3. **Color Coding**: Status information is color-coded for quick visual identification:
   - OK: Green
   - Anchors: Yellow
   - Synth: Cyan
   - Anchors/Synth: Bold Yellow
   - Error: Bold Red
4. **Formatted Headers**: Column headers are automatically formatted and aligned
5. **Summary Footer**: The totals row is formatted as a footer with bold text

### Implementation Notes for TableWriter

1. **Table Configuration**:
   ```go
   table := tablewriter.NewWriter(os.Stdout)
   table.SetHeader([...])
   table.SetBorder(false)
   table.SetColumnSeparator(" | ")
   table.SetCenterSeparator("+")
   table.SetHeaderAlignment(tablewriter.ALIGN_CENTER)
   table.SetAlignment(tablewriter.ALIGN_CENTER)
   ```

2. **Color Coding**:
   ```go
   table.Rich([]string{...}, []tablewriter.Colors{
       {},  // No color for column 1
       {},  // No color for column 2
       {},  // No color for column 3
       statusColor,  // Apply color to status column
   })
   ```

3. **Footer Configuration**:
   ```go
   table.SetFooter([...])
   table.SetFooterAlignment(tablewriter.ALIGN_CENTER)
   table.SetFooterColor([...])
   ```

Example output:

```
================================================================================
HEALING STATUS REPORT (Cycle 3)
Duration: 45.2s, Cycle Healed: 5, Total Healed: 14 (Anchors: 11, Synth: 3)

+------------+-------------+-------------+--------------+------------+-------------+---------------+
|   SOURCE   | DESTINATION | ANCHOR GAPS | ANCHOR HEALED | SYNTH GAPS | SYNTH HEALED |    STATUS     |
+------------+-------------+-------------+--------------+------------+-------------+---------------+
|     DN     | BVN-Apollo  |     12      |      2       |     0      |      0      |    Anchors    |
|     DN     | BVN-Europa  |      0      |      0       |     5      |      1      |     Synth     |
| BVN-Apollo |     DN      |      3      |      1       |     2      |      0      | Anchors/Synth |
| BVN-Apollo | BVN-Europa  |      0      |      0       |     0      |      0      |      OK       |
| BVN-Europa |     DN      |      8      |      1       |     3      |      1      | Anchors/Synth |
| BVN-Europa | BVN-Apollo  |      0      |      0       |     0      |      0      |      OK       |
+------------+-------------+-------------+--------------+------------+-------------+---------------+
|   TOTALS   |             |     23      |      4       |     10     |      2      |               |
+------------+-------------+-------------+--------------+------------+-------------+---------------+
================================================================================
```

Note: In the actual terminal output, the Status column will be color-coded for better visibility.

## Integration with Controlled Pacing

The reporting system will be integrated with the existing controlled pacing mechanism:

1. The report will be generated at the end of each healing cycle
2. The report will be displayed before the pause between cycles
3. The report will include information about the cycle duration and pause time

## Implementation Status

The reporting system has been fully implemented with the following components:

1. Data structures for tracking healing status (see [Data Structure](#data-structure) section)
2. Modified `healAll` function that collects data during each cycle
3. `generateHealingReport` function that creates the report
4. `displayHealingReport` function that formats and displays the report using tablewriter
5. Updated main healing loop that calls these functions at appropriate times

For the code implementation details, see the [Reporting System Implementation](./heal_all_implementation.md#reporting-system-implementation) section in the Implementation Guide.

## Integration with Other Components

### Integration with Stable Node Submission

The reporting system integrates with the stable node submission approach by:

1. **Tracking Routing Conflicts**: Reporting on conflicting routes errors to help diagnose URL construction issues
2. **Node Performance Metrics**: Providing insights into which nodes are performing well for submissions

For details on the stable node submission implementation, see the [Implementation Guide](./heal_all_implementation.md#stable-node-submission).

### Integration with Controlled Pacing

The reporting system works with the controlled pacing mechanism by:

1. **Cycle Tracking**: Displaying the current cycle number and duration
2. **Healing Activity**: Showing how many transactions were healed in each cycle
3. **Pause Indication**: Indicating when the system is pausing between cycles

For future enhancements to the reporting system and other components, see the [Future Enhancements](./developer_guide.md#future-enhancements) section in the Developer Guide.
5. **Additional Table Customization**: Further enhance table appearance with custom styles and formatting

## Conclusion

The reporting functionality will enhance the `heal-all` command by providing clear visibility into the healing status of the network. This will help operators identify and address issues more efficiently, improving the overall health of the Accumulate network.
