# Chains

### Anchoring

```plantuml
@startuml
left to right direction
hide empty description

state "BVN (block N)" as BVN1 {
  state "Main Chain" as Main {
    state "Entry" as TxnEntry
    state "Anchor" as MainAnchor
  }

  state "Root Chain" as BvnRoot {
    state "Entry" as BvnRootEntry
    state "Anchor" as BvnRootAnchor
  }

  state "Main Index" as MainIndex #white
  state "Root Index" as BvnRootIndex #white
}

state "BVN (block N+1)" as BVN2 {
  state "Anchor" as BvnAnchorTxn #line.bold {
    state "Root" as BvnRootAnchorHash
  }

  state "Anchor Sequence\nChain" as AnchorSeq {
    state "Entry" as AnchorSeqEntry
  }
}

state DN {
  state "Anchor Chain\n(BVN Root)" as DnBvnRoot {
    state "Entry" as DnBvnRootEntry
    state "Anchor" as DnBvnRootAnchor
  }

  state "Root Chain" as DnRoot {
    state "Entry" as DnRootEntry
    state "Anchor" as DnRootAnchor
  }

  state "Anchor Index\n(BVN Root)" as DnBvnRootIndex #white
  state "Root Index" as DnRootIndex #white
}

[*] --> TxnEntry
TxnEntry -[dashed]-> MainAnchor
MainAnchor --> BvnRootEntry
BvnRootEntry -[dashed]-> BvnRootAnchor

MainAnchor -[dotted]-> MainIndex
BvnRootEntry -[dotted]-> MainIndex
BvnRootAnchor -[dotted]-> BvnRootIndex

BvnAnchorTxn --> AnchorSeqEntry

BvnRootAnchor --> BvnRootAnchorHash
BvnRootAnchorHash --> DnBvnRootEntry
DnBvnRootEntry -[dashed]-> DnBvnRootAnchor
DnBvnRootAnchor --> DnRootEntry
DnRootEntry -[dashed]-> DnRootAnchor

DnBvnRootAnchor -[dotted]-> DnBvnRootIndex
DnRootEntry -[dotted]-> DnBvnRootIndex
DnRootAnchor -[dotted]-> DnRootIndex

@enduml
```

Dashed arrows between a chain entry and anchor indicate that (1) more than one
entry may be added but only one anchor is created per block and (2) the anchor
is a hash calculated from all the entries up to that point.

Dotted arrows between a chain entry or anchor indicate that the *index* of the
entry or anchor is added to the index chain, not the hash.