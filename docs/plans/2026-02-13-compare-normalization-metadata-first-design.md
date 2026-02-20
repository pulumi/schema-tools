# Compare Normalization Metadata-First ExecPlan

Status: Revised for implementation alignment on 2026-02-19
Primary epic: `st-ac3`
Primary slice: `st-ac3.7` and children `st-ac3.7.1`..`st-ac3.7.5`
Canonical plan artifact: this document

## Purpose / Big Picture

Compare currently relies on direct token matching and heuristic rename/maxItemsOne logic. For bridged providers, that can miss true equivalence or suppress real breaks because the authoritative rename/maxItemsOne history lives in `bridge-metadata.json`.

This work adds a metadata-first normalization layer so compare decisions prefer persisted bridge metadata, with strict metadata requirements for remote compare flows.

Affected users/systems:
- `schema-tools compare` maintainers and reviewers for issue #84.
- Provider teams using compare output to assess compatibility.
- CI/reporting consumers relying on stable break detection semantics.

## Current State (Observed in This Repo)

- Compare engine lives in `internal/compare`, called by `internal/cmd/compare.go`.
- There is no `pkg/compare` package in this workspace.
- Existing `st-ac3.7.x` tasks contain useful intent but some stale file paths (`pkg/compare/...`) and missing explicit `PlanRef` linkage.
- `docs/templates/gastown-execplan.md` is not present in this repository; this file is updated in place as the canonical ExecPlan.

## Scope and Non-Goals

In scope for `st-ac3.7`:
- Build pure normalization core in `internal/normalize`.
- Parse and validate metadata file shape for the `auto-aliasing` contract.
- Resolve token equivalence and maxItemsOne evidence with deterministic precedence.
- Integrate normalization seam into existing compare flow, including explicit rename and maxItemsOne change categories.
- Enforce strict metadata presence for remote compare flows.

Out of scope for `st-ac3.7`:
- Metadata retrieval adapters (GitHub/provider/local), provider process invocation, or network IO.
- New CLI source-selection UX/flags.
- Surfacing normalization diagnostics in compare output (deferred).

## Architecture / Approach Decision

Decision: implement a pure `internal/normalize` package that receives old/new schema plus old/new metadata payloads in strict mode, then returns normalized schemas and normalization change metadata consumed by compare.

Rationale:
- Keeps normalization testable and deterministic.
- Preserves existing CLI/retrieval flow boundaries until `st-ac3.8+`.
- Keeps execution policy explicit (strict metadata gating for remote flow), not implicit IO behavior.

### Authority Split (Mandatory)

- File-shape authority: local types for the complete `bridge-metadata.json` auto-aliasing payload.
- Semantic authority: reuse bridge semantics/helpers where they are exported and practical.

Why:
- Bridge code does not export one top-level full-file struct for `bridge-metadata.json`.
- Bridge code does define behavior we should not fork when callable.

Guardrails:
1. Do not invent alternative semantic rules when bridge behavior/helper already exists.
2. If helper coverage is missing for a required decision path, stop and escalate.
3. Use parity fixtures from bridge-produced metadata payloads to detect drift.

### Metadata Shape Contract (must decode as-is)

`auto-aliasing` key with:
- `resources`: map Terraform token -> token history
- `datasources`: map Terraform token -> token history

Token history:
- `current`
- `past[]` with `name`, `inCodegen`, `majorVersion`
- `majorVersion`
- `fields`

Field history:
- `maxItemsOne` (`nil` = unknown)
- recursive `fields`
- recursive `elem`

Unknown JSON fields must be tolerated (forward compatibility).

### Resolver Policy (fixed precedence)

1. Metadata-derived mapping/evidence.
2. Deterministic legacy heuristic fallback only inside resolver decision points when metadata evidence is incomplete.

Safety:
- No silent suppression on ambiguous mapping.
- Emit diagnostics when fallback or ambiguity occurs (capture now, output surfacing deferred).

Mode behavior:
- Strict-only remote mode: missing required metadata is typed failure.
- Mixed/one-sided mode is explicitly deferred.

## Plan of Work (PlanRef Seeds)

### WP-A10 (`st-ac3.7.1`) Metadata Contract + Loader

Outcome:
- Parse/validate metadata payload shape and version envelope.

Primary files:
- `internal/normalize/types.go`
- `internal/normalize/loader.go`
- `internal/normalize/types_test.go`
- `internal/normalize/loader_test.go`
- `internal/normalize/testdata/metadata/*.json`

Primary symbols/contracts:
- `type MetadataEnvelope`
- `type AutoAliasing`
- typed errors: `ErrMetadataRequired`, `ErrMetadataInvalid`, `ErrMetadataVersionUnsupported`
- loader entrypoint returning typed metadata + validation diagnostics/errors

### WP-A20 (`st-ac3.7.2`) Token Remap Resolver

Outcome:
- Deterministic canonical token mapping across old/new using metadata current/past history.

Primary files:
- `internal/normalize/token_remap.go`
- `internal/normalize/token_remap_test.go`
- `internal/normalize/testdata/remap/*.json`

Primary symbols/contracts:
- canonical map structure (old/new -> canonical token)
- conflict/cycle detection surfaced as diagnostics

### WP-A30 (`st-ac3.7.3`) Field History Resolver

Outcome:
- Flatten and compare maxItemsOne history evidence for resolver policy consumption.

Primary files:
- `internal/normalize/field_history.go`
- `internal/normalize/property_renames.go` (only if required by flattening/compat helpers)
- `internal/normalize/field_history_test.go`
- `internal/normalize/testdata/maxitems/*.json`

Primary symbols/contracts:
- deterministic flattened path representation
- transition classification (`changed`, `unchanged`, `unknown`)

### WP-A40 (`st-ac3.7.4`) Policy + Diagnostics

Outcome:
- One resolver implementing strict-mode precedence and diagnostics emission.

Primary files:
- `internal/normalize/resolver.go`
- `internal/normalize/diagnostics.go`
- `internal/normalize/resolver_test.go`

Primary symbols/contracts:
- strict resolver options/behavior
- precedence implementation
- diagnostic categories:
  - `ambiguous_mapping`
  - `heuristic_fallback_used`
  - `metadata_schema_conflict`
  - `metadata_missing_strict`
  - `token_conflict`
  - `token_chain_cycle`
  - `field_path_ambiguous`
  - `field_history_incomplete`

### WP-A50 (`st-ac3.7.5`) Compare Integration Seam

Outcome:
- Existing compare flow consumes normalized schemas in remote strict flow.
- Normalization-specific breaking changes are surfaced as explicit categories instead of remove/add churn.

Primary files:
- `internal/normalize/normalize.go`
- `internal/cmd/compare.go`
- `internal/normalize/normalize_test.go`
- `internal/compare/engine_test.go` (or other compare tests covering integration behavior)

Primary symbols/contracts:
- normalization orchestrator for old/new schema + strict metadata
- compare call path integration using normalized output in remote strict flow
- explicit normalization categories:
  - `renamed-resource`
  - `renamed-function`
  - `max-items-one-changed`

## Decomposition Constraints for Bead Planning

Task slice constraints:
- One independently verifiable behavioral outcome per task.
- No task should require unfinished future tasks to prove correctness.
- Avoid mixed concerns (parser + resolver + CLI in one task).

Verification expectation per slice:
- Every task must have one explicit command and one explicit success signal.
- Prefer narrow package tests first, then broaden.

Required review gates per task (mandatory):
1. Spec compliance gate: implementation matches this ExecPlan + task scope.
2. Code quality gate: tests, clarity, error behavior, and regression risk.
3. Independent verification gate: command rerun with expected signal.

## Validation and Acceptance (Observable)

Acceptance checklist:
- [ ] Metadata fixtures decode into stable local structs preserving key/optionality semantics.
- [ ] Unsupported/invalid metadata yields typed failures.
- [ ] Token remap handles no-rename, rename, and multi-hop histories deterministically.
- [ ] maxItemsOne field history flattening handles nested `fields`/`elem` and nil/true/false transitions.
- [ ] Resolver precedence is metadata-first with deterministic fallback behavior, test-covered.
- [ ] Remote strict mode fails when required metadata is missing.
- [ ] Compare output surfaces normalization breaking changes as `renamed-resource`, `renamed-function`, and `max-items-one-changed`.
- [ ] Local compare flow remains unchanged.

Verification command baseline:
- `go test ./internal/normalize`
- `go test ./internal/compare`
- `go test ./...`

## Risks and Escalation Triggers

Risks:
- Bridge helper coverage may be incomplete for direct reuse in this repo.
- Metadata shape drift can break decoding assumptions.
- Ambiguous rename chains can lead to accidental suppression if poorly handled.
- Stale task specs can cause wrong-file implementation churn.

Escalate immediately when:
1. Required bridge semantic helper is unavailable or incompatible with current dependencies.
2. Real bridge metadata fixtures violate assumed shape/optionality.
3. Ambiguity cannot be resolved without speculative behavior.
4. A task requires source retrieval/CLI flag work before `st-ac3.8`.
5. Planned file/symbol scope does not match actual repository layout.

## Branch and Ownership Intent

Planning/design workspace branch:
- `design/st-ac3.7-metadata-first-execplan` (created in this workspace)

Implementation branch policy (for next skill):
- Planning must create and use epic integration branch via:
  - `gt mq integration create <epic-id>`
  - `gt mq integration status <epic-id>`
- Do not execute implementation on `main`/`master`.

Crew/worktree ownership intent:
- This crew workspace owns `st-ac3.7` planning and subsequent wave dispatch.
- Keep unrelated implementation efforts in separate worktrees.

## Handoff Notes for `codex-crew-plan-beads`

Task-quality corrections required while updating beads:
1. Add `PlanRef` to each child task (`WP-A10`..`WP-A50`).
2. Replace stale path references (`pkg/compare/...`) with actual repo paths.
3. Ensure each task lists concrete symbols/contracts, execution steps, non-goals, and escalation trigger.
4. Preserve dependency chain:
   - `WP-A10` blocks `WP-A20` and `WP-A30`
   - `WP-A20` and `WP-A30` block `WP-A40`
   - `WP-A40` blocks `WP-A50`
5. Add explicit review-gate contract text (spec compliance then code quality) to each task.

## Unresolved Questions

None at this time.
