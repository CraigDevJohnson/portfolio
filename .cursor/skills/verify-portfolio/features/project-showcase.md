<!-- markdownlint-disable MD013 -->
# Project showcase

Project showcase lets a visitor move from Home into three project dossiers, read each problem, approach, outcome, and technology set, and follow the available project destinations.

## Sub-features

- `projects-home-entry` opens Projects from the Home hero.
- `projects-dossiers` renders one lead dossier and two supporting dossiers.
- `projects-content` exposes Problem, Approach, Outcome, and Technology for every dossier.
- `projects-destinations` exposes only the destination links supplied by project data.
- `projects-responsive` preserves dossier reading order at desktop and mobile widths.

## How to get to it (user POV)

- From Home, choose `See Projects`.
- Choose `Projects` from either shared navigation landmark.
- Read the `Projects` page from the lead dossier through both supporting dossiers.
- Choose a visible destination such as a repository or live site; it opens in a new tab when external.

## Driving it with Playwright CLI

Preconditions:

- `doctor` passes and the browser starts at `$VERIFY_URL/` with a fresh snapshot.

- **Home entry.** Run `click ".home-overlook-secondary-action[href='/projects']"` and snapshot. The URL path is `/projects`, the H1 is `Projects`, and `Projects` is current in the desktop navigation.
- **Dossiers.** Evaluate `() => [...document.querySelectorAll('[data-project-dossier]')].map(article => article.querySelector('h3')?.textContent?.trim())`. The ordered result is `Personal Portfolio Website`, `New User Account Provisioning`, and `Soccer Schedule Scraper`.
- **Content.** For each `[data-project-dossier]`, require one each of the headings `Problem`, `Approach`, `Outcome`, and `Technology`, plus non-empty body content.
- **Destinations.** Evaluate each visible link inside `.project-dossier-actions`; require a non-empty label and `href`, and require external links to have `target="_blank"` plus `rel` containing `noopener` and `noreferrer`.
- **Responsive order.** Capture full-page screenshots at `1440x1000` and `390x844`. At both widths, evaluate the ordered dossier headings and require the same three-name sequence.
- **Proof.** Save Home before-action evidence, the Projects landing snapshot, both responsive screenshots, ordered dossier assertions, and destination attributes under `evidence/project-showcase/`.

## Gotchas

- Project images can be remote. A failed remote image must reveal the dossier's local fallback without hiding its text.
- Do not click external destinations unless the verification scope includes the external site; inspecting visible link attributes is enough for portfolio behavior.
- The lead/support visual layout changes by viewport, but DOM reading order must remain stable.
- The route has three dossiers in current data. Update this map when project data changes rather than weakening the count assertion.
