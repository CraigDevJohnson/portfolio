# Interface conventions

The portfolio helps prospective colleagues and employers understand Craig's cloud
engineering work and find the relevant experience, projects, or contact channel.
The public header and footer provide the same identity and navigation on every page.

## Visual language

Preserve the Emberglass palette from `cmd/web/tailwind/theme.css`: night mulberry
(`#17121B`), cocoa cedar (`#2E2130`), candle oat (`#FFF0D8`), campfire apricot
(`#FFA677`), rosehip (`#FF7FA8`), and pond mint (`#78E3C3`). Use the existing tokens
in CSS. IBM Plex Sans carries identity, headings, body copy, and controls;
IBM Plex Mono carries dates, technical labels, and other utility text. Use regular
(400) for body copy, medium (500) for quiet controls, semibold (600) for emphasis,
and bold (700) for the largest headings and identity. Load real italic styles for
Plex Sans. Use the shared weight tokens and avoid unsupported intermediate or
extra-bold weights. Home hero text uses slightly looser tracking and line spacing;
long Experience role titles wrap naturally with a 1.2 line height.

Page eyebrows identify the section; the main heading explains its content rather
than repeating the section name. Use Experience as the reference for page heroes:
a prominent heading, a concise introduction, and relevant summary cards or actions
in the wide text column beside a narrower image. Use the shared overview layout,
with content determining height and text before imagery on small screens. Move
existing summary cards into the hero instead of repeating them in a separate band.
Derive counts from the content they summarize; keep longer background prose in
the body. Home retains its identity composition, and task pages use their useful
actions rather than adding statistics just to fill space.

The warm CJ medallion is the shared signature. Keep surrounding navigation quiet:
text labels for destinations, a pill for the active header destination, and an
underline for the active footer destination. Do not add decorative route icons.

Portfolio links form one vertical list at every width, with Tools and Connect
stacked beside it. Built with credits use a compact grid grouped by Backend,
Frontend, Infrastructure, and Delivery, with quiet rules and mint text links.
Keep these credits separate from navigation and use a plain **AI-assisted
development** credit rather than naming one AI vendor. On small screens, place
navigation before the technology credits. Preserve at least 44 by 44 CSS pixels for interactive targets.
The menu fills the available viewport, traps focus while open, and returns focus
to a visible control when dismissed or resized to desktop navigation.

## Identity and language

- Use `SiteIdentity()` for the public header, public footer, and operator footer.
  Display **Craig Johnson** and **Cloud Engineer Principal**. The decorative **CJ**
  monogram always accompanies the full name; it is hidden from assistive technology.
- Keep route labels in the canonical `navItems()` list. Use full words in navigation
  and mark the matching link with `aria-current="page"` in each navigation region.
- Preserve proper product names: **Go**, **templ**, **htmx**, **Tailwind CSS**,
  **AWS Lambda**, **OpenTofu**, **GitHub Actions**, **GitHub**, **LinkedIn**, and **macOS**. Do not force brand
  names into uppercase with CSS. Uppercase utility headings remain a visual style.
- Use sentence case for action labels. Keep repeated destinations consistent: **View experience**,
  **View projects**, **View skills**, and **Contact me**. Use **Email me** for the
  direct email action and **Learn more** for a skill's external reference.
- Use **CI/CD** and **IaC** consistently. Explain unfamiliar abbreviations when
  introducing them in prose; avoid shortening navigation labels to save space.
  Use **calendar (.ics) file** on introduction and **.ics file** thereafter.
- When a link opens a new tab, append **(opens in a new tab)** to its accessible
  name with a separating space, and use `rel="noopener noreferrer"`.

## Icons

Reuse the typed `UIIcon` component for professional concepts and contact channels.
Keep visible text beside contact icons. Decorative icons use `aria-hidden="true"`
and `focusable="false"`, inherit the text color, and retain their square aspect
ratio. Use actual brand marks for GitHub and LinkedIn, and the mail icon for Email.
Use the shared SVG icons for hobbies, professional capabilities, Skills practices,
status, and controls. Do not use emoji or text glyphs as UI icons. Product skill cards retain their real
brand SVG assets on shared cream frames; use official light-background variants
such as Terramate's black mark for contrast. Use skills, categories, and proficiency in catalog labels and counts;
reserve the workbench metaphor for visual styling. Preserve icon-library
attribution when adding source assets.

## Sources and verification

Product spelling follows [Go](https://go.dev/), [templ](https://templ.guide/),
[htmx](https://htmx.org/), [Tailwind CSS](https://tailwindcss.com/), and
[Apple's macOS page](https://www.apple.com/os/macos/).
Microsoft credential expansions follow [Microsoft Learn](https://learn.microsoft.com/en-us/credentials/certifications/posts/mcsa-mcsd-mcse-certifications-retire-with-continued-investment-to-role-based-certifications).
[W3C abbreviation guidance](https://www.w3.org/WAI/WCAG22/Understanding/abbreviations.html)
supports providing an expansion or meaning; it is a Level AAA criterion and this
convention is not a claim of whole-site WCAG conformance.

After source changes, run the repository's `task ci` gate. Verify the final built
pages at compact, tablet, and desktop widths, including both sides of the 70rem
navigation breakpoint, keyboard dismissal and focus return, footer link labels,
and the operator footer. Capture fresh screenshots after the final build.
