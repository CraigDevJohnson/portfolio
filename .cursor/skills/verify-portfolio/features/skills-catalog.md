<!-- markdownlint-disable MD013 -->
# Skills catalog

Skills catalog lets a visitor search the technical toolkit, combine category and proficiency filters, open a skill detail inline, clear empty results, and preserve the current catalog state in the URL.

## Sub-features

- `skills-search` filters by skill name, description, category, and tags after a short debounce.
- `skills-category` filters through links in the `Filter by category` group.
- `skills-proficiency` filters through links in the `Filter by proficiency` group.
- `skills-detail` loads and closes one inline HTMX detail card.
- `skills-empty` explains a no-match state and clears all filters.
- `skills-history` pushes filter state into `/skills` query parameters and restores it through browser history.

## How to get to it (user POV)

- Choose `Skills` from the main navigation.
- Enter text in `Search the toolkit` or choose `Search`.
- Choose a category such as `Cloud Platforms` and a proficiency such as `Expert`.
- Choose a skill card's `Details` control; choose `Close detail` to dismiss it.
- Choose `Clear filters` after a query has no matches.

## Driving it with Playwright CLI

Preconditions:

- `doctor` passes and the browser starts at `$VERIFY_URL/` with a fresh snapshot.
- `window.htmx` is present. A missing CDN script is a failed HTMX precondition, not proof of the dynamic path.

- **Reach Skills.** Run `click "nav[aria-label='Main navigation'] a[data-nav-page='skills']"` and snapshot. The URL path is `/skills`, `Skills` is current in the main navigation, and `Explore the full technical toolkit` is visible.
- **Search.** Run `fill "#skills-search" "Terraform"`, then `run-code "async (page) => { await page.waitForFunction(() => document.querySelector('.skills-result-summary')?.textContent?.trim() === '1 skill shown'); }"`. The URL is `/skills?q=Terraform`, the searchbox retains `Terraform`, and the only `[data-skill-detail-trigger]` is the `Terraform` card.
- **Category filter.** Clear the search with `fill "#skills-search" ""`, wait for the unfiltered count, then click `"a[data-skill-filter-category='Cloud Platforms']"`. The pushed URL contains `category=Cloud+Platforms`, that link has `aria-current="page"`, and the visible catalog contains the nine current primary-or-tagged Cloud Platforms entries.
- **Proficiency filter.** Click `"a[data-skill-filter-proficiency='expert']"`. The URL retains the category and adds `proficiency=expert`; the result summary and visible cards reflect the intersection.
- **Detail.** Return to `/skills?q=Terraform`, snapshot, then click `"#skill-trigger-29"`. Wait for `#skill-detail-heading-29`, snapshot again, and require the trigger's `aria-expanded="true"`. Choose `#skill-detail-close-29`; the detail slot becomes empty and the trigger returns to `aria-expanded="false"`.
- **Empty state.** Fill `#skills-search` with `definitely-not-a-portfolio-skill`, wait for `No skills match these filters`, then choose `#skills-clear-filters`. The URL returns to `/skills`, the query is empty, and the catalog is populated.
- **History.** Search for `Terraform`, go back, then go forward. The input, result summary, visible card, and URL must agree after each history move.
- **Proof.** Run `.cursor/skills/verify-portfolio/scripts/prove-skills-search` for the maintained starter proof. It captures Home, Skills landing, entered query, filtered result, assertions, browser requests, console diagnostics, and a SHA-256 manifest under `evidence/skills-search/`.

## Gotchas

- Search waits `300ms`; wait for the observable result summary instead of sleeping a fixed interval.
- Category values are human-readable strings such as `Cloud Platforms`, while proficiency values are lowercase such as `expert`.
- HTMX replaces the filterable region. Snapshot again before driving a skill card or clear link.
- A URL change without matching visible cards is not proof; assert both.
- A server-rendered fallback after form submission does not prove the HTMX swap. Retain the browser request record when claiming HTMX.
