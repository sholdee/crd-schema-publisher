# Schema Row Layout Implementation Plan

Date: 2026-04-24
Related spec: `docs/superpowers/specs/2026-04-24-schema-row-layout-design.md`
Status: Planned

## Objective

Implement a stacked property-row layout in the schema renderer so long field names remain fully visible while descriptions and constraints stay readable across screen sizes and nesting depths.

## Scope

In scope:

- schema page property-row markup in `renderer/renderer.go`
- schema page CSS for row layout, wrapping behavior, nested spacing, and responsive adjustments
- renderer HTML tests that assert the new structure and preserve existing behavior

Out of scope:

- search behavior changes
- schema extraction or path metadata changes
- theme-wide styling changes outside what the renderer needs for this layout

## Plan

### 1. Refactor property markup to a shared header/body structure

Update the inline template in `renderer/renderer.go` so both expandable and leaf properties use the same internal structure:

- add a header container for field name, badges, and disclosure affordance
- add a body container for description and constraints
- keep existing `data-*` attributes on the row containers unchanged
- preserve `<details>/<summary>` for expandable nodes and a non-details container for leaf nodes

Implementation notes:

- expandable rows should keep `summary` as the interactive header region
- leaf rows should mirror the same DOM shape closely enough that one CSS layout system can style both
- the field name stays in a dedicated element so wrapping rules can be applied narrowly

### 2. Replace horizontal row layout with stacked layout rules

Adjust the schema page CSS in `renderer/renderer.go` to move from the current single-line flex row to a vertical card layout:

- let field names wrap naturally instead of forcing `white-space: nowrap`
- keep badges grouped in the header and allow them to wrap when space is tight
- make description and constraint blocks full-width under the header
- allow long tokens in description and constraint text to break safely

Implementation notes:

- keep the disclosure chevron visually stable when the title wraps
- avoid CSS that depends on name width and description width sharing one row
- ensure leaf and expandable rows share spacing, typography, and card treatment

### 3. Reduce nesting width pressure

Rework nested spacing so hierarchy stays clear without consuming as much horizontal room:

- reduce left padding in `.prop-content` and nested child containers
- add a subtle child-group rail or border to preserve depth cues
- add narrower-screen adjustments that compress nesting further

Implementation notes:

- maintain enough separation that deep trees remain scannable
- avoid mobile-only logic; the improved layout should work across screen sizes with responsive tuning

### 4. Update renderer tests for the new DOM shape

Extend `renderer/renderer_test.go` so the tests lock in the intended structure and guard against regressions:

- assert shared header/body markup for expandable and leaf rows
- assert long-name-friendly CSS hooks exist
- keep existing checks for search/path metadata and interactive wiring
- retain deep-nesting coverage and add structure-oriented assertions where useful

Implementation notes:

- prefer stable substrings that represent the intended structure over brittle full-template snapshots
- keep tests focused on behavior and layout contract, not incidental formatting

### 5. Verify rendering and responsiveness

Run renderer verification after the code change:

- `go test ./...`
- `go run ./cmd/ preview`

Manual verification in preview:

- desktop-width schema page with mixed leaf and expandable rows
- phone-width schema page with long field names
- deeply nested schema page to confirm reduced indentation pressure
- search and expand/collapse still behaving normally

## Expected File Changes

- `renderer/renderer.go`
- `renderer/renderer_test.go`

## Risks And Mitigations

### Risk: summary layout becomes awkward when names wrap

Mitigation:

- use an explicit header layout container inside `summary`
- keep the chevron separate from the wrapping text block

### Risk: reduced indentation makes depth harder to read

Mitigation:

- pair smaller padding with a child-group rail
- preserve card boundaries and disclosure affordances as hierarchy cues

### Risk: tests become too brittle

Mitigation:

- assert for key class names, containers, and content placement rather than exact HTML formatting

## Definition Of Done

The work is done when:

1. Long field names remain fully visible without forcing descriptions off-screen.
2. Descriptions and constraints render below the header and stay readable on narrow screens.
3. Deep nesting consumes less horizontal space than before while remaining legible.
4. Expandable and leaf rows share one consistent visual model.
5. Existing search and expand/collapse behavior still passes verification.
