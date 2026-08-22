package app

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

type experienceCSSRule struct {
	selector      string
	declarations  map[string]string
	minWidthRem   float64
	forcedColors  bool
	reducedMotion bool
}

var experienceRangeWidthPattern = regexp.MustCompile(`(?i)\bwidth\s*>=\s*([0-9]+(?:\.[0-9]+)?)rem`)

func TestExperienceRouteCSSContract(t *testing.T) {
	css := readTask2Artifact(t, "cmd", "web", "tailwind", "pages", "experience.css")
	if err := validateExperienceRouteCSS(css); err != nil {
		t.Fatal(err)
	}
}

func TestExperienceRouteCSSValidatorRejectsRegressions(t *testing.T) {
	const valid = `
    .experience-page[data-layout='career-eras'] { display:grid; gap:0; }
    .experience-orientation { position:relative; display:grid; grid-template-columns:minmax(0,1fr); align-items:stretch; isolation:isolate; }
    .experience-orientation > :not(.experience-orientation-trail) { position:relative; z-index:1; }
    .experience-orientation-trail { position:absolute; z-index:0; inset:1rem -0.75rem; display:block; width:auto; height:auto; margin:0; pointer-events:none; transform:none; }
    .experience-orientation-trail .signal-trail-svg { display:block; }
    .experience-overview-stats { display:grid; grid-template-columns:minmax(0,1fr); }
    .experience-era-list { display:grid; grid-template-columns:minmax(0,1fr); grid-template-rows:auto; align-items:start; }
    .experience-era-item { min-width:0; grid-column:auto; grid-row:auto; align-self:start; height:auto; margin:0; transform:none; }
    .experience-era { scroll-margin-top:calc(var(--header-height) + var(--space-lg)); }
    .experience-era-layout { display:grid; grid-template-columns:minmax(0,1fr); align-items:start; min-width:0; }
    .experience-era-title-row { display:flex; flex-wrap:wrap; align-items:baseline; justify-content:space-between; }
    .experience-role-list { display:grid; grid-template-columns:minmax(0,1fr); align-items:start; }
    .experience-role-item { min-width:0; align-self:start; height:auto; min-height:0; }
    .experience-role-card { min-width:0; align-self:start; height:auto; min-height:0; }
    .experience-role-title-row { display:grid; grid-template-columns:minmax(0,1fr) auto; align-items:start; }
    .experience-era-trail { position:absolute; inset:0 auto 0 0; display:block; width:1.25rem; height:auto; margin:0; transform:none; border-inline-start:var(--line-signal) solid var(--pond-mint); }
    .experience-era-trail .signal-trail-svg { display:none; }
    @media (min-width:30rem) {
      .experience-overview-stats { grid-template-columns:repeat(2,minmax(0,1fr)); }
    }
    @media (min-width:48rem) {
      .experience-overview-stats { grid-template-columns:repeat(3,minmax(0,1fr)); }
    }
    @media (min-width:70rem) {
      .experience-orientation { grid-template-columns:minmax(18rem,0.78fr) minmax(0,1.22fr); }
      .experience-era-list { grid-template-columns:minmax(0,1fr); }
      .experience-era-layout { grid-template-columns:minmax(15rem,0.62fr) minmax(0,1.38fr); }
      .experience-era-heading { grid-column:1; grid-row:1; }
      .experience-role-list { grid-column:2; grid-row:1; align-items:start; }
    }
    @media (prefers-reduced-motion:reduce) {
      .experience-orientation-trail, .experience-orientation-trail * { animation:none!important; transition:none!important; }
      .experience-era-trail, .experience-era-trail * { animation:none!important; transition:none!important; }
    }
    @media (forced-colors:active) {
      .experience-era-trail { --signal-trail-forced-block-border:0; --signal-trail-forced-inline-border:var(--line-signal) solid CanvasText; border-block-start:0!important; border-inline-start:var(--line-signal) solid CanvasText!important; background:Canvas!important; color:CanvasText!important; }
      .experience-era-trail .signal-trail-svg { display:none!important; }
    }
  `

	tests := []struct {
		name string
		css  string
	}{
		{name: "compact summary starts with two columns", css: strings.Replace(valid, `grid-template-columns:minmax(0,1fr);`, `grid-template-columns:repeat(2,minmax(0,1fr));`, 1)},
		{name: "30rem summary jumps to three columns", css: strings.Replace(valid, `repeat(2,minmax(0,1fr))`, `repeat(3,minmax(0,1fr))`, 1)},
		{name: "48rem summary jumps to four columns", css: strings.Replace(valid, `repeat(3,minmax(0,1fr))`, `repeat(4,minmax(0,1fr))`, 1)},
		{name: "role card stretches", css: strings.Replace(valid, `.experience-role-card { min-width:0; align-self:start; height:auto;`, `.experience-role-card { min-width:0; align-self:stretch; height:100%;`, 1)},
		{name: "role date leaves heading row", css: strings.Replace(valid, `.experience-role-title-row { display:grid; grid-template-columns:minmax(0,1fr) auto;`, `.experience-role-title-row { display:block; grid-template-columns:minmax(0,1fr);`, 1)},
		{name: "trail inherits relative positioning", css: strings.Replace(valid, `position:absolute; inset:0 auto 0 0;`, `position:relative; inset:auto;`, 1)},
		{name: "old 68rem inner composition", css: strings.Replace(valid, `@media (min-width:70rem)`, `@media (min-width:68rem)`, 1)},
		{name: "wide orientation remains stacked", css: strings.Replace(valid, `grid-template-columns:minmax(18rem,0.78fr) minmax(0,1.22fr);`, `grid-template-columns:minmax(0,1fr);`, 1)},
		{name: "70rem roles placed before heading", css: strings.Replace(valid, `.experience-era-heading { grid-column:1; grid-row:1; }`, `.experience-era-heading { grid-column:2; grid-row:1; }`, 1)},
		{name: "70rem eras become horizontal", css: strings.Replace(valid, `.experience-era-list { grid-template-columns:minmax(0,1fr); }`, `.experience-era-list { grid-template-columns:repeat(3,minmax(0,1fr)); }`, 1)},
		{name: "forced colors lose vertical structural rule", css: strings.Replace(valid, `border-inline-start:var(--line-signal) solid CanvasText!important`, `border-inline-start:var(--line-signal) solid transparent!important`, 1)},
		{name: "reduced motion keeps animation", css: strings.Replace(valid, `animation:none!important; transition:none!important;`, `animation:trail-flow 2s infinite; transition:all 1s;`, 1)},
		{name: "second pseudo rail added", css: valid + `.experience-era-list::before { content:""; position:absolute; inset:0; border-inline-start:2px solid red; }`},
		{name: "track after owns alternate rail", css: valid + `.experience-era-track::after { content:""; position:absolute; inset:0; border-block-start:2px solid red; }`},
		{name: "single-colon track after owns alternate rail", css: valid + `.experience-era-track:after { content:""; position:absolute; inset:0; border-block-start:2px solid red; }`},
		{name: "uppercase track after owns alternate rail", css: valid + `.experience-era-track::AFTER { content:""; position:absolute; inset:0; border-block-start:2px solid red; }`},
		{name: "data sequence after owns alternate rail", css: valid + `[data-career-sequence]::after { content:""; position:absolute; inset:0; border-block-start:2px solid red; }`},
		{name: "valued data sequence after owns alternate rail", css: valid + `[data-career-sequence=""]::after { content:""; position:absolute; inset:0; border-block-start:2px solid red; }`},
		{name: "later 70rem heading override reorders composition", css: valid + `@media (min-width:70rem) { .experience-era-heading { grid-column:2; grid-row:1; } }`},
		{name: "noncanonical wide trail override", css: valid + `@media (min-width:80rem) { .experience-era-trail { width:100%; height:6rem; border-inline-start:0; } }`},
		{name: "higher-specificity wide heading override", css: valid + `@media (min-width:70rem) { .experience-page .experience-era-heading { grid-row:auto; } }`},
		{name: "functional selector wide heading override", css: valid + `@media (min-width:70rem) { :is(.experience-era-heading) { grid-row:auto; } }`},
		{name: "functional selector with ID arm", css: valid + `@media (min-width:70rem) { :is(#never, .experience-era-heading) { grid-row:auto; } }`},
		{name: "base important declaration defeats later breakpoint", css: valid + `.experience-era-heading { grid-row:auto!important; }`},
		{name: "comma media branch changes 70rem truth", css: strings.Replace(valid, `@media (min-width:70rem)`, `@media (min-width:70rem), (min-width:30rem)`, 1)},
		{name: "impossible 70rem media condition", css: strings.Replace(valid, `@media (min-width:70rem)`, `@media (min-width:70rem) and (max-width:69rem)`, 1)},
		{name: "forced colors comma branch leaks into ordinary mode", css: strings.Replace(valid, `@media (forced-colors:active)`, `@media (forced-colors:active), (min-width:0rem)`, 1)},
		{name: "required blocks wrapped in unsupported supports", css: strings.Replace(valid, `@media (min-width:70rem)`, `@supports (display:grid) { @media (min-width:70rem)`, 1) + `}`},
		{name: "nested media rule is ignored", css: valid + `.experience-era-heading { @media (min-width:70rem) { grid-row:auto; } }`},
		{name: "placement selector stretches second era", css: valid + `.experience-era-item:nth-child(2) { align-self:stretch; height:100%; }`},
		{name: "comment separates important keyword", css: `.experience-era-heading { grid-row:auto!/**/important; }` + valid},
		{name: "whitespace and comment separate important keyword", css: `.experience-era-heading { grid-row:auto ! /* priority */ important; }` + valid},
		{name: "case varied comment separated important keyword", css: `.experience-era-heading { grid-row:auto!/**/IMPORTANT; }` + valid},
		{name: "escaped important hex identifier", css: `.experience-era-heading { color:red !\69mportant; }` + valid},
		{name: "escaped important hex identifier with terminator whitespace", css: `.experience-era-heading { color:red !\69 mportant; }` + valid},
		{name: "escaped important six digit hex identifier", css: `.experience-era-heading { color:red !\000069mportant; }` + valid},
		{name: "escaped important case varied hex identifier", css: `.experience-era-heading { color:red !\49mportant; }` + valid},
		{name: "escaped important internal short hex identifier", css: `.experience-era-heading { color:red !i\6D portant; }` + valid},
		{name: "import statement at-rule", css: `@import url("override.css");` + valid},
		{name: "case varied import statement at-rule", css: `@IMPORT URL("override.css");` + valid},
		{name: "comment formatted import statement at-rule", css: `/* before */ @import/**/ url("override.css") /* tail */ ;` + valid},
		{name: "escaped import statement at-rule", css: `@\69mport url("override.css");` + valid},
		{name: "layer statement at-rule", css: `@layer route;` + valid},
		{name: "formatted case varied layer statement at-rule", css: "@LaYeR\n route /* tail */ ;" + valid},
		{name: "unknown statement at-rule", css: `@unknown fixture;` + valid},
		{name: "noncanonical 81rem override", css: valid + `@media (min-width:81rem) { .experience-era-heading { grid-row:1; } }`},
		{name: "range-syntax noncanonical 81rem override", css: valid + `@media (width >= 81rem) { .experience-era-heading { grid-row:1; } }`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateExperienceRouteCSS(test.css); err == nil {
				t.Fatal("validateExperienceRouteCSS() error = nil, want mutation rejected")
			}
		})
	}
	if err := validateExperienceRouteCSS(valid); err != nil {
		t.Fatalf("validateExperienceRouteCSS(valid) error = %v", err)
	}
}

func TestExperienceAtRuleValidatorAcceptsAtRuleTextInDeclarationValues(t *testing.T) {
	css := `.experience-era-heading {
		--experience-contact: craig@example.com;
		--experience-reference: @layer-is-text;
		background-image: url(https://example.com/@layer.css);
		content: "@import url('override.css');";
	}`
	if err := validateExperienceAtRuleContract(css); err != nil {
		t.Fatalf("validateExperienceAtRuleContract() rejected declaration text: %v", err)
	}
}

func TestExperienceImportantScannerIgnoresQuotedAndNonterminalText(t *testing.T) {
	for _, value := range []string{
		`"literal !important text"`,
		`"literal !\69mportant text"`,
		`literal-!important-text`,
		`!\69mportant is custom-property text`,
	} {
		t.Run(value, func(t *testing.T) {
			if experienceHasImportant(value) {
				t.Fatalf("experienceHasImportant(%q) = true, want false", value)
			}
		})
	}
}

func TestExperienceCSSIsImportedIntoPagesLayer(t *testing.T) {
	appCSS := readTask2Artifact(t, "cmd", "web", "tailwind", "app.css")
	commentFree, err := task2StripCSSComments(appCSS)
	if err != nil {
		t.Fatalf("parse app.css comments: %v", err)
	}
	const routeImport = `@import "./pages/experience.css" layer(pages);`
	if count := strings.Count(task2CanonicalCSS(commentFree), routeImport); count != 1 {
		t.Fatalf("Experience route import count = %d, want exactly one pages-layer import", count)
	}
}

func TestExperienceSelectorsHaveOneRouteOwner(t *testing.T) {
	legacyPaths := [][]string{
		{"cmd", "web", "tailwind", "base.css"},
		{"cmd", "web", "tailwind", "shared.css"},
		{"cmd", "web", "tailwind", "components.css"},
	}
	for _, path := range legacyPaths {
		css := readTask2Artifact(t, path...)
		rules, err := collectTask2StyleRules(css)
		if err != nil {
			t.Fatalf("parse legacy stylesheet %s: %v", strings.Join(path, "/"), err)
		}
		for _, rule := range rules {
			selector := task2CanonicalCSS(rule.header)
			if strings.Contains(selector, ".experience-") || strings.Contains(selector, ".page-kit-hero-experience") {
				t.Errorf("legacy stylesheet %s still owns Experience selector %q", strings.Join(path, "/"), rule.header)
			}
		}
	}
}

func TestExperienceForcedColorOrientationUsesSharedTrailHooks(t *testing.T) {
	baseCSS := readTask2Artifact(t, "cmd", "web", "tailwind", "base.css")
	rules, err := collectExperienceCSSRules(baseCSS, 0, false, false)
	if err != nil {
		t.Fatalf("parse base.css: %v", err)
	}
	want := map[string]string{
		"border-block-start":  "var(--signal-trail-forced-block-border,var(--line-signal) solid CanvasText)!important",
		"border-inline-start": "var(--signal-trail-forced-inline-border,0)!important",
	}
	if !experienceHasRule(rules, "[data-signal-trail]", 0, true, false, want) {
		t.Fatalf("shared forced-color trail fallback lacks route-controlled orientation hooks %v", want)
	}
}

func validateExperienceRouteCSS(css string) error {
	if err := validateExperienceAtRuleContract(css); err != nil {
		return fmt.Errorf("validate Experience at-rule contract: %w", err)
	}
	rules, err := collectExperienceCSSRules(css, 0, false, false)
	if err != nil {
		return fmt.Errorf("parse Experience route CSS: %w", err)
	}
	type requirement struct {
		label       string
		selector    string
		minWidthRem float64
		forced      bool
		reduced     bool
		want        map[string]string
	}
	required := []requirement{
		{label: "route spacing reset", selector: ".experience-page[data-layout='career-eras']", want: map[string]string{"display": "grid", "gap": "0"}},
		{label: "compact orientation", selector: ".experience-orientation", want: map[string]string{"position": "relative", "display": "grid", "grid-template-columns": "minmax(0,1fr)", "align-items": "stretch", "isolation": "isolate"}},
		{label: "orientation content layer", selector: ".experience-orientation > :not(.experience-orientation-trail)", want: map[string]string{"position": "relative", "z-index": "1"}},
		{label: "orientation typed underlay", selector: ".experience-orientation-trail", want: map[string]string{"position": "absolute", "z-index": "0", "display": "block", "width": "auto", "height": "auto", "margin": "0", "pointer-events": "none", "transform": "none"}},
		{label: "orientation SVG", selector: ".experience-orientation-trail .signal-trail-svg", want: map[string]string{"display": "block"}},
		{label: "compact summary", selector: ".experience-overview-stats", want: map[string]string{"display": "grid", "grid-template-columns": "minmax(0,1fr)"}},
		{label: "compact era sequence", selector: ".experience-era-list", want: map[string]string{"display": "grid", "grid-template-columns": "minmax(0,1fr)", "grid-template-rows": "auto", "align-items": "start"}},
		{label: "compact era reset", selector: ".experience-era-item", want: map[string]string{"min-width": "0", "grid-column": "auto", "grid-row": "auto", "align-self": "start", "height": "auto", "margin": "0", "transform": "none"}},
		{label: "anchor offset", selector: ".experience-era", want: map[string]string{"scroll-margin-top": "calc(var(--header-height) + var(--space-lg))"}},
		{label: "compact era interior", selector: ".experience-era-layout", want: map[string]string{"display": "grid", "grid-template-columns": "minmax(0,1fr)", "align-items": "start", "min-width": "0"}},
		{label: "era heading metadata row", selector: ".experience-era-title-row", want: map[string]string{"display": "flex", "flex-wrap": "wrap", "align-items": "baseline", "justify-content": "space-between"}},
		{label: "compact roles", selector: ".experience-role-list", want: map[string]string{"display": "grid", "grid-template-columns": "minmax(0,1fr)", "align-items": "start"}},
		{label: "natural role item", selector: ".experience-role-item", want: map[string]string{"min-width": "0", "align-self": "start", "height": "auto", "min-height": "0"}},
		{label: "natural role card", selector: ".experience-role-card", want: map[string]string{"min-width": "0", "align-self": "start", "height": "auto", "min-height": "0"}},
		{label: "role heading date row", selector: ".experience-role-title-row", want: map[string]string{"display": "grid", "grid-template-columns": "minmax(0,1fr) auto", "align-items": "start"}},
		{label: "compact vertical typed trail", selector: ".experience-era-trail", want: map[string]string{"position": "absolute", "inset": "0 auto 0 0", "display": "block", "width": "1.25rem", "height": "auto", "margin": "0", "transform": "none", "border-inline-start": "var(--line-signal) solid var(--pond-mint)"}},
		{label: "compact SVG reset", selector: ".experience-era-trail .signal-trail-svg", want: map[string]string{"display": "none"}},
		{label: "30rem summary", selector: ".experience-overview-stats", minWidthRem: 30, want: map[string]string{"grid-template-columns": "repeat(2,minmax(0,1fr))"}},
		{label: "48rem summary", selector: ".experience-overview-stats", minWidthRem: 48, want: map[string]string{"grid-template-columns": "repeat(3,minmax(0,1fr))"}},
		{label: "70rem paired orientation", selector: ".experience-orientation", minWidthRem: 70, want: map[string]string{"grid-template-columns": "minmax(18rem,0.78fr) minmax(0,1.22fr)"}},
		{label: "70rem single sequence", selector: ".experience-era-list", minWidthRem: 70, want: map[string]string{"grid-template-columns": "minmax(0,1fr)"}},
		{label: "70rem split interior", selector: ".experience-era-layout", minWidthRem: 70, want: map[string]string{"grid-template-columns": "minmax(15rem,0.62fr) minmax(0,1.38fr)"}},
		{label: "70rem heading placement", selector: ".experience-era-heading", minWidthRem: 70, want: map[string]string{"grid-column": "1", "grid-row": "1"}},
		{label: "70rem role placement", selector: ".experience-role-list", minWidthRem: 70, want: map[string]string{"grid-column": "2", "grid-row": "1", "align-items": "start"}},
		{label: "reduced trail", selector: ".experience-era-trail", reduced: true, want: map[string]string{"animation": "none!important", "transition": "none!important"}},
		{label: "reduced orientation trail", selector: ".experience-orientation-trail", reduced: true, want: map[string]string{"animation": "none!important", "transition": "none!important"}},
		{label: "forced vertical trail", selector: ".experience-era-trail", forced: true, want: map[string]string{"--signal-trail-forced-block-border": "0", "--signal-trail-forced-inline-border": "var(--line-signal) solid CanvasText", "border-block-start": "0!important", "border-inline-start": "var(--line-signal) solid CanvasText!important", "background": "Canvas!important", "color": "CanvasText!important"}},
		{label: "forced SVG removal", selector: ".experience-era-trail .signal-trail-svg", forced: true, want: map[string]string{"display": "none!important"}},
	}
	for _, item := range required {
		if !experienceHasEffectiveRule(rules, item.selector, item.minWidthRem, item.forced, item.reduced, item.want) {
			return fmt.Errorf("Experience CSS lacks %s rule %q at min-width %.0frem with declarations %v", item.label, item.selector, item.minWidthRem, item.want)
		}
	}
	allowedBreakpoints := map[float64]bool{0: true, 30: true, 48: true, 70: true}
	for _, rule := range rules {
		if !allowedBreakpoints[rule.minWidthRem] {
			return fmt.Errorf("Experience CSS uses noncanonical min-width %.3grem in selector %q", rule.minWidthRem, rule.selector)
		}
		if !rule.forcedColors && !rule.reducedMotion {
			for property, value := range rule.declarations {
				if experienceHasImportant(value) {
					return fmt.Errorf("Experience CSS uses !important outside accessibility media for %s in selector %q", property, rule.selector)
				}
			}
		}
	}
	placementProperties := map[string]bool{"grid-column": true, "grid-row": true}
	for _, rule := range rules {
		for _, selector := range task2SplitTopLevel(rule.selector, ',') {
			selector = task2CanonicalCSS(selector)
			if selector != ".experience-era-item:nth-child(1)" && selector != ".experience-era-item:nth-child(2)" && selector != ".experience-era-item:nth-child(3)" {
				continue
			}
			for property := range rule.declarations {
				if !placementProperties[property] {
					return fmt.Errorf("Experience placement selector %q owns non-placement declaration %q", selector, property)
				}
			}
		}
	}
	allowedSelectors := experienceAllowedRouteSelectors()
	for _, rule := range rules {
		for _, selector := range task2SplitTopLevel(rule.selector, ',') {
			selector = task2CanonicalCSS(selector)
			if !allowedSelectors[selector] {
				return fmt.Errorf("Experience CSS uses selector outside its closed route contract: %q", selector)
			}
		}
	}
	criticalTargets := []string{
		".experience-orientation",
		".experience-orientation-trail",
		".experience-overview-stats",
		".experience-era-list",
		".experience-era-item",
		".experience-era-layout",
		".experience-era-heading",
		".experience-role-list",
		".experience-role-item",
		".experience-role-card",
		".experience-role-title-row",
		".experience-era-trail",
	}
	allowedCriticalSelectors := map[string]bool{
		".experience-orientation": true,
		".experience-orientation > :not(.experience-orientation-trail)": true,
		".experience-orientation-trail":                                 true,
		".experience-orientation-trail *":                               true,
		".experience-orientation-trail .signal-trail-svg":               true,
		".experience-overview-stats":                                    true,
		".experience-era-list":                                          true,
		".experience-era-item":                                          true,
		".experience-era-item::before":                                  true,
		".experience-era-layout":                                        true,
		".experience-era-heading":                                       true,
		".experience-role-list":                                         true,
		".experience-role-item":                                         true,
		".experience-role-card":                                         true,
		".experience-role-title-row":                                    true,
		".experience-era-trail":                                         true,
		".experience-era-trail *":                                       true,
		".experience-era-trail .signal-trail-svg":                       true,
	}
	for _, rule := range rules {
		for _, selector := range task2SplitTopLevel(rule.selector, ',') {
			selector = task2CanonicalCSS(selector)
			for _, target := range criticalTargets {
				if experienceSelectorContainsClassToken(selector, target) && !allowedCriticalSelectors[selector] {
					return fmt.Errorf("Experience CSS uses unapproved selector %q for layout-critical target %q", selector, target)
				}
			}
		}
	}
	for _, rule := range rules {
		for _, selector := range task2SplitTopLevel(rule.selector, ',') {
			selector = experienceCanonicalPseudoSelector(selector)
			if !strings.Contains(selector, ".experience-") && !strings.Contains(selector, "[data-career-sequence") || !strings.Contains(selector, "::before") && !strings.Contains(selector, "::after") {
				continue
			}
			if selector != ".experience-era-item::before" {
				return fmt.Errorf("Experience CSS adds a competing pseudo-element rail/decorator through %q", selector)
			}
		}
	}
	return nil
}

func validateExperienceAtRuleContract(css string) error {
	if err := validateExperienceStatementAtRules(css); err != nil {
		return err
	}
	return validateExperienceAtRuleBlocks(css, false)
}

func validateExperienceStatementAtRules(css string) error {
	commentFreeCSS, err := task2StripCSSComments(css)
	if err != nil {
		return err
	}

	statementStart := 0
	parenthesisDepth := 0
	bracketDepth := 0
	for index := 0; index < len(commentFreeCSS); {
		switch commentFreeCSS[index] {
		case '\'', '"':
			next, stringErr := task2SkipCSSString(commentFreeCSS, index)
			if stringErr != nil {
				return stringErr
			}
			index = next
			continue
		case '(':
			parenthesisDepth++
		case ')':
			if parenthesisDepth > 0 {
				parenthesisDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '{', '}':
			if parenthesisDepth == 0 && bracketDepth == 0 {
				statementStart = index + 1
			}
		case ';':
			if parenthesisDepth == 0 && bracketDepth == 0 {
				if err := rejectExperienceStatementAtRule(commentFreeCSS[statementStart:index]); err != nil {
					return err
				}
				statementStart = index + 1
			}
		}
		index++
	}
	return rejectExperienceStatementAtRule(commentFreeCSS[statementStart:])
}

func rejectExperienceStatementAtRule(statement string) error {
	statement = strings.TrimSpace(statement)
	if strings.HasPrefix(statement, "@") {
		return fmt.Errorf("Experience CSS uses unsupported statement at-rule %q", statement)
	}
	return nil
}

func validateExperienceAtRuleBlocks(css string, nested bool) error {
	blocks, err := parseTask2CSSBlocks(css)
	if err != nil {
		return err
	}
	allowed := map[string]bool{
		"@media (min-width:30rem)":               true,
		"@media (min-width:48rem)":               true,
		"@media (min-width:70rem)":               true,
		"@media (prefers-reduced-motion:reduce)": true,
		"@media (forced-colors:active)":          true,
	}
	for _, block := range blocks {
		header := strings.TrimSpace(block.header)
		if strings.HasPrefix(header, "@") {
			canonical := experienceCanonicalAtRule(header)
			if nested {
				return fmt.Errorf("Experience CSS nests at-rule %q; at-rules must be top-level", header)
			}
			if !allowed[canonical] {
				return fmt.Errorf("Experience CSS uses unsupported at-rule %q", header)
			}
			if err := validateExperienceAtRuleBlocks(block.body, true); err != nil {
				return err
			}
			continue
		}
		if experienceBodyHasNestedBlock(block.body) {
			return fmt.Errorf("Experience selector %q contains an unsupported nested group rule", header)
		}
	}
	return nil
}

func experienceCanonicalAtRule(header string) string {
	canonical := strings.ToLower(task2CanonicalCSS(header))
	canonical = strings.ReplaceAll(canonical, ": ", ":")
	canonical = strings.ReplaceAll(canonical, " :", ":")
	return canonical
}

func experienceBodyHasNestedBlock(body string) bool {
	for index := 0; index < len(body); {
		if body[index] == '\'' || body[index] == '"' {
			next, err := task2SkipCSSString(body, index)
			if err != nil {
				return true
			}
			index = next
			continue
		}
		if body[index] == '{' || body[index] == '}' {
			return true
		}
		index++
	}
	return false
}

func experienceHasImportant(value string) bool {
	for index := 0; index < len(value); {
		if value[index] == '\'' || value[index] == '"' {
			next, err := task2SkipCSSString(value, index)
			if err != nil {
				return true
			}
			index = next
			continue
		}
		if value[index] != '!' {
			index++
			continue
		}
		keyword := skipExperienceCSSWhitespace(value, index+1)
		identifier, end := consumeExperienceCSSIdentifier(value, keyword)
		if strings.EqualFold(identifier, "important") && skipExperienceCSSWhitespace(value, end) == len(value) {
			return true
		}
		index++
	}
	return false
}

func consumeExperienceCSSIdentifier(value string, start int) (string, int) {
	var identifier strings.Builder
	index := start
	for index < len(value) {
		if isExperienceCSSIdentifierByte(value[index]) {
			identifier.WriteByte(value[index])
			index++
			continue
		}
		if value[index] != '\\' || index+1 >= len(value) {
			break
		}

		escapeStart := index + 1
		escapeEnd := escapeStart
		for escapeEnd < len(value) && escapeEnd-escapeStart < 6 && isExperienceCSSHexByte(value[escapeEnd]) {
			escapeEnd++
		}
		if escapeEnd > escapeStart {
			codePoint, err := strconv.ParseUint(value[escapeStart:escapeEnd], 16, 32)
			if err != nil || codePoint == 0 || codePoint > 0x10ffff || codePoint >= 0xd800 && codePoint <= 0xdfff {
				identifier.WriteRune('\uFFFD')
			} else {
				identifier.WriteRune(rune(codePoint))
			}
			index = consumeExperienceCSSEscapeTerminator(value, escapeEnd)
			continue
		}

		if value[escapeStart] == '\n' || value[escapeStart] == '\r' || value[escapeStart] == '\f' {
			break
		}
		identifier.WriteByte(value[escapeStart])
		index = escapeStart + 1
	}
	return identifier.String(), index
}

func isExperienceCSSHexByte(value byte) bool {
	return value >= '0' && value <= '9' || value >= 'a' && value <= 'f' || value >= 'A' && value <= 'F'
}

func consumeExperienceCSSEscapeTerminator(value string, index int) int {
	if index >= len(value) {
		return index
	}
	if value[index] == '\r' && index+1 < len(value) && value[index+1] == '\n' {
		return index + 2
	}
	if isExperienceCSSWhitespaceByte(value[index]) {
		return index + 1
	}
	return index
}

func skipExperienceCSSWhitespace(value string, index int) int {
	for index < len(value) && isExperienceCSSWhitespaceByte(value[index]) {
		index++
	}
	return index
}

func isExperienceCSSWhitespaceByte(value byte) bool {
	return value == ' ' || value == '\t' || value == '\r' || value == '\n' || value == '\f'
}

func collectExperienceCSSRules(css string, inheritedMinWidth float64, forcedColors, reducedMotion bool) ([]experienceCSSRule, error) {
	blocks, err := parseTask2CSSBlocks(css)
	if err != nil {
		return nil, err
	}
	var rules []experienceCSSRule
	for _, block := range blocks {
		header := strings.TrimSpace(block.header)
		if strings.HasPrefix(header, "@") {
			minWidth := inheritedMinWidth
			foundWidth := false
			for _, match := range aboutMinWidthPattern.FindAllStringSubmatch(header, -1) {
				foundWidth = true
				value, parseErr := strconv.ParseFloat(match[1], 64)
				if parseErr != nil {
					return nil, fmt.Errorf("parse media min-width %q: %w", match[1], parseErr)
				}
				if value > minWidth {
					minWidth = value
				}
			}
			for _, match := range experienceRangeWidthPattern.FindAllStringSubmatch(header, -1) {
				foundWidth = true
				value, parseErr := strconv.ParseFloat(match[1], 64)
				if parseErr != nil {
					return nil, fmt.Errorf("parse media range width %q: %w", match[1], parseErr)
				}
				if value > minWidth {
					minWidth = value
				}
			}
			if strings.Contains(strings.ToLower(header), "width") && !foundWidth {
				return nil, fmt.Errorf("unsupported Experience width media query %q", header)
			}
			canonical := strings.ReplaceAll(strings.ToLower(task2CanonicalCSS(header)), ": ", ":")
			forced := forcedColors || strings.Contains(canonical, "forced-colors:active")
			reduced := reducedMotion || strings.Contains(canonical, "prefers-reduced-motion:reduce")
			nested, nestedErr := collectExperienceCSSRules(block.body, minWidth, forced, reduced)
			if nestedErr != nil {
				return nil, nestedErr
			}
			rules = append(rules, nested...)
			continue
		}
		rules = append(rules, experienceCSSRule{selector: header, declarations: task2Declarations(block.body), minWidthRem: inheritedMinWidth, forcedColors: forcedColors, reducedMotion: reducedMotion})
	}
	return rules, nil
}

func experienceHasRule(rules []experienceCSSRule, selector string, minWidthRem float64, forced, reduced bool, want map[string]string) bool {
	for _, rule := range rules {
		if rule.minWidthRem != minWidthRem || rule.forcedColors != forced || rule.reducedMotion != reduced || !task2SelectorListContains(rule.selector, selector) {
			continue
		}
		matches := true
		for property, value := range want {
			if !task2CSSValueEqual(rule.declarations[property], value) {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

func experienceHasEffectiveRule(rules []experienceCSSRule, selector string, minWidthRem float64, forced, reduced bool, want map[string]string) bool {
	effective := make(map[string]string)
	matched := false
	for _, rule := range rules {
		if rule.minWidthRem > minWidthRem || rule.forcedColors && !forced || rule.reducedMotion && !reduced || !task2SelectorListContains(rule.selector, selector) {
			continue
		}
		matched = true
		for property, value := range rule.declarations {
			effective[property] = value
		}
	}
	if !matched {
		return false
	}
	for property, value := range want {
		if !task2CSSValueEqual(effective[property], value) {
			return false
		}
	}
	return true
}

func experienceSelectorContainsClassToken(selector, target string) bool {
	selector = task2CanonicalCSS(selector)
	target = task2CanonicalCSS(target)
	for start := 0; start < len(selector); {
		index := strings.Index(selector[start:], target)
		if index == -1 {
			return false
		}
		index += start
		after := index + len(target)
		if after == len(selector) || !isExperienceCSSIdentifierByte(selector[after]) {
			return true
		}
		start = after
	}
	return false
}

func isExperienceCSSIdentifierByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || value == '-' || value == '_'
}

func experienceCanonicalPseudoSelector(selector string) string {
	selector = strings.ToLower(task2CanonicalCSS(selector))
	selector = strings.ReplaceAll(selector, ":before", "::before")
	selector = strings.ReplaceAll(selector, ":after", "::after")
	selector = strings.ReplaceAll(selector, ":::before", "::before")
	selector = strings.ReplaceAll(selector, ":::after", "::after")
	return selector
}

func experienceAllowedRouteSelectors() map[string]bool {
	selectors := []string{
		".experience-page[data-layout='career-eras']",
		".experience-orientation",
		".experience-orientation > :not(.experience-orientation-trail)",
		".experience-orientation-trail",
		".experience-orientation-trail *",
		".experience-orientation-trail .signal-trail-svg",
		".experience-orientation-section",
		".experience-hero-content",
		".experience-hero-layout",
		".experience-overview-stats",
		".experience-overview-stat",
		".experience-overview-stat .page-kit-stat-value",
		".experience-overview-stat .page-kit-stat-label",
		".experience-overview-stat-technologies",
		".experience-era-sequence",
		".experience-era-index-section",
		".experience-era-index",
		".experience-era-index-heading",
		".experience-era-nav-list",
		".experience-era-link",
		".experience-era-link-mark",
		".experience-era-nav-list li:nth-child(1) .experience-era-link-mark",
		".experience-era-nav-list li:nth-child(2) .experience-era-link-mark",
		".experience-era-nav-list li:nth-child(3) .experience-era-link-mark",
		".experience-technology-strip",
		".experience-technology-panel",
		".experience-technology-heading",
		".experience-technology-description",
		".experience-technology-chips",
		".experience-technology-item",
		".experience-technology-item .page-kit-chip",
		".experience-sequence-heading",
		".experience-era-track",
		".experience-era-list",
		".experience-era-item",
		".experience-era-item-systems-growth",
		".experience-era-item-cloud-leadership",
		".experience-era-item::before",
		".experience-era",
		".experience-era-layout",
		".experience-era-heading",
		".experience-era-title-row",
		".experience-era-title",
		".experience-era-meta",
		".experience-era-range",
		".experience-era-summary",
		".experience-era-role-index",
		".experience-era-role-index li",
		".experience-status-pill-current",
		".experience-role-list",
		".experience-role-item",
		".experience-role-card",
		".experience-role-heading",
		".experience-role-title-row",
		".experience-role-title",
		".experience-role-duration",
		".experience-role-company",
		".experience-role-responsibilities",
		".experience-role-technologies",
		".experience-role-technologies .page-kit-chip",
		".experience-era-trail",
		".experience-era-trail *",
		".experience-era-trail .signal-trail-svg",
	}
	allowed := make(map[string]bool, len(selectors))
	for _, selector := range selectors {
		allowed[selector] = true
	}
	return allowed
}
