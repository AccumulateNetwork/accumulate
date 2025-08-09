#!/bin/bash

# Script to create and push a new version of Accumulate Protocol
# Version: 1.5.0 - CrossChainConductor Release

set -e

echo "================================================"
echo "  Accumulate Protocol v1.5.0 Release Process"
echo "================================================"
echo ""

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Configuration
VERSION="v1.5.0-experimental"
BRANCH_NAME="3653-add-a-crosschainconductor-process-for-coordinating-partitions"

echo -e "${BLUE}Current Status:${NC}"
git status --short
echo ""

echo -e "${BLUE}Recent Commits:${NC}"
git log --oneline -n 5
echo ""

# Step 1: Ensure we're on the right branch
echo -e "${YELLOW}Step 1: Verifying branch...${NC}"
CURRENT_BRANCH=$(git branch --show-current)
if [ "$CURRENT_BRANCH" != "$BRANCH_NAME" ]; then
    echo -e "${RED}Error: Not on the expected branch${NC}"
    echo "Expected: $BRANCH_NAME"
    echo "Current: $CURRENT_BRANCH"
    exit 1
fi
echo -e "${GREEN}✓ On correct branch: $BRANCH_NAME${NC}"
echo ""

# Step 2: Push the branch to GitLab
echo -e "${YELLOW}Step 2: Push branch to GitLab?${NC}"
echo "This will push all commits to the remote branch."
read -p "Continue? (y/n): " -n 1 -r
echo ""
if [[ $REPLY =~ ^[Yy]$ ]]; then
    git push origin $BRANCH_NAME
    echo -e "${GREEN}✓ Branch pushed to GitLab${NC}"
else
    echo -e "${YELLOW}Skipping push${NC}"
fi
echo ""

# Step 3: Create a Merge Request
echo -e "${YELLOW}Step 3: Create Merge Request${NC}"
echo "To create a merge request on GitLab:"
echo "1. Go to: https://gitlab.com/accumulatenetwork/accumulate/-/merge_requests/new"
echo "2. Source branch: $BRANCH_NAME"
echo "3. Target branch: main"
echo "4. Title: 'feat: CrossChainConductor with ProofService optimization (v1.5.0)'"
echo "5. Description: Copy content from RELEASE_NOTES_v1.5.0.md"
echo ""
echo "Press any key when MR is created..."
read -n 1 -s
echo ""

# Step 4: After merge, create a tag
echo -e "${YELLOW}Step 4: Create Release Tag${NC}"
echo "After the merge request is approved and merged:"
echo ""
echo "# Checkout main and pull latest"
echo -e "${BLUE}git checkout main${NC}"
echo -e "${BLUE}git pull origin main${NC}"
echo ""
echo "# Create and push tag"
echo -e "${BLUE}git tag -a $VERSION -m \"Release $VERSION: CrossChainConductor with ProofService\"${NC}"
echo -e "${BLUE}git push origin $VERSION${NC}"
echo ""

# Step 5: Create GitLab Release
echo -e "${YELLOW}Step 5: Create GitLab Release${NC}"
echo "To create a release on GitLab:"
echo "1. Go to: https://gitlab.com/accumulatenetwork/accumulate/-/releases/new"
echo "2. Tag name: $VERSION"
echo "3. Release title: 'Accumulate $VERSION - CrossChainConductor Release'"
echo "4. Release notes: Copy from RELEASE_NOTES_v1.5.0.md"
echo "5. Attach any binaries if needed"
echo ""

# Step 6: Alternative - Direct tag on branch
echo -e "${YELLOW}Alternative: Tag Current Branch${NC}"
echo "If you want to tag the current branch directly (without merging to main):"
read -p "Create tag $VERSION on current branch? (y/n): " -n 1 -r
echo ""
if [[ $REPLY =~ ^[Yy]$ ]]; then
    # Add release notes to commit
    git add RELEASE_NOTES_v1.5.0.md
    git commit -m "docs: Add release notes for v1.5.0" || echo "Release notes already committed"
    
    # Create annotated tag
    git tag -a $VERSION -m "Release $VERSION: CrossChainConductor with ProofService

Major Features:
- CrossChainConductor for orchestrating cross-partition transactions
- Centralized ProofService with collection proof optimization (13.2x speedup)
- Partition failure handling with circuit breaker pattern
- Visual monitoring and comprehensive metrics

Performance:
- 95% memory reduction for large batches
- 100% success rate in extended load testing
- 17.79 req/sec sustained throughput

See RELEASE_NOTES_v1.5.0.md for full details."
    
    echo -e "${GREEN}✓ Tag $VERSION created${NC}"
    
    read -p "Push tag to GitLab? (y/n): " -n 1 -r
    echo ""
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        git push origin $VERSION
        echo -e "${GREEN}✓ Tag pushed to GitLab${NC}"
        echo ""
        echo "View at: https://gitlab.com/accumulatenetwork/accumulate/-/tags/$VERSION"
    fi
else
    echo -e "${YELLOW}Skipping tag creation${NC}"
fi

echo ""
echo -e "${GREEN}================================================${NC}"
echo -e "${GREEN}  Release Process Instructions Complete${NC}"
echo -e "${GREEN}================================================${NC}"
echo ""
echo "Summary of changes in $VERSION:"
echo "- CrossChainConductor implementation"
echo "- ProofService with 13.2x performance improvement"
echo "- Collection proof optimization (95% memory reduction)"
echo "- Partition failure handling"
echo "- Visual monitoring tools"
echo ""
echo "Next steps:"
echo "1. Create merge request (if not done)"
echo "2. Get code review and approval"
echo "3. Merge to main"
echo "4. Create release on GitLab"
echo "5. Announce release to community"