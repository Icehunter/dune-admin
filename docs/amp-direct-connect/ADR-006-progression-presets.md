# ADR-006: Progression presets

**Status:** accepted
**Date:** 2026-05-23

## Context

Dune: Awakening's progression system is structured as a tree of "journey
nodes" — quests, sub-quests, and tutorial steps — stored in the
`dune.journey_story_node` table. Completing a node:

1. Marks the node and (recursively) all of its child nodes complete.
2. Applies any **gameplay tags** mapped to that node, via the
   `tags-data.json` file shipped with the repo.

`dune-admin` already has an endpoint to complete a single journey node by
ID (`cmdCompleteJourneyNode`), with the cascade-to-children and tag
application built in. But the IDs aren't memorable
(`DA_MQ_FindTheFremen`, `DA_SQ_VermiliusGap`, etc.), and a typical
operator wants to do things like "skip the new-player tutorial" or
"unlock all the Dunipedia lore" — which require completing the *right
set* of nodes, not just one.

Without a higher-level abstraction, an operator has to either:

- Know the journey-node ID strings and the right root for each
  conceptual milestone, or
- Click through the journey tree picking nodes one at a time, hoping
  they hit the right set.

Both are awful UX for a self-hosted server admin who just wants their
player base to start at chapter 2.

## Decision

Introduce **progression presets**: a curated, in-code catalog of named
bundles, each pointing to one or more "root" journey nodes whose
existing cascade behavior is sufficient to fulfill the bundle's intent.

Concretely:

- A preset is `{id, name, description, node_count, nodes[]}` — the
  `node_count` is a UI hint for how much will change.
- Applying a preset iterates its `nodes` and calls the existing
  `cmdCompleteJourneyNode` for each. **No new completion logic is added** —
  presets are pure orchestration over the existing primitive.
- The catalog lives in code (`progressionPresets` slice in
  `progression_presets.go`), not in the database. This is deliberate:
  presets are content authored against a specific understanding of the
  game's journey tree, and they need to be reviewed and updated
  alongside game-version bumps.

Two endpoints expose the feature:

- `GET /api/v1/progression/presets` — returns the catalog.
- `POST /api/v1/players/progression/apply-preset` with `{account_id,
  preset_id}` — applies the preset to a player.

The frontend renders the catalog as a list of buttons in the Player
detail view; clicking a button posts the apply request and shows the
returned summary message.

## Why this and not X

- **Why a curated catalog and not user-defined presets?** Authoring a
  preset requires knowing which journey nodes are valid roots, which
  ones cascade cleanly, and which ones cause unintended state. That's
  not a knob a server admin should be expected to twiddle. A
  user-defined-preset feature is a possible future extension once the
  curated set has stabilized.
- **Why store the catalog in code rather than in the DB?** The catalog
  is part of the deployment, not the game state. Storing it in code
  means a redeploy ships the right version; storing it in the DB would
  let a stale catalog drift past a game update silently.
- **Why reuse `cmdCompleteJourneyNode` rather than build a bulk
  completion path?** Because all the hard work — the recursive cascade,
  the tag application, the JSONB state mutation — is already there and
  tested. Presets are *literally* a loop over `cmdCompleteJourneyNode`.
  Don't duplicate it.
- **Why does the summary message report counts?** So an operator can
  sanity-check that a preset actually did what they expected. "Applied
  preset 'Skip NPE': 4 node(s), 0 tag(s)" tells them what changed; an
  unhelpfully terse "OK" wouldn't.

## Preserve when re-implementing

1. **Presets must apply atomically per node** — each
   `cmdCompleteJourneyNode` call is its own transaction; partial
   completion is acceptable if a later node in the preset fails. The
   summary message must reflect actual counts.
2. **The catalog must not depend on the player.** A preset is a
   server-wide notion ("Act 1 complete"); it must produce equivalent
   results no matter which account it's applied to. Account-scoped
   presets would be a different feature.
3. **The "takes effect on next login" wording** in the summary message
   must remain accurate — DB-side completion does not push state to a
   running game session. If a future change makes presets push live
   updates (see RabbitMQ research notes), the wording must update too.
4. **The preset endpoint surface must be authenticated like the rest of
   the player-mutation endpoints.** Anyone who can apply a preset can
   skip arbitrary amounts of progression on any account; this is
   admin-only by design.
5. **Each preset's `nodes` list must contain root IDs**, not internal
   nodes. The cascade does the rest. Pointing a preset at a non-root
   node will partially apply at best and produce mismatched tags at
   worst.

## Implementation-incidental

- The string-parsing of the inner success message
  (`parseCompletionCounts`) is brittle. If `cmdCompleteJourneyNode`
  returned structured counts instead of a formatted string, this code
  would disappear. That refactor is fine to take.
- The JSON shape of the catalog (`node_count` field, `nodes` field
  name) is a contract with the current frontend. Renames need to be
  coordinated.
- The frontend rendering — buttons in a panel, one fetch on tab open —
  is the simplest possible UI. A richer UI (search, categories,
  preview-before-apply) is welcome but not required.
- The specific nine presets shipped today were chosen to cover the
  Act 1 / tutorial / lore use cases that came up in practice. Adding,
  removing, or renaming presets is expected as the game evolves.

## Related

- [ADR-001](ADR-001-direct-connect-mode.md) — this feature works in any
  mode, but in direct mode it composes naturally with the other AMP
  affordances (especially backup-and-restore workflows).
- See `tags-data.json` and `cmdCompleteJourneyNode` in the upstream
  journey logic — presets are a thin layer on top.
