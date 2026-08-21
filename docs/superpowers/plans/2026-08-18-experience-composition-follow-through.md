# Experience Composition Follow-Through Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the Experience page into a deliberately paced career narrative whose navigation, recurring tools, and three eras stay readable and balanced from mobile through an ultrawide monitor.

**Architecture:** Keep the existing derived experience view models and semantic three-era markup. Remove the competing page/section spacing systems, pair the orientation controls at wide widths, and keep the career sequence as three full-width chapters with split era/role interiors rather than collapsing the entire career into three narrow columns.

**Tech Stack:** Go, Templ, Tailwind CSS v4 source CSS, existing CSS contract parser, Chrome responsive inspection.

**Spec:** `docs/superpowers/specs/2026-08-18-portfolio-feedback-adjustments-design.md`

## Global Constraints

- Preserve all Experience content, the three eras, jump links, overview counts, role order, and technology order.
- Continue using the Emberglass Night Mulberry, Cocoa Cedar, Candle Oat, Apricot, Rosehip, and Pond Mint tokens.
- Continue using Bricolage Grotesque, Atkinson Hyperlegible, and IBM Plex Mono in their existing roles.
- Edit `.templ` and `cmd/web/tailwind/pages/experience.css`; never hand-edit generated Templ Go or Tailwind output.
- Preserve unrelated worktree changes.

## Design direction

The subject is career progression for a hiring manager who needs to understand widening responsibility quickly. The visual signature is a single vertical signal spine connecting three editorial chapters; the era markers encode actual chronology, while everything around that spine stays restrained.

```text
[ Hero: career thesis + proof ]

[ Jump to an era       | Recurring field kit ]

Responsibility widened over time
  ● [ Foundation summary | two readable role cards ]
  ● [ Systems summary    | three readable role cards ]
  ● [ Cloud summary      | two readable role cards ]

[ Projects / Skills handoff ]
```

This is specific to the page's chronological content and avoids the generic three-equal-dashboard-card pattern that caused the current clumping.

---

### Task 1: Establish one spacing rhythm and one wide-screen era topology

**Files:**
- Modify: `internal/app/experience_style_contract_test.go`
- Modify: `cmd/web/tailwind/pages/experience.css`

**Interfaces:**
- Preserves: `.experience-page[data-layout='career-eras']`, `.experience-era-list`, `.experience-era-layout`, and the three anchor IDs.
- Produces: zero page-level inter-section gap, a single-column era list at every width, and a split era interior at `70rem` and above.

- [ ] **Step 1: Write the failing layout contract**

  Change the valid fixture and validator requirements so `.experience-page[data-layout='career-eras']` owns `gap:0`, the `80rem` three-column era placement is forbidden, and the `70rem` split interior remains the final wide layout.

- [ ] **Step 2: Run the Experience contract and verify the correct failure**

  Run: `go test ./internal/app -run '^TestExperienceRouteCSS' -count=1`

  Expected: FAIL because the production stylesheet still adds a page gap and restores three narrow era columns at `80rem`.

- [ ] **Step 3: Implement the minimal topology correction**

  Set the route root gap to zero. Delete the ordinary `80rem` three-column era/list/track overrides and their forced-color orientation override. Keep the vertical trail, one-column era list, and `70rem` split era interior as the wide-screen endpoint.

- [ ] **Step 4: Verify green and preserve mutation coverage**

  Run: `go test ./internal/app -run '^TestExperienceRouteCSS' -count=1`

  Expected: PASS, including a mutation that reintroduces a three-column wide layout.

### Task 2: Pair orientation and recurring tools without oversized empty panels

**Files:**
- Modify: `cmd/web/partials/experience_kit_stages.templ`
- Modify: `cmd/web/tailwind/pages/experience.css`
- Modify: `cmd/web/partials/experience_viewmodels_test.go`

**Interfaces:**
- Produces: `.experience-orientation` containing the existing era index and recurring technology panels.
- Preserves: the exact three jump links and eight recurring technologies.

- [ ] **Step 1: Write the failing rendered structure test**

  Render `ExperienceKitStages` and require one orientation wrapper containing the era navigation before the recurring technology panel, with all existing link targets and technology labels intact.

- [ ] **Step 2: Verify red**

  Run: `go test ./cmd/web/partials -run 'Experience.*(Orientation|Technology|Stages)' -count=1`

  Expected: FAIL because the panels are separate top-level sections.

- [ ] **Step 3: Implement the orientation band**

  Wrap both panels in one tight section. Stack them on compact layouts and use a balanced two-column grid at wide widths. Give the recurring-tools title a controlled route-specific scale, align its copy and chip grid, and remove height caused only by generic section padding.

- [ ] **Step 4: Regenerate and verify green**

  Run: `task generate`

  Run: `go test ./cmd/web/partials ./internal/app -run 'Experience' -count=1`

  Expected: PASS.

### Task 3: Iterate visually across the required widths

**Files:**
- Modify if evidence requires: `cmd/web/tailwind/pages/experience.css`

**Interfaces:**
- Consumes: Chrome viewport capability at 390, 768, 1119, 1121, 1440, and the user's native wide viewport.
- Produces: no horizontal overflow, no clipped text, readable role cards, and deliberate section gaps.

- [ ] **Step 1: Build authored sources**

  Run: `task generate && task tailwind-build`

- [ ] **Step 2: Inspect screenshots and geometry at every required width**

  Record viewport width, document scroll width, orientation-panel bounds, era widths, role-card widths, and adjacent section gaps. Capture screenshots for visual judgment.

- [ ] **Step 3: Correct any observed spacing or wrapping defect in the route-owned CSS**

  Keep the signal spine as the only bold structural device; remove any decorative or spacing rule that does not clarify chronology.

- [ ] **Step 4: Repeat build and visual inspection until no listed defect remains**

  Expected: each viewport has one coherent composition, with no page-level horizontal overflow or clumped wide-screen columns.

