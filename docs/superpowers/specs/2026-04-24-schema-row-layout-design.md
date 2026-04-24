# Schema Row Layout Design

Date: 2026-04-24
Status: Proposed

## Goal

Prevent long field names and deep nesting from pushing schema descriptions off-screen, especially on phones, while keeping full field names visible inline.

## Problem

The schema renderer currently lays out each property row horizontally:

- expandable rows render the field name and badges in a single `summary` line, with the description below only after expansion
- leaf rows render the field name, badges, and description in one horizontal flex row

This creates two related failures:

- long field names consume the width needed for description text
- each nesting level reduces usable width further, making clipping and poor wrapping worse on narrow screens

The current layout also differs between expandable and leaf rows, which makes responsive fixes harder to apply consistently.

## Constraints

- Full field names must remain visible by default. No truncation, ellipsis, or hidden-overflow name treatment.
- Existing search, path metadata, expand/collapse behavior, and keyboard interaction must continue to work.
- The fix should stay local to the schema renderer/template and avoid unrelated interaction redesign.
- The hierarchy must remain legible even if indentation is reduced.

## Recommended Approach

Adopt a stacked row layout for all property rows and reduce the horizontal cost of nesting.

This means:

- each property row gets a header block for field name, badges, and disclosure affordance
- each property row gets a body block for description and constraints
- child groups use lighter visual structure and less left padding than today

This directly removes the competition between field-name width and description width while also protecting nested rows from progressive width collapse.

## Alternatives Considered

### 1. Mobile-only stacked layout

Keep the current desktop layout and stack only below a breakpoint.

Rejected because:

- medium-width screens and deep nesting would still suffer
- the same content would behave differently across screen sizes
- the renderer would keep two layout models to maintain

### 2. Truncate or collapse long field names

Use ellipsis, overflow controls, or a disclosure affordance to preserve description width.

Rejected because:

- it conflicts with the requirement that full field names stay visible
- schema docs rely on exact field names for scanning and path recognition

## Layout Design

### Property row structure

All property rows should share the same visual model.

Expandable rows:

- `summary` becomes a header-only region
- the header contains disclosure chevron, field name, and badges
- the description and constraints render in the content block immediately below the header, before child properties

Leaf rows:

- leaf cards use the same header/body structure without `details`
- the header contains field name and badges
- the body contains description and constraints

### Header behavior

- Field names may wrap to multiple lines.
- Badges should remain visually attached to the header content, but may wrap onto a following line when needed.
- The disclosure chevron should remain stable and readable even when the field name wraps.

### Body behavior

- Description text spans the full row width beneath the header.
- Constraints render below the description in the same body block.
- Long tokens in descriptions or constraints should be allowed to break so they cannot overflow on narrow screens.

## Nesting Design

The current tree communicates depth mostly through left padding. The revised design should shift some of that burden to lighter structural cues.

### Indentation changes

- Reduce left padding for nested content at all depths.
- Apply an additional reduction on narrower screens.

### Structural cues

- Add a subtle child-group rail or border so nested relationships remain visually clear.
- Keep card boundaries and disclosure affordances as the primary hierarchy signal.

This preserves the tree structure while reclaiming width for readable text.

## Non-Goals

- No change to search semantics or path matching behavior.
- No change to how schema paths are generated or stored in data attributes.
- No change to expand/collapse controls beyond visual layout adjustments.
- No change to theme direction outside the renderer layout needed for this fix.

## Affected Areas

- `renderer/renderer.go`

Expected change areas inside that file:

- schema page CSS for property rows, nested containers, and responsive behavior
- property template markup for expandable and leaf rows

No backend data-model or extraction changes are expected.

## Verification

Implementation is complete when the following are true:

1. Long field names remain fully visible without horizontal clipping.
2. Description text remains readable on narrow screens and does not extend past the viewport edge.
3. Deeply nested rows retain more usable width than the current layout.
4. Expandable and leaf rows use the same stacked visual model.
5. Search highlighting, path metadata, and expand/collapse behavior still work.

## Testing Plan

- Add or update renderer tests that assert the new shared header/body markup for leaf and expandable rows.
- Add or update renderer tests that cover long names and deeply nested properties rendering in the new structure.
- Manually verify in `go run ./cmd/ preview` at phone-width and desktop-width breakpoints.

## Risks

### Increased vertical density

Rows will be taller than today. This is accepted because the current failure mode hides or clips the most useful text on constrained screens.

### Summary hit-area/layout interactions

Wrapping content inside `summary` can create awkward alignment if the chevron and badges are not explicitly structured. The header should use a stable internal layout rather than relying on a single unbounded inline row.

### Inconsistent spacing between leaf and expandable rows

This is avoided by making both row types share the same header/body model instead of patching them separately.
