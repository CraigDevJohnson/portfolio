<!-- markdownlint-disable MD013 -->
# Portfolio navigation

Portfolio navigation lets a visitor move among Home, About, Experience, Skills, Projects, Education, Contact, and Soccer from shared desktop or mobile chrome, use Home calls to action, and skip directly to main content.

## Sub-features

- `navigation-desktop` follows every link in the `Main navigation` landmark.
- `navigation-mobile` opens, traps focus in, closes, and follows links from the `Mobile navigation` landmark.
- `navigation-current` marks the current page with `aria-current="page"`.
- `navigation-home-cta` reaches Experience, Projects, and Contact from the Home hero.
- `navigation-skip` moves keyboard focus to `#maincontent`.

## How to get to it (user POV)

- Open `/` and choose a link in the desktop header.
- At a viewport narrower than `70rem`, choose `Open navigation menu`, then choose a mobile link.
- From Home, choose `View Experience`, `See Projects`, or `Get in Touch`.
- Press Tab once from a fresh page and activate `Skip to main content`.

## Driving it with Playwright CLI

Preconditions:

- `doctor` passes and `VERIFY_URL`, `PORTFOLIO_PWCLI`, and `PORTFOLIO_PW_SESSION` are set as documented in `SKILL.md`.
- The browser session has been opened at `$VERIFY_URL/` and a fresh snapshot has been taken.

- **Desktop routes.** Follow every desktop link, returning to Home before the next click: `home:/`, `about:/about`, `experience:/experience`, `skills:/skills`, `projects:/projects`, `education:/education`, `contact:/contact`, and `soccer:/soccer`. After each navigation, assert the path, visible H1, and exactly one matching desktop link with `aria-current="page"`.
- **Home CTAs.** Return Home before each action and follow `View Experience`, `See Projects`, and `Get in Touch`. Assert `/experience`, `/projects`, and `/contact` respectively, plus the destination H1.
- **Mobile open.** Run `resize 390 844`, `goto "$VERIFY_URL/"`, snapshot, then `click "#mobile-menu-btn"`. Snapshot again. The button is named `Close navigation menu`, has `aria-expanded="true"`, and `nav[aria-label='Mobile navigation']` has `aria-hidden="false"`.
- **Mobile focus trap.** After opening the menu, require Home to have focus and the page outside the menu to be inert. Press `Shift+Tab` twice to cross the first boundary and require focus to wrap to Soccer; press `Tab` and require focus to wrap to the menu trigger.
- **Mobile follow.** Run `click "nav[aria-label='Mobile navigation'] a[data-nav-page='contact']"`. The URL path is `/contact`, the heading is `Get in Touch`, and the menu is closed after navigation.
- **Mobile escape.** Reopen `#mobile-menu-btn`, press `Escape`, and snapshot. The button is named `Open navigation menu`, has focus, and reports `aria-expanded="false"`.
- **Skip link.** Run `goto "$VERIFY_URL/"`, press `Tab`, snapshot, then press `Enter`. Evaluate `() => document.activeElement?.id`; it must return `maincontent`.
- **Proof.** Save separate desktop and mobile before/action/result screenshots and snapshots under `evidence/portfolio-navigation/`; assert each URL and heading, current navigation state, menu state, inert background, focus wrapping, and focused element.

## Gotchas

- Desktop and mobile contain duplicate link labels. Scope selectors to the named navigation landmark.
- The mobile trigger remains in the DOM at desktop sizes; resize before treating it as a user-visible entry point.
- Re-snapshot after every full navigation because element refs are stale.
- External footer links open new tabs. Do not confuse a new-tab destination with an in-app route result.
