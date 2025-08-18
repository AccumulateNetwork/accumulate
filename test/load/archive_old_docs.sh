#!/bin/bash

# Archive old documentation files
# This script moves redundant documentation to an archive folder
# while preserving the consolidated guide

echo "=== Archiving Old Documentation ==="
echo "Creating archive directory..."

# Create archive directory with timestamp
ARCHIVE_DIR="docs_archive_$(date +%Y%m%d_%H%M%S)"
mkdir -p "$ARCHIVE_DIR"

echo "Archive directory: $ARCHIVE_DIR"

# Files to keep (DO NOT ARCHIVE)
KEEP_FILES=(
    "CONSOLIDATED_DOCS.md"      # New consolidated guide
    "README.md"                 # Primary readme
    "LOAD_TEST_GUIDE.md"        # Essential reference
    "TPS_PERFORMANCE_REPORT.md" # Performance baseline
)

# Function to check if file should be kept
should_keep() {
    local file="$1"
    for keep in "${KEEP_FILES[@]}"; do
        if [[ "$file" == "$keep" ]]; then
            return 0
        fi
    done
    return 1
}

# Archive old documentation
echo "Archiving documentation files..."
count=0
for file in *.md; do
    if [[ -f "$file" ]]; then
        if should_keep "$file"; then
            echo "  ✅ Keeping: $file"
        else
            echo "  📦 Archiving: $file"
            mv "$file" "$ARCHIVE_DIR/"
            ((count++))
        fi
    fi
done

echo ""
echo "=== Archive Summary ==="
echo "Files archived: $count"
echo "Files kept: ${#KEEP_FILES[@]}"
echo "Archive location: $ARCHIVE_DIR"
echo ""

# Create index of archived files
echo "Creating archive index..."
cat > "$ARCHIVE_DIR/INDEX.md" << 'EOF'
# Archived Documentation Index

This directory contains documentation that has been consolidated into CONSOLIDATED_DOCS.md

## Archive Date
EOF
echo "$(date)" >> "$ARCHIVE_DIR/INDEX.md"

cat >> "$ARCHIVE_DIR/INDEX.md" << 'EOF'

## Archived Files

### CrossChain Documentation
- CrossChainConductor_Design_Document.md - Detailed design (618 lines)
- CrossChainConductor_Code_Reference.md - Code walkthrough (421 lines)
- PROOF_CENTRALIZATION_DESIGN.md - Proof system design
- PROOF_CENTRALIZATION_DESIGN_NO_CACHE.md - No-cache variant
- PARTITION_FAILURE_DESIGN.md - Failure handling

### Load Test Documentation  
- sl_design.md - Streamlined test design
- sl_long_running_design.md - Long-running tests
- sl_README.md - Streamlined test readme
- TEST_USAGE.md - Test usage guide
- RUN_INSTRUCTIONS.md - Running instructions
- HOW_TO_RUN_VISUAL_TESTS.md - Visual test guide

### API & Connection Documentation
- v3_connection_fixes.md - V3 connection fixes
- APPLY_V3_FIXES.md - How to apply fixes
- API_Improvements_TODO.md - API improvements

### Code Reviews
- CODE_REVIEW_COLLECTION_PROOFS.md - Collection proof review
- CODE_REVIEW_FINDINGS.md - General findings
- FINAL_REVIEW_SUMMARY.md - Final review

### Project Documentation
- COMPLETE_PROJECT_DOCUMENTATION.md - Full project docs
- README_COMPLETE.md - Complete readme
- AI_ASSISTANT_GUIDE.md - AI guide
- CHANGES_SINCE_LAST_COMMIT.md - Change tracking

### Configuration & Setup
- DEVNET_CONFIGURATION.md - DevNet config
- DISCOVERY_DEMO.md - Discovery demo
- REPOSITORY_CLEANUP_PLAN.md - Cleanup plan

## Consolidation

All essential information from these files has been consolidated into:
- **CONSOLIDATED_DOCS.md** - Single comprehensive guide
- **README.md** - Quick start reference  
- **LOAD_TEST_GUIDE.md** - Test reference
- **TPS_PERFORMANCE_REPORT.md** - Performance baselines

## Recovery

If you need to restore any archived file:
```bash
cp docs_archive_*/FILENAME.md .
```
EOF

echo "Archive index created: $ARCHIVE_DIR/INDEX.md"
echo ""
echo "=== Cleanup Complete ==="
echo ""
echo "Recommended next steps:"
echo "1. Review CONSOLIDATED_DOCS.md for all documentation"
echo "2. Check $ARCHIVE_DIR if you need original files"
echo "3. Consider adding $ARCHIVE_DIR to .gitignore"
echo "4. Commit the consolidation:"
echo "   git add CONSOLIDATED_DOCS.md"
echo "   git add -u  # Stage deletions"
echo "   git commit -m 'docs: consolidate test/load documentation'"
echo ""