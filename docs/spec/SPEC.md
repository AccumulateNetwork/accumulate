# Accumulate — Specification

This is the normative specification for Accumulate. It describes the system as
it **is**, not as it is intended to become. Where the two differ, the
difference is a defect and is filed as an issue — never papered over here.

The specification is split by subsystem because it is large. Each part carries
the same two sections:

1. **Architecture — what we are doing.** Purpose, surfaces, objects,
   invariants. Readable without the code.
2. **Specification — how it is implemented.** Layout, components, interfaces,
   data flows. Every architectural claim in section 1 traces to a mechanism in
   section 2.

## Parts

| Part | Covers |
|---|---|
| [Executor](executor.md) | How a block is produced: what runs, in what order, and what may be staged |
| [Database abstraction](database.md) | The storage contract every backend satisfies, and how a backend is chosen |
| [Healing](healing.md) | Filling gaps in sequenced cross-partition streams |

Parts still to be written: consensus (DAG-BFT), the protocol's account and
transaction model, the API, and cross-partition messaging (synthetics and
anchors). A missing part is a gap, not a statement that the subsystem has no
rules.

## Working against the spec

- **All work is done against the spec.** Read the relevant section first. A
  change that contradicts the spec means fixing the spec or the plan first.
- **New work requires updating the spec** — in the same change set, not later.
- **Gaps found while writing the spec are filed as issues, not silently
  fixed.** A spec that quietly documents the fix rather than the behaviour is
  fiction.

See accumulatenetwork/accumulate#4178 and the
[standard](https://github.com/PaulSnow/tracking_repo/blob/main/docs/standards/spec-driven-development.md).
