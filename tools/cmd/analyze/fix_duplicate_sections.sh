#!/bin/bash

# Create a backup of the original file
cp /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/tools/cmd/analyze/SNAPSHOT_FORMAT.md /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/tools/cmd/analyze/SNAPSHOT_FORMAT.md.bak2

# Find the line numbers of the two "Snapshot Struct Reference" sections
FIRST_SECTION=$(grep -n "^## Snapshot Struct Reference" /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/tools/cmd/analyze/SNAPSHOT_FORMAT.md | head -1 | cut -d: -f1)
SECOND_SECTION=$(grep -n "^## Snapshot Struct Reference" /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/tools/cmd/analyze/SNAPSHOT_FORMAT.md | tail -1 | cut -d: -f1)

# If there are two sections, remove the first one
if [ "$FIRST_SECTION" != "$SECOND_SECTION" ]; then
  # Find the end of the first section (where the next ## heading starts)
  NEXT_SECTION=$(tail -n +$((FIRST_SECTION+1)) /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/tools/cmd/analyze/SNAPSHOT_FORMAT.md | grep -n "^##[^#]" | head -1 | cut -d: -f1)
  NEXT_SECTION=$((FIRST_SECTION + NEXT_SECTION))
  
  # Create a temporary file with content before the first section
  head -n $((FIRST_SECTION-1)) /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/tools/cmd/analyze/SNAPSHOT_FORMAT.md > /tmp/before_first_section.md
  
  # Create a temporary file with content after the first section
  tail -n +$NEXT_SECTION /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/tools/cmd/analyze/SNAPSHOT_FORMAT.md > /tmp/after_first_section.md
  
  # Combine the files to create the updated document
  cat /tmp/before_first_section.md /tmp/after_first_section.md > /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/tools/cmd/analyze/SNAPSHOT_FORMAT.md
  
  # Clean up temporary files
  rm /tmp/before_first_section.md /tmp/after_first_section.md
  
  echo "Duplicate 'Snapshot Struct Reference' section has been removed."
else
  echo "No duplicate sections found."
fi
