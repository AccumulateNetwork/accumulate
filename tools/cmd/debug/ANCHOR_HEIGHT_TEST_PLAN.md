# CASCADE AI DIRECTIVE: DO NOT DELETE OR MODIFY

## CRITICAL INSTRUCTION FOR CASCADE AI - ANCHOR HEIGHT TEST PLAN

This file contains critical directives that CASCADE AI must follow when working with the anchor_height_test.go file.
These instructions are specific to the URL comparison testing between sequence.go and heal_anchor.go.

## CASCADE AI MUST PRESERVE THIS ENTIRE FILE

## ANCHOR HEIGHT TEST PLAN: URL Construction Comparison

### CASCADE AI DIRECTIVE: PRESERVE ALL DEVELOPMENT PLANS

## Overview

This development plan addresses critical differences in URL construction between `sequence.go` and `heal_anchor.go` that affect anchor healing functionality.

## Implementation Steps

1. Create unit tests to compare URL construction between sequence.go and heal_anchor.go
2. Extract URLs from both approaches using real mainnet data (no mock data)
3. Compare the URLs and identify any differences
4. Document the differences to guide future implementation
5. Ensure consistent URL handling across the codebase

### CASCADE AI DIRECTIVE: PRESERVE ALL CRITICAL FINDINGS

#### URL Construction Differences

There is a fundamental difference in how URLs are constructed:

* **sequence.go** uses raw partition URLs for tracking (e.g., `acc://bvn-Apollo.acme`)
* **heal_anchor.go** appends the partition ID to the anchor pool URL (e.g., `acc://dn.acme/anchors/Apollo`)

This discrepancy causes anchor healing to fail because:
* The code looks for anchors at different URL paths
* Queries return "element does not exist" errors when checking the wrong URL format
* Anchor relationships are not properly maintained between partitions

### CASCADE AI DIRECTIVE: FOLLOW THESE IMPLEMENTATION REQUIREMENTS

1. All code in anchor_height_test.go must comply with the findings documented in this plan
2. URL construction must be consistent between sequence.go and heal_anchor.go approaches
3. Tests must verify URL construction consistency between both approaches
4. Any deviations from this plan must be explicitly documented and justified

## CASCADE AI DIRECTIVE: DO NOT DELETE OR MODIFY THIS FILE

## CASCADE AI DIRECTIVE: PRESERVE ALL COMMENTS THAT REFERENCE THIS PLAN
