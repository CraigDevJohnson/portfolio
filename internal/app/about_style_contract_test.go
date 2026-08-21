package app

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const aboutDenseCompositionRem = 70

var aboutMinWidthPattern = regexp.MustCompile(`(?i)min-width\s*:\s*([0-9]+(?:\.[0-9]+)?)rem`)

type aboutCSSRule struct {
	selector     string
	declarations map[string]string
	minWidthRem  float64
	forcedColors bool
}

func TestAboutRouteCSSContract(t *testing.T) {
	css := readTask2Artifact(t, "cmd", "web", "tailwind", "pages", "about.css")
	if err := validateAboutRouteCSS(css); err != nil {
		t.Fatal(err)
	}
}

func TestAboutRouteCSSValidatorRejectsRegressions(t *testing.T) {
	const valid = `
    .about-switchback {
      position: relative;
      display: grid;
      grid-template-columns: minmax(0, 1fr);
      grid-template-rows: auto;
      min-width: 0;
      isolation: isolate;
    }
    .about-switchback > * {
      position: relative;
      z-index: 1;
      min-width: 0;
      grid-column: auto;
      grid-row: auto;
    }
    .about-story-copy { max-width: 62ch; }
    .about-quick-facts {
      position: static;
      inset: auto;
      min-width: 0;
      margin: 0;
      align-self: stretch;
      transform: none;
    }
    .about-timeline-trail {
      position: absolute;
      z-index: 0;
      inset: var(--space-md) 0;
      display: block;
      width: auto;
      height: auto;
      margin: 0;
      pointer-events: none;
      transform: none;
    }
    .about-timeline-track { z-index: 1; }
    .about-timeline-list { z-index: 2; }
    .about-timeline-entry {
      background: color-mix(in srgb, var(--cocoa-cedar) 94%, var(--night-mulberry));
    }
    .about-hobby-strip {
      display: grid;
      grid-template-columns: minmax(0, 1fr);
    }
    .about-hobby-strip > * {
      min-width: 0;
      grid-column: auto;
      grid-row: auto;
    }
    @media (min-width: 48rem) {
      .about-hobby-strip { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    }
    @media (min-width: 70rem) {
      .about-hero.page-hero-narrative .page-kit-hero-photo {
        right: var(--space-lg);
        left: auto;
        width: 43%;
        transform: rotate(0.4deg);
      }
      .about-hero.page-hero-narrative .page-kit-hero-content {
        width: 60%;
        margin-left: 0;
      }
      .about-switchback {
        grid-template-columns: minmax(0, 1fr) minmax(17rem, 20rem);
        grid-template-rows: repeat(2, auto);
      }
      .about-quick-facts {
        position: sticky;
        inset: auto;
        top: calc(var(--header-height) + var(--space-lg));
        align-self: start;
        grid-column: 2;
        grid-row: 1 / 3;
      }
    }
    @media (min-width: 80rem) {
      .about-hobby-strip { grid-template-columns: repeat(4, minmax(0, 1fr)); }
    }
    @media (forced-colors: active) {
      .about-timeline-trail {
        border-block-start: var(--line-signal) solid CanvasText !important;
        color: CanvasText !important;
      }
      .about-timeline-trail .signal-trail-svg { display: none !important; }
    }
  `

	tests := []struct {
		name string
		css  string
	}{
		{name: "collapsed compact story track", css: strings.Replace(valid, `grid-template-columns: minmax(0, 1fr);`, `grid-template-columns: 2.375rem minmax(0, 1fr);`, 1)},
		{name: "facts sticky below dense boundary", css: strings.Replace(valid, `position: static;`, `position: sticky;`, 1)},
		{name: "facts transform not reset", css: strings.Replace(valid, `transform: none;`, `transform: translateX(2rem);`, 1)},
		{name: "trail loses background positioning", css: strings.Replace(valid, `.about-timeline-trail {
      position: absolute;`, `.about-switchback-trail {
      position: relative;`, 1)},
		{name: "trail inset is not composition owned", css: strings.Replace(valid, `inset: var(--space-md) 0;
      display: block;`, `inset: 10rem 2rem auto;
      display: block;`, 1)},
		{name: "timeline cards become transparent", css: strings.Replace(valid, `background: color-mix(in srgb, var(--cocoa-cedar) 94%, var(--night-mulberry));`, `background: transparent;`, 1)},
		{name: "about hero returns to left", css: strings.Replace(valid, `right: var(--space-lg);
        left: auto;`, `right: auto;
        left: var(--space-lg);`, 1)},
		{name: "story line length too long", css: strings.Replace(valid, `max-width: 62ch;`, `max-width: 90ch;`, 1)},
		{name: "legacy dense threshold", css: strings.Replace(valid, `min-width: 70rem`, `min-width: 68rem`, 1)},
		{name: "wide facts lack header-aware offset", css: strings.Replace(valid, `calc(var(--header-height) + var(--space-lg))`, `2rem`, 1)},
		{name: "tablet hobbies use implicit twelve-track placement", css: strings.Replace(valid, `repeat(2, minmax(0, 1fr))`, `repeat(12, minmax(0, 1fr))`, 1)},
		{name: "wide hobbies never become four explicit tracks", css: strings.Replace(valid, `repeat(4, minmax(0, 1fr))`, `repeat(2, minmax(0, 1fr))`, 1)},
		{name: "forced colors lose structural rule", css: strings.Replace(valid, `solid CanvasText !important`, `solid transparent !important`, 1)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateAboutRouteCSS(test.css); err == nil {
				t.Fatal("validateAboutRouteCSS() error = nil, want regression rejected")
			}
		})
	}

	if err := validateAboutRouteCSS(valid); err != nil {
		t.Fatalf("validateAboutRouteCSS(valid) error = %v", err)
	}
}

func TestAboutCSSIsImportedIntoPagesLayer(t *testing.T) {
	appCSS := readTask2Artifact(t, "cmd", "web", "tailwind", "app.css")
	commentFree, err := task2StripCSSComments(appCSS)
	if err != nil {
		t.Fatalf("parse app.css comments: %v", err)
	}
	const routeImport = `@import "./pages/about.css" layer(pages);`
	if count := strings.Count(task2CanonicalCSS(commentFree), routeImport); count != 1 {
		t.Fatalf("About route import count = %d, want exactly one pages-layer import", count)
	}
}

func TestAboutSelectorsHaveOneRouteOwner(t *testing.T) {
	legacyFiles := []string{
		readTask2Artifact(t, "cmd", "web", "tailwind", "shared.css"),
		readTask2Artifact(t, "cmd", "web", "tailwind", "base.css"),
		readTask2Artifact(t, "cmd", "web", "tailwind", "components.css"),
	}
	for index, css := range legacyFiles {
		rules, err := collectTask2StyleRules(css)
		if err != nil {
			t.Fatalf("parse legacy stylesheet %d: %v", index, err)
		}
		for _, rule := range rules {
			selector := task2CanonicalCSS(rule.header)
			if strings.Contains(selector, ".about-") || strings.Contains(selector, ".page-kit-hero-about") {
				t.Errorf("legacy stylesheet %d still owns About selector %q", index, rule.header)
			}
		}
	}
}

func TestForcedColorsKeepSignalTrailStructuralRoot(t *testing.T) {
	accessibilityCSS := readTask2Artifact(t, "cmd", "web", "tailwind", "base.css")
	rules, err := collectTask2ForcedColorRules(accessibilityCSS)
	if err != nil {
		t.Fatalf("parse forced-color rules: %v", err)
	}

	hidesSVG := false
	for _, rule := range rules {
		if !task2CSSValueEqual(task2Declarations(rule.body)["display"], "none !important") {
			continue
		}
		for _, selector := range task2SplitTopLevel(rule.header, ',') {
			selector = task2CanonicalCSS(selector)
			if strings.Contains(selector, ".signal-trail-svg") {
				hidesSVG = true
				continue
			}
			if aboutSelectorTargetsSignalTrailRoot(selector) {
				t.Errorf("forced colors hide the structural signal-trail root through %q", selector)
			}
		}
	}
	if !hidesSVG {
		t.Error("forced colors do not hide the decorative signal-trail SVG")
	}
}

func TestFocusedSkipLinkMeetsMinimumTargetSize(t *testing.T) {
	baseCSS := readTask2Artifact(t, "cmd", "web", "tailwind", "base.css")
	rules, err := collectAboutCSSRules(baseCSS, 0, false)
	if err != nil {
		t.Fatalf("parse base stylesheet: %v", err)
	}
	if !aboutHasRule(rules, ".site-skip-link:focus-visible", 0, false, map[string]string{
		"display":        "inline-flex",
		"min-block-size": "2.75rem",
		"align-items":    "center",
	}) {
		t.Error("focused skip link lacks an explicit 44px minimum target size")
	}
}

func validateAboutRouteCSS(css string) error {
	rules, err := collectAboutCSSRules(css, 0, false)
	if err != nil {
		return fmt.Errorf("parse About route CSS: %w", err)
	}

	type requiredRule struct {
		label       string
		selector    string
		minWidthRem float64
		forced      bool
		want        map[string]string
	}
	required := []requiredRule{
		{label: "compact switchback", selector: ".about-switchback", want: map[string]string{"position": "relative", "display": "grid", "grid-template-columns": "minmax(0,1fr)", "grid-template-rows": "auto", "min-width": "0", "isolation": "isolate"}},
		{label: "compact child reset", selector: ".about-switchback > *", want: map[string]string{"position": "relative", "z-index": "1", "min-width": "0", "grid-column": "auto", "grid-row": "auto"}},
		{label: "readable story", selector: ".about-story-copy", want: map[string]string{"max-width": "62ch"}},
		{label: "compact facts reset", selector: ".about-quick-facts", want: map[string]string{"position": "static", "inset": "auto", "min-width": "0", "margin": "0", "align-self": "stretch", "transform": "none"}},
		{label: "typed timeline trail underlay", selector: ".about-timeline-trail", want: map[string]string{"position": "absolute", "z-index": "0", "inset": "var(--space-md) 0", "display": "block", "width": "auto", "height": "auto", "margin": "0", "pointer-events": "none", "transform": "none"}},
		{label: "timeline track above trail", selector: ".about-timeline-track", want: map[string]string{"z-index": "1"}},
		{label: "timeline cards above trail", selector: ".about-timeline-list", want: map[string]string{"z-index": "2"}},
		{label: "opaque timeline card", selector: ".about-timeline-entry", want: map[string]string{"background": "color-mix(in srgb,var(--cocoa-cedar) 94%,var(--night-mulberry))"}},
		{label: "compact hobby track", selector: ".about-hobby-strip", want: map[string]string{"display": "grid", "grid-template-columns": "minmax(0,1fr)"}},
		{label: "hobby child reset", selector: ".about-hobby-strip > *", want: map[string]string{"min-width": "0", "grid-column": "auto", "grid-row": "auto"}},
		{label: "tablet hobby track", selector: ".about-hobby-strip", minWidthRem: 48, want: map[string]string{"grid-template-columns": "repeat(2,minmax(0,1fr))"}},
		{label: "right-oriented About hero image", selector: ".about-hero.page-hero-narrative .page-kit-hero-photo", minWidthRem: 70, want: map[string]string{"right": "var(--space-lg)", "left": "auto", "width": "43%", "transform": "rotate(0.4deg)"}},
		{label: "right-oriented About hero content", selector: ".about-hero.page-hero-narrative .page-kit-hero-content", minWidthRem: 70, want: map[string]string{"width": "60%", "margin-left": "0"}},
		{label: "wide switchback", selector: ".about-switchback", minWidthRem: 70, want: map[string]string{"grid-template-columns": "minmax(0,1fr) minmax(17rem,20rem)", "grid-template-rows": "repeat(2,auto)"}},
		{label: "wide sticky facts", selector: ".about-quick-facts", minWidthRem: 70, want: map[string]string{"position": "sticky", "inset": "auto", "top": "calc(var(--header-height) + var(--space-lg))", "align-self": "start", "grid-column": "2", "grid-row": "1 / 3"}},
		{label: "wide hobby track", selector: ".about-hobby-strip", minWidthRem: 80, want: map[string]string{"grid-template-columns": "repeat(4,minmax(0,1fr))"}},
		{label: "forced-color structural trail", selector: ".about-timeline-trail", forced: true, want: map[string]string{"border-block-start": "var(--line-signal) solid CanvasText !important", "color": "CanvasText !important"}},
		{label: "forced-color SVG removal", selector: ".about-timeline-trail .signal-trail-svg", forced: true, want: map[string]string{"display": "none !important"}},
	}

	for _, requirement := range required {
		if !aboutHasRule(rules, requirement.selector, requirement.minWidthRem, requirement.forced, requirement.want) {
			return fmt.Errorf("About CSS lacks %s rule %q at min-width %.0frem with declarations %v", requirement.label, requirement.selector, requirement.minWidthRem, requirement.want)
		}
	}
	return nil
}

func collectAboutCSSRules(css string, inheritedMinWidth float64, forcedColors bool) ([]aboutCSSRule, error) {
	blocks, err := parseTask2CSSBlocks(css)
	if err != nil {
		return nil, err
	}

	var rules []aboutCSSRule
	for _, block := range blocks {
		header := strings.TrimSpace(block.header)
		if strings.HasPrefix(header, "@") {
			minWidth := inheritedMinWidth
			for _, match := range aboutMinWidthPattern.FindAllStringSubmatch(header, -1) {
				value, parseErr := strconv.ParseFloat(match[1], 64)
				if parseErr != nil {
					return nil, fmt.Errorf("parse media min-width %q: %w", match[1], parseErr)
				}
				if value > minWidth {
					minWidth = value
				}
			}
			forced := forcedColors || strings.Contains(strings.ToLower(task2CanonicalCSS(header)), "forced-colors: active")
			nested, nestedErr := collectAboutCSSRules(block.body, minWidth, forced)
			if nestedErr != nil {
				return nil, nestedErr
			}
			rules = append(rules, nested...)
			continue
		}
		rules = append(rules, aboutCSSRule{selector: header, declarations: task2Declarations(block.body), minWidthRem: inheritedMinWidth, forcedColors: forcedColors})
	}
	return rules, nil
}

func aboutHasRule(rules []aboutCSSRule, selector string, minWidthRem float64, forced bool, want map[string]string) bool {
	for _, rule := range rules {
		if rule.minWidthRem != minWidthRem || rule.forcedColors != forced || !task2SelectorListContains(rule.selector, selector) {
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

func aboutSelectorTargetsSignalTrailRoot(selector string) bool {
	lastCombinator := -1
	for _, combinator := range []string{" ", ">", "+", "~"} {
		if index := strings.LastIndex(selector, combinator); index > lastCombinator {
			lastCombinator = index
		}
	}
	target := selector[lastCombinator+1:]
	if strings.Contains(target, ".signal-trail-svg") {
		return false
	}
	return regexp.MustCompile(`\.signal-trail(?:[^a-zA-Z0-9_-]|$)`).MatchString(target)
}
