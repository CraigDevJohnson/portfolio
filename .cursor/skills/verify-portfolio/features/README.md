<!-- markdownlint-disable MD013 -->
# Portfolio verification map

This directory is the maintained source for verifying Craig Johnson's user-facing portfolio. The primary surface is the public server-rendered website. Soccer is a public tool with external LPS and Google boundaries; the management portal is optional and protected in production, with a deliberately isolated loopback preview for local verification.

## Baseline preconditions

- Launch with `.cursor/skills/verify-portfolio/scripts/control-portfolio launch` and require `doctor` to pass.
- Keep the same `VERIFY_RUN_ID` for lifecycle, browser, and evidence commands.
- Read the server URL from `output/playwright/verify-portfolio/$VERIFY_RUN_ID/state/url`; never assume an existing `localhost:8080` belongs to this run.
- Use the named browser session `portfolio-$VERIFY_RUN_ID` and `/Users/craigjohnson/.codex/skills/playwright/scripts/playwright_cli.sh`.
- Start public-page recipes at `/`. Start portal recipes at `/mgmt` only after the preview safety banner is present.
- Do not import a real LPS JWT, start Google OAuth, or target live AWS from this verification launch.

## Driving conventions

- Capture a Playwright snapshot before using element refs; use the stable selectors written in each feature recipe when they are unique.
- Prefer `nav[aria-label='Main navigation']`, `nav[aria-label='Mobile navigation']`, accessible labels, route paths, and repo-owned `data-*` attributes.
- Snapshot again after navigation, HTMX swaps, mobile-menu changes, and loaded detail regions.
- Use `domcontentloaded` or explicit visible-state waits; do not wait for `networkidle` because remote fonts, Gravatar, and CDN assets can keep the network busy.
- Treat desktop and mobile entry points as separate paths. Verifying one does not implicitly verify the other.

## Proof and skip reporting

- Save artifacts below `output/playwright/verify-portfolio/$VERIFY_RUN_ID/evidence/<feature-id>/`.
- Capture the visible action and resulting state, not only a final screenshot.
- Pair screenshots with accessibility snapshots and explicit route/content/control assertions.
- For HTMX, retain browser requests showing the fragment URL and prove the pushed URL plus swapped content.
- Public browsing is read-only. For an ICS download, preserve the downloaded file and inspect its content. For portal preview actions, preserve the safety banner, `Preview only` feedback, request record, and server log.
- Preview fixtures prove UI behavior at an isolated external boundary. They do not prove live LPS, Google, Cognito, CloudWatch, EC2, public connectivity, or deployed behavior.
- Report each unreachable entry point with the attempted command and unmet precondition. Do not credit a skipped path through another route.

## Feature entry contract

Each feature file uses the same four H2 sections: `Sub-features`, `How to get to it (user POV)`, `Driving it with Playwright CLI`, and `Gotchas`. Commands assume the baseline exports from the parent skill.

## Features

- [Portfolio navigation](./portfolio-navigation.md) covers desktop and mobile navigation, Home CTAs, current-page state, and the skip link.
- [Skills catalog](./skills-catalog.md) covers search, category and proficiency filters, URL-backed state, empty results, and HTMX skill details.
- [Project showcase](./project-showcase.md) covers the Home Projects CTA, the three project dossiers, destinations, and responsive reading order.
- [Soccer schedule](./soccer-schedule.md) covers the production entry page and safe fixture-backed selection and ICS download behavior.
- [Management portal preview](./management-portal-preview.md) covers the loopback-only dashboard, instance controls, metrics, logs, and interruption states without live AWS or Cognito.
