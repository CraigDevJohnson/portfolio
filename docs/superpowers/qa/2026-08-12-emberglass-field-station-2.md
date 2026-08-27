# Emberglass Field Station 2.0 — Task 13 QA evidence matrix

Status: COMPLETE — AUTOMATED RUNTIME, INDEPENDENT VISUAL, SOURCE, AND PACKAGE GATES CLOSED

Authoritative browser evidence is the single serialized run at
`output/playwright/task-13/runs/2026-08-16-final-14/`. Earlier runs are RED or
diagnostic history only. Every result below is backed by the final raw result,
telemetry, provenance, and artifact manifest; screenshots are evidence files,
not source-overlay inputs.

Independent visual verdict: APPROVED — P0/P1/P2 none; all 11 Final13 failures are visibly and metrically closed, and representative public, Skills, Soccer, Portal, reduced-motion, forced-color, focus, marker, and fade states are coherent in Final14.

## Run provenance

| Field | Exact evidence | Result |
| --- | --- | --- |
| Accepted Task 12 report | `.superpowers/sdd/2026-08-12-emberglass-field-station-2/task-12-report.md`; SHA-256 `cecea12942cdf48775ecec6827b2c9b71be4850b8a9e9c669a3a7c04ad8b1b55` | PASS |
| Accepted Task 12 manifest | `.superpowers/sdd/2026-08-12-emberglass-field-station-2/task-12-source-manifest.txt`; SHA-256 `228d32a1d9ff50041b9ee94de8e6da00f47b905d85a2235b3234414bb2485549` | PASS |
| Accepted Task 12 ledger | `.superpowers/sdd/2026-08-12-emberglass-field-station-2/task-12-command-ledger.txt`; SHA-256 `01ca10f75040db67898a0b892f1ed0847b1ef7c89a57063cd9bc5e7ce5a943a6` | PASS |
| Git identity | HEAD `1d53687a71ae6c67be632457f3226671bccdfdd0`; branch `copilot/explore-non-aws-app-runner-options`; timezone `America/Boise` | PASS |
| Run | `2026-08-16-final-14`; `2026-08-16T12:25:13.894Z` through `2026-08-16T12:29:20.340Z`; origin `http://127.0.0.1:8182` | PASS |
| Preflight | `preflight-static.json`; 65,610 bytes; SHA-256 `57d18a1f6717949f2aa89c9edada1ef66e4ad08c74303b09276cc0b46b96af3e`; token `fccbe6a285db838f20bcfff32915ac267dd7c75fbc62b29534ccae9d848cc4ce` | PASS |
| Build | `portfolio-server`; 18,074,418 bytes; mtime `2026-08-16T12:24:26.213Z`; SHA-256 `23e7efb68872b9c66dbe3bfe1698d8cce094d5441a65e22843cc7150b5cc915d` | PASS |
| Compiled CSS | `cmd/web/static/css/tailwind.css`; 346,784 bytes; mtime `2026-08-16T12:24:12.618Z`; SHA-256 `fc10acc118646d2ccb731090fc957c8663230e6aa0e5e7ca56d5d971f7d5279c` | PASS |
| JavaScript | `cmd/web/static/js/main.js`; 58,978 bytes; mtime `2026-08-13T16:36:29.890Z`; SHA-256 `9433d16b91f6a51f1937cf9c72fc6cdc3258da4c7fc51432838bbff1dcde556d` | PASS |
| Browser | Chromium `151.0.7922.138`; CSS DPR 1; timezone `America/Boise`; serialized named session `task13-2026-08-16-final-14` | PASS |
| Runtime binding | `runtime/runtime-build.json` SHA-256 `730c699fcffa2023b4355f089a664aa86234faf478e2db590539250f2700d93d`; generated runtime SHA-256 `cdc6fdeb462c0c529ce6f8a960bbb9b1e9353a1f7d8820c892934c8255c23944` | PASS |
| Final result | `raw/task13-results.json`; 12,559,023 bytes; SHA-256 `823d556eba234c22337899fd9f85a20516e9c92e264a122780aa2ff14639f541`; `passed:true`, `fatal:null`, `failures:[]`, `browserFailures:[]` | PASS |
| CLI/result binding | `raw/playwright-cli-result.json` SHA-256 `6cd8d8a7e786f868f4e84b92085ccb505a74225a7e04c631fad9a59686fdb26e`; durable browser result SHA-256 `6858ab766611fdecebd0185e7e1211f5bd0fb1eb18af7e25f770e02c2bf1eb9d` | PASS |
| Source provenance | `raw/provenance-before.json` SHA-256 `a212e4b28d69ad65f028e8b63e4293222d67661f95941b54385a45bafd32779e`; `raw/provenance-after.json` SHA-256 `f83b80bd889a125f04318e99a96c6988189a6830c1d499f70c47f0c884d7051d`; 94 live inputs, 27 probe sources, 23 oracle references, and all six served/build records match semantically | PASS |
| Screenshots/artifacts | `artifact-manifest.json`; 305,772 bytes; SHA-256 `ea7d41e79beaf23c7d39e98e9cfe04d4582228aec2d5b187ce87a31a9fe58b70`; 428 valid PNG records and 441 manifested files; 442 actual files including the self-excluded manifest | PASS |
| Telemetry | `raw/telemetry-raw.json` SHA-256 `d51222ad792270487ed00f147db46b73ce49dbd1ab16ac19db0b4c5d46451b5f`; `raw/telemetry-classified.json` SHA-256 `bed555b541653bda89d535e7bc9be3a9decb53c0c20818aca0f33e99f8a6f3c9` | PASS |
| Browser closure | The serialized shell reached its unguarded named-session close before finalization; the run daemon directory contains only a zero-byte `.err`, and no open handle remains against it | PASS |
| Preview cleanup | Root sent Ctrl-C to session 51773 / PID 85640; logs record `shutting down server` and `server shutdown complete`; session exited 0; fresh `lsof` returned exit 1/no output and curl returned exit 7/could not connect | PASS |

## Record and screenshot reconciliation

| Module | Records | Screenshot records | Result |
| --- | ---: | ---: | --- |
| Base route matrix | 50 | 100 `base-*` | PASS |
| Shared shell and Home | 7 | 12 `shared-*` + 2 `home-*` | PASS |
| Skills | 23 | 56 `skills-*` | PASS |
| Soccer | 55 | 108 `soccer-*` | PASS |
| Portal | 40 | 80 `portal-*` | PASS |
| Contrast | 94 | 30 `contrast-*` | PASS |
| Preferences | 20 | 40 `prefs-*` | PASS |
| Total | 289 | 428 | PASS |

The artifact manifest accounts for 445,941,289 bytes in 441 non-manifest
files; the 428 screenshots account for 413,176,180 bytes. Rehashing proved
every path canonical, contained, unique, regular, present, byte-exact, and
SHA-exact. There are no symlinks, path escapes, duplicate paths, missing files,
extra files, phantom records, or last-focus diagnostic screenshots.

## Base full-document route/width matrix

Every row below covers 390, 768, 1119, 1121, and 1440 CSS pixels. Each record
requires the expected HTTP status, exact path, one H1, one typed trail, no
duplicate IDs, page overflow no greater than 1px, visible controls at least
44px, local-scroller containment/reachability, focus/header clearance, and
settled structure. Each record owns one viewport and one full-page PNG.

| Case pattern | Route/state | Records | Result |
| --- | --- | ---: | --- |
| `base:home:{width}` | `/`, default | 5 | PASS |
| `base:about:{width}` | `/about`, default | 5 | PASS |
| `base:experience:{width}` | `/experience`, default | 5 | PASS |
| `base:skills:{width}` | `/skills`, default | 5 | PASS |
| `base:projects:{width}` | `/projects`, default | 5 | PASS |
| `base:education:{width}` | `/education`, default | 5 | PASS |
| `base:contact:{width}` | `/contact`, default | 5 | PASS |
| `base:soccer:{width}` | `/soccer`, local capability | 5 | PASS |
| `base:mgmt:{width}` | `/mgmt`, normal preview | 5 | PASS |
| `base:__preview-portal-error:{width}` | `/__preview/portal/error`, expected 503 | 5 | PASS |

## Route composition and content preservation

| Route | Approved composition and retained behavior | Evidence | Result |
| --- | --- | --- | --- |
| `/` | Systems Overlook; hero, portrait/fallback, topology, proof, copy, and links | Base + shared/Home records | PASS |
| `/about` | Alaska Switchback; dynamic stats, narrative, facts, timeline, hobbies, values | Base records | PASS |
| `/experience` | Career Eras; roles, dates, narrative, technologies, links | Base records | PASS |
| `/skills` | Skills Field Workbench; searchable/filterable catalog, detail, history, no-JS GET fallback | Base + 23 Skills records | PASS |
| `/projects` | Project Dossiers; images, metadata, copy, external links | Base records | PASS |
| `/education` | Education Field Guide; entries, credentials, chronology, copy, links | Base records | PASS |
| `/contact` | Correspondence Window; contact channels, availability, copy, links | Base + Contact preference records | PASS |
| `/soccer` | Matchday Planner; five-stage flow, schedule/results, selection, Calendar/download semantics | Base + 55 Soccer records | PASS |
| `/mgmt` | Operator Workspace; session context, six instance rows, actions, metrics, logs, diagnostics | Base + 40 Portal records | PASS |
| `/__preview/portal/error` | Complete 503 interruption document and recovery navigation | Base + Portal + contrast records | PASS |

## Navigation and shared-shell states

| Exact case ID | State | Result |
| --- | --- | --- |
| `shared:mobile-menu:390` | Menu open, body lock, forward/reverse wrap, Escape restoration | PASS |
| `shared:active:about:768` | Active About route at the first table-width tier | PASS |
| `shared:active:experience:1119` | Last mobile-navigation seam | PASS |
| `shared:active:projects:1121` | First desktop-navigation seam | PASS |
| `shared:active:contact:1440` | Scrolled header and active route | PASS |
| `shared:skip-link:390` | Skip-link focus and fixed-header clearance | PASS |
| `home:gravatar-fallback:390` | Forced local 404, fixed-size `CJ` fallback, no layout shift | PASS |

## Skills states

| Case group | Exact coverage | Records | Result |
| --- | --- | ---: | --- |
| URL/filter states | `search`, `category`, `proficiency`, `combined`, `invalid-normalized`, `valid-empty`, `no-results` at 390 and 1440 | 14 | PASS |
| Request lifecycle | `busy-success`, `rapid-replacement`, `transport-failure-retry` at 390; exact request-scoped busy/error/stale ownership | 3 | PASS |
| Detail behavior | `detail` at 390 and 1440; keyboard focus and pointer no-focus-theft; Close restores exact trigger | 2 | PASS |
| URL/history | `refresh-canonical:1440` and `history-cache:390`, including cache miss, focus, and caret | 2 | PASS |
| Compact rail | `last-filter-focus-clearance:390`; both rails reach their final filter and the focus ring clears the fade | 1 | PASS |
| Progressive enhancement | `no-js:390`; native GET search and category-link fallback | 1 | PASS |

The Skills matrix retains exactly 23 records and 56 screenshots. Every case has
an empty failure list; request replacement, retry, focus/caret restoration,
responsive containment, and structural assertions are stored verbatim in the
authoritative final result.

## Soccer closed fixtures

The closed vocabulary is `manual`, `import`, `token-invalid`, `token-expired`,
`token-rejected`, `token-upstream-error`, `players`, `no-players`,
`team-selection`, `no-games`, `upcoming`, `past`, `combined`,
`google-disconnected`, `google-connected`, `google-add-success`,
`google-add-error`, `google-sync-success`, `google-sync-error`,
`expired-session-reset`, and `loading`.

| Case group | Records | Result |
| --- | ---: | --- |
| `soccer:{fixture}:390` for all 21 names | 21 | PASS |
| `soccer:{fixture}:1440` for all 21 names | 21 | PASS |
| `soccer:unknown-fixture-closed` exact 404/console contract | 1 | PASS |

All four token-feedback fixtures at both widths retain dialog, card,
sticky-action, and actual text-Range geometry. Preview Google actions remain
inert and the network classifier observed no real integration traffic.

## Soccer interaction and enhancement states

| Exact case ID/group | State | Records | Result |
| --- | --- | ---: | --- |
| `soccer:modal:{390,1440}` | Open modal, trap, close paths, focus restoration, body lock/inert background | 2 | PASS |
| `soccer:selection-groups:{390,1440}` | Independent upcoming/past none/partial/all selection | 2 | PASS |
| `soccer:google-connected:390:selection-locks` | Non-vacuous disabled Google action in both groups | 1 | PASS |
| `soccer:player-team-oob:390` | Primary/OOB ownership for player/team changes | 1 | PASS |
| `soccer:loading-lifecycle:390` | Persistent loading and exact two-owner busy settlement | 1 | PASS |
| `soccer:selected-only-ics:390` | Selected-only native ICS; SHA-256 `3e630cff9bc313af6480e405a4f76f7a396dc4b5e763c1777b89a87ad65ad568` | 1 | PASS |
| `soccer:no-js-native-download:390` | JavaScript-disabled native download; ICS SHA-256 `55a3be376d0fba731d679cb0e3061c59ec0afb80d5367a7a74bf7a9efeb53afb` | 1 | PASS |
| `soccer:busy-error:390` | Synthetic 500, settled error, exact busy/feedback ownership | 1 | PASS |
| `soccer:pointer-keyboard-focus:390` | Keyboard target focus versus pointer no-focus-theft | 1 | PASS |
| `soccer:stale-primary-oob:1440` | Stale primary/OOB response suppression | 1 | PASS |

## Portal documents, fragments, and interactions

| Case group | Exact coverage | Records | Result |
| --- | --- | ---: | --- |
| Documents | `normal`, `empty`, `retrieval-error`, `full-error` at 390/1440 | 8 | PASS |
| Fragments | Metrics and logs `loaded`, `empty`, `error` at 390/1440 | 12 | PASS |
| Actions | `stop`, `restart`, `start`, `invalid` at 390/1440 | 8 | PASS |
| Concurrency | `concurrency` at 390/1440 | 2 | PASS |
| Request identity/focus | `same-target-out-of-order`, `pointer-keyboard-focus` at 390 | 2 | PASS |
| Stable baselines | `late-failure-preserves-sibling`, `late-failure-abort-baseline`, `after-swap-baseline-preservation` at 390 | 3 | PASS |
| Responsive/stress | `responsive-and-stress` at 390, 768, 1440 | 3 | PASS |
| Unknown/hostile | `unknown-hostile` at 390/1440; neutral unknown state, actions disabled, hostile strings contained | 2 | PASS |

The 40 records cover all six server lifecycle presentations, exact target busy
ownership, same- and cross-target races, sibling preservation, pointer versus
keyboard focus, compact cards, the 768 table seam, rightmost-column
reachability, truthful overflow hints, and the complete 503 document.

## Measured contrast ledger

All 94 samples pass their per-record WCAG threshold or the narrowly recorded
inactive-control exemption. The table reports the lowest passing ratio within
each threshold class; `—` means that case owns no sample in that class. Exact
foreground/background colors, font size/weight, selector, state, geometry,
pixel samples, thresholds, and ratios remain in `raw/task13-results.json`.

| Contrast case | Samples | Min 4.5:1-class | Min 3:1-class | Inactive exemptions | Result |
| --- | ---: | ---: | ---: | ---: | --- |
| `contrast:public-home` | 15 | 7.696 | 5.549 | 2 | PASS |
| `contrast:skills` | 17 | 6.391 | 3.671 | 2 | PASS |
| `contrast:skills-compact-scroller` | 1 | — | 7.246 | 0 | PASS |
| `contrast:hero-about` | 1 | — | 14.332 | 0 | PASS |
| `contrast:hero-experience` | 1 | — | 12.438 | 0 | PASS |
| `contrast:hero-projects` | 1 | — | 12.001 | 0 | PASS |
| `contrast:hero-education` | 1 | — | 11.324 | 0 | PASS |
| `contrast:hero-contact` | 1 | — | 12.328 | 0 | PASS |
| `contrast:soccer-feedback` | 7 | 7.767 | 3.918 | 0 | PASS |
| `contrast:soccer-status` | 9 | 7.299 | 3.824 | 0 | PASS |
| `contrast:soccer-info` | 2 | 7.466 | 3.450 | 0 | PASS |
| `contrast:shared-warning-danger` | 11 | 7.149 | 3.222 | 2 | PASS |
| `contrast:portal` | 22 | 6.447 | 3.740 | 0 | PASS |
| `contrast:portal-info` | 2 | 7.466 | 3.668 | 0 | PASS |
| `contrast:portal-interruption` | 3 | 7.767 | 4.701 | 0 | PASS |

The modal-specific `contrast:soccer-feedback` case remains on
`/__preview/soccer/token-invalid` and owns only its error, field,
recovery-action, and modal samples. `soccer normal text` and `soccer large
display text` occur exactly once each under modal-free `contrast:soccer-status`
on `/__preview/soccer/google-add-success`. Behavioral, static-source, and
mutation shields reject modal ownership, absence, or duplicate ownership of
those hero samples.

## Reduced-motion and forced-colors matrix

Each row below has one viewport and one full-page screenshot. Reduced-motion
records require visible content, the expected media match, no violating motion,
static trail behavior, and `scroll-behavior:auto`; Skills additionally checks
both live filter scrollers. Forced-color records require authored system-token
ownership, rendered system-color paint, exact trail axis/edges, visible focus,
active/current markers, controls, feedback, and family-specific modal/table/
status coverage.

| Family | Exact case IDs | Records | Result |
| --- | --- | ---: | --- |
| Public | `prefs:public:{reduced,forced}:{390,1440}` | 4 | PASS |
| Contact | `prefs:contact:{reduced,forced}:{390,1440}` | 4 | PASS |
| Skills | `prefs:skills:{reduced,forced}:{390,1440}` | 4 | PASS |
| Soccer | `prefs:soccer:{reduced,forced}:{390,1440}` | 4 | PASS |
| Portal | `prefs:portal:{reduced,forced}:{390,1440}` | 4 | PASS |

Contact is explicit rather than inferred from the generic public route: its
forced-color current marker is non-vacuous at both widths. Skills is likewise
explicit: its reduced-motion rows assert both real filter-tab scrollers and its
forced-color rows cover the workbench controls and compact focus ownership.

## Structural probe contract

The 175 non-contrast module records run the shared structural oracle, while
contrast and preference cases run their own fail-closed rendered-geometry
contracts. Final14 records no structural failure. Covered invariants include:

- full-document versus fragment H1/trail cardinality;
- duplicate IDs, page overflow, clipping, fixed-header clearance, and 44px
  interactive targets;
- truthful local-scroll hints, keyboard reachability, table final-column
  containment, and complete focus paint;
- HTMX request/settling/swapping owners, exact busy targets, and stable terminal
  state before screenshot capture;
- modal body lock, dialog visibility, safe-area containment, focus ownership,
  and outside-overlay inert inventory;
- ordinary/gradient text glyph geometry, compositing exclusions, all-edge
  boundary/outline pixels, and state restoration after paint probes.

## Telemetry and network classifier

| Stream | Raw/accounted | Expected | Unexpected/late/missing | Result |
| --- | ---: | ---: | ---: | --- |
| Console | 29 / 29 | 29 | 0 | PASS |
| Page errors | 0 / 0 | 0 | 0 | PASS |
| Responses | 3,892 / 3,892 | 18 deliberate non-2xx | 0 | PASS |
| Failed requests | 4 / 4 | 4 deliberate abort/failure cases | 0 | PASS |
| Requests | 3,896 / 3,896 | Fully accounted | 0 | PASS |
| External requests | 1,840 | Only `fonts.googleapis.com`, `fonts.gstatic.com`, `unpkg.com`, `www.gravatar.com` | 0 unapproved | PASS |
| Forbidden integrations | 0 | No LPS, Google Calendar/OAuth, AWS, EC2, CloudWatch, Cognito, credential, token, or production action traffic | 0 | PASS |

The exact response ledger is five base 503s, Home 404, closed Soccer fixture
404, Soccer synthetic 500, two Portal document 503s, four Portal detail 500s,
two Portal invalid-ID 400s, one late Portal 500, and one contrast-page 503. The
four failed requests are the Skills transport failure, no-JS Soccer download
abort, Portal late-failure abort, and Portal after-swap failure. Every associated
Chromium/HTMX console event is consumed exactly once. Classifier reconciliation
passes with empty late, unexpected, missing-expected, and forbidden subsets.

## Defect and RED chronology

| Run | Exact evidence | RED result | Classification and closure |
| --- | --- | --- | --- |
| Final11 | `runs/2026-08-16-final-11/raw/task13-results.json`; SHA-256 `2784b43289a8a3f2fcaf46abfaffdfed00b2e39cd0084429a8c6211a4db3cfbe`; manifest SHA-256 `ad9f4e5c5f31f23064ecee66980e86aa3cdbb8248775bc37123393ad814d38d2` | 18 failures; five other modules green | Eight rendered contrast defects: Home muted text; Skills secondary action default/focus text; danger default/hover/focus/disabled text/boundaries. Ten forced-focus ownership false-reds covered public, Contact, Skills, Soccer, and Portal at both widths. Product paint was fixed in canonical owners; authored-token parsing was hardened with mutation tests. |
| Final13 | `runs/2026-08-16-final-13/raw/task13-results.json`; SHA-256 `c0f34e7daca508516bb93175059dfe17cfb0f7aaa6b633447bee8251bfae35ff`; manifest SHA-256 `8f08e3fab8f751868a11f0fbaf5db0098dc63ec3545f88f6c40ce77c993a8b59` | 11 failures; five other modules green | Nine stronger rendered-paint REDs: Skills/about/experience/projects/education/contact/Portal display text plus danger default/focus boundary. Canonical shadow, generated-glyph, layer, and danger-boundary contracts closed them. Two Soccer hero REDs were a case-state harness defect: the modal-open token-invalid route occluded page-hero samples. Mutation-first shields moved exactly those two samples to modal-free google-add-success while preserving modal-specific coverage and total record count. |
| Final14 | `runs/2026-08-16-final-14/raw/task13-results.json`; SHA-256 `823d556eba234c22337899fd9f85a20516e9c92e264a122780aa2ff14639f541` | 0 failures; 289 records green | Definitive recapture; all six modules, provenance, telemetry, runtime/CLI binding, artifacts, and independent visual review pass. |

## Repository-authoritative final command ledger

The pre-browser build, Final14 run, and post-review gate sequence use identical
served/source hashes. Exact commands and output are retained in
`task-13-command-ledger.txt`.

| Gate | Exact outcome | Result |
| --- | --- | --- |
| `task generate` | Exit 0; updates=0; 54.289375ms | PASS |
| `task fmt` | Exit 0; `Formatting files...` | PASS |
| `task lint` | Exit 201; context loading failed with `no go files to analyze`; task reported underlying exit 5 | BLOCKED — ENVIRONMENT/TOOL |
| `task vet` | Exit 0; no output; run immediately after lint | PASS |
| `task test` | Exit 0; 11.066s | PASS |
| `task build` | Exit 0; generation updates=0 in 51.912917ms; Tailwind CSS 4.2.4 completed in 72ms | PASS |
| Compiled CSS architecture | `VERIFY_COMPILED_CSS=1`; exit 0; `ok portfolio/internal/app 1.453s` | PASS |
| JavaScript/shell syntax | `main.js`, every Task13 JS/MJS source, and runner shell syntax | PASS |
| Full Node hardening | 117/117 plus 70 Soccer/Portal mutation shields; 2.752s | PASS |
| `git diff --check` | Exit 0; staged name list empty | PASS |

The lint result is not called a pass. It is an isolated context-loading failure,
not a reported source finding; the immediately following vet, repository test,
build, compiled-CSS, syntax, Node, and diff gates all pass. Post-gate binary,
compiled CSS, JavaScript, harness, focused-test, product-contract, and canonical
component hashes remain byte-identical to Final14, so no browser recapture is
required.

## Design acceptance crosswalk

| Criterion | Source/runtime evidence | Result |
| --- | --- | --- |
| Palette, type, controls, trails | 94 contrast + 20 preference records; canonical owner tests | PASS |
| Distinct route structures | 50 base records and composition matrix | PASS |
| Shared shell/actions/stats/feedback/focus/spacing | 7 shared, 23 Skills, 55 Soccer, 40 Portal records | PASS |
| No override-layer drift; explicit breakpoint ownership | Task 12 accepted architecture plus Task13 selector/layer mutation tests | PASS |
| Correct Go/Templ/HTMX/CSS responsibility split | Frozen source overlay and focused contracts | PASS |
| Portfolio/Soccer/Calendar/download/nav/external-link behavior | Module behavior records, selected-only ICS, exact network ledger | PASS |
| Required widths/states have no overlap/clipping/cutoff/page overflow | 175 structural records and 428 screenshots; independent visual approval | PASS |
| Focus, reduced motion, forced colors, error, empty, loading | Module records + 20 explicit preference records; independent visual approval | PASS |
| Repository gates | Ordered sequence above; lint preserved as an environment/tool block, every remaining gate passed | PASS WITH RECORDED LINT BLOCK |

## Dirty-tree, snapshot, and package reconciliation

| Check | Evidence | Result |
| --- | --- | --- |
| Pre-run tree | Exact porcelain, staged-path, and name-only ledgers are embedded in both provenance files; staged paths are empty | PASS |
| Accepted base | `task-12-after/` and the three sealed Task 12 handoff hashes above | PASS |
| Task 13 allowlist | 46 paths: 18 live source/test deltas, 27 harness/probe files, and this QA file | PASS |
| Generated exclusion | No `*_templ.go`, compiled `tailwind.css`, binary, cache, failed run, browser artifact, or `.playwright-cli` path enters through the Task13 delta; historical `.playwright-cli` paths inherited unchanged from Task12 remain outside Task13 scope | PASS |
| Screenshot exclusion | The 428 Final14 PNGs remain only under the run root and are not copied into the source snapshot | PASS |
| Legacy removal | `emberglass.css`, `emberglass-responsive.css`, and `emberglass-accessibility.css` remain absent | PASS |
| User work preservation | Snapshot is based on frozen Task12 and receives only the exact allowlist; unrelated dirty paths/hunks remain untouched and unstaged | PASS |
| Process boundary | No browser session residue; server shutdown completed cleanly; port 8182 has no listener or successful HTTP response | PASS |
| Final manifest/package | Live/head/manifest hash equality, 46-path name-status equality, exclusions, and no-index package verified after the root gates | PASS |

## Final verdict

Final14 is the accepted automated and independently visually approved browser
run: six modules and 289 records pass, with zero final/browser/fatal failures,
exact closed telemetry, stable before/after provenance, 428 valid screenshots,
complete runtime/CLI/final binding, and no P0/P1/P2 visual finding. The Task13
source/harness changes are narrow and traceable to Final11/Final13 RED evidence.
The post-review repository sequence preserves one exact lint environment/tool
block and passes every other gate; final source, manifest, snapshot, and package
reconciliation closes the technical handoff.
