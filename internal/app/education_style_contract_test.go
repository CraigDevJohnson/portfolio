package app

import (
	"fmt"
	"strings"
	"testing"
)

func TestEducationRouteCSSContract(t *testing.T) {
	css := readTask2Artifact(t, "cmd", "web", "tailwind", "pages", "education.css")
	if err := validateEducationRouteCSS(css); err != nil {
		t.Fatal(err)
	}
}

func TestEducationRouteCSSValidatorRejectsRegressions(t *testing.T) {
	css := readTask2Artifact(t, "cmd", "web", "tailwind", "pages", "education.css")
	tests := []struct {
		name string
		old  string
		new  string
	}{
		{name: "domains stretch to equal heights", old: `align-items: start;`, new: `align-items: stretch;`},
		{name: "domain loses natural height", old: `height: auto;`, new: `height: 100%;`},
		{name: "tablet Linux placement overlaps", old: `grid-column: 1 / span 6;`, new: `grid-column: 1 / -1;`},
		{name: "navigation Microsoft placement loses span", old: `grid-column: 1 / span 8;`, new: `grid-column: auto;`},
		{name: "wide Delivery placement shifts left", old: `grid-column: 7 / -1;`, new: `grid-column: 1 / -1;`},
		{name: "layout uses inferred child position", old: `.education-domain[data-domain="cloud"] {`, new: `.education-domain:nth-child(1) {`},
		{name: "layout turns on dense packing", old: `grid-auto-flow: row;`, new: `grid-auto-flow: dense;`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutated := strings.Replace(css, test.old, test.new, 1)
			if mutated == css {
				t.Fatalf("mutation target %q not found", test.old)
			}
			if err := validateEducationRouteCSS(mutated); err == nil {
				t.Fatal("validateEducationRouteCSS() error = nil, want regression rejected")
			}
		})
	}

	for _, test := range []struct {
		name string
		css  string
	}{
		{name: "later wide override undoes Cloud placement", css: css + `@media (min-width: 80rem) { .education-domain[data-domain="cloud"] { grid-column: auto; } }`},
		{name: "higher specificity override undoes Security placement", css: css + `@media (min-width: 80rem) { .education-page .education-domain[data-domain="security"] { grid-row: 1; } }`},
		{name: "noncanonical breakpoint changes Delivery placement", css: css + `@media (min-width: 81rem) { .education-domain[data-domain="delivery"] { grid-column: 1 / -1; } }`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := validateEducationRouteCSS(test.css); err == nil {
				t.Fatal("validateEducationRouteCSS() error = nil, want regression rejected")
			}
		})
	}
}

func TestEducationCSSIsImportedIntoPagesLayer(t *testing.T) {
	appCSS := readTask2Artifact(t, "cmd", "web", "tailwind", "app.css")
	const routeImport = `@import "./pages/education.css" layer(pages);`
	if count := strings.Count(task2CanonicalCSS(appCSS), routeImport); count != 1 {
		t.Fatalf("Education route import count = %d, want exactly one pages-layer import", count)
	}
}

func TestEducationSelectorsHaveOneRouteOwner(t *testing.T) {
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
			if strings.Contains(selector, ".education-") {
				t.Errorf("legacy stylesheet %d still owns Education selector %q", index, rule.header)
			}
		}
	}
}

func validateEducationRouteCSS(css string) error {
	if strings.Contains(strings.ToLower(task2CanonicalCSS(css)), "nth-child") {
		return fmt.Errorf("Education route CSS uses forbidden nth-child inference")
	}
	rules, err := collectAboutCSSRules(css, 0, false)
	if err != nil {
		return fmt.Errorf("parse Education route CSS: %w", err)
	}

	required := []struct {
		selector string
		minWidth float64
		want     map[string]string
	}{
		{selector: ".education-domain-mosaic", want: map[string]string{"display": "grid", "grid-template-columns": "minmax(0,1fr)", "grid-auto-flow": "row", "align-items": "start"}},
		{selector: ".education-domains-section", want: map[string]string{"position": "relative", "isolation": "isolate"}},
		{selector: ".education-domains-intro", want: map[string]string{"position": "relative", "z-index": "1"}},
		{selector: ".education-domain-mosaic", want: map[string]string{"position": "relative", "z-index": "1"}},
		{selector: ".education-domain", want: map[string]string{"min-width": "0", "height": "auto", "align-self": "start", "grid-column": "auto", "grid-row": "auto"}},
		{selector: ".education-credential-grid", want: map[string]string{"display": "grid", "grid-template-columns": "minmax(0,1fr)", "align-items": "start"}},
		{selector: ".education-degree-logo.is-failed", want: map[string]string{"display": "none"}},
		{selector: ".education-degree-fallback", want: map[string]string{"display": "grid"}},
		{selector: ".education-field-guide-trail", want: map[string]string{"position": "absolute", "z-index": "0", "display": "block", "width": "var(--line-signal)", "height": "auto", "margin": "0", "pointer-events": "none", "transform": "none"}},
		{selector: ".education-field-guide-trail .signal-trail-svg", want: map[string]string{"display": "none"}},
		{selector: ".education-domain-mosaic", minWidth: 30, want: map[string]string{"grid-template-columns": "minmax(0,1fr)"}},
		{selector: ".education-domain", minWidth: 30, want: map[string]string{"grid-column": "auto", "grid-row": "auto"}},
		{selector: ".education-credential-grid", minWidth: 30, want: map[string]string{"grid-template-columns": "minmax(0,1fr)"}},
		{selector: ".education-domain-mosaic", minWidth: 48, want: map[string]string{"grid-template-columns": "repeat(12,minmax(0,1fr))"}},
		{selector: ".education-field-guide-trail", minWidth: 48, want: map[string]string{"width": "auto", "height": "auto", "background": "transparent", "transform": "scaleX(-1)"}},
		{selector: ".education-field-guide-trail .signal-trail-svg", minWidth: 48, want: map[string]string{"display": "block"}},
		{selector: `.education-domain[data-domain="cloud"]`, minWidth: 48, want: map[string]string{"grid-column": "1 / -1", "grid-row": "1"}},
		{selector: `.education-domain[data-domain="microsoft"]`, minWidth: 48, want: map[string]string{"grid-column": "1 / -1", "grid-row": "2"}},
		{selector: `.education-domain[data-domain="linux"]`, minWidth: 48, want: map[string]string{"grid-column": "1 / span 6", "grid-row": "3"}},
		{selector: `.education-domain[data-domain="security"]`, minWidth: 48, want: map[string]string{"grid-column": "7 / -1", "grid-row": "3"}},
		{selector: `.education-domain[data-domain="delivery"]`, minWidth: 48, want: map[string]string{"grid-column": "1 / -1", "grid-row": "4"}},
		{selector: `.education-domain[data-domain="cloud"] .education-credential-grid`, minWidth: 48, want: map[string]string{"grid-template-columns": "repeat(3,minmax(0,1fr))"}},
		{selector: `.education-domain[data-domain="microsoft"] .education-credential-grid`, minWidth: 48, want: map[string]string{"grid-template-columns": "repeat(2,minmax(0,1fr))"}},
		{selector: `.education-domain[data-domain="linux"] .education-credential-grid`, minWidth: 48, want: map[string]string{"grid-template-columns": "minmax(0,1fr)"}},
		{selector: `.education-domain[data-domain="security"] .education-credential-grid`, minWidth: 48, want: map[string]string{"grid-template-columns": "minmax(0,1fr)"}},
		{selector: `.education-domain[data-domain="delivery"] .education-credential-grid`, minWidth: 48, want: map[string]string{"grid-template-columns": "repeat(3,minmax(0,1fr))"}},
		{selector: ".education-credential-card", minWidth: 48, want: map[string]string{"grid-template-columns": "minmax(0,1fr)", "align-items": "start"}},
		{selector: ".education-credential-arrow", minWidth: 48, want: map[string]string{"position": "absolute", "inset-block-start": "var(--space-md)", "inset-inline-end": "var(--space-md)"}},
		{selector: `.education-domain[data-domain="cloud"]`, minWidth: 70, want: map[string]string{"grid-column": "1 / -1", "grid-row": "1"}},
		{selector: `.education-domain[data-domain="microsoft"]`, minWidth: 70, want: map[string]string{"grid-column": "1 / span 8", "grid-row": "2"}},
		{selector: `.education-domain[data-domain="linux"]`, minWidth: 70, want: map[string]string{"grid-column": "9 / -1", "grid-row": "2"}},
		{selector: `.education-domain[data-domain="security"]`, minWidth: 70, want: map[string]string{"grid-column": "1 / span 4", "grid-row": "3"}},
		{selector: `.education-domain[data-domain="delivery"]`, minWidth: 70, want: map[string]string{"grid-column": "5 / -1", "grid-row": "3"}},
		{selector: `.education-domain[data-domain="cloud"] .education-credential-grid`, minWidth: 70, want: map[string]string{"grid-template-columns": "repeat(3,minmax(0,1fr))"}},
		{selector: `.education-domain[data-domain="microsoft"] .education-credential-grid`, minWidth: 70, want: map[string]string{"grid-template-columns": "repeat(2,minmax(0,1fr))"}},
		{selector: `.education-domain[data-domain="linux"] .education-credential-grid`, minWidth: 70, want: map[string]string{"grid-template-columns": "minmax(0,1fr)"}},
		{selector: `.education-domain[data-domain="security"] .education-credential-grid`, minWidth: 70, want: map[string]string{"grid-template-columns": "minmax(0,1fr)"}},
		{selector: `.education-domain[data-domain="delivery"] .education-credential-grid`, minWidth: 70, want: map[string]string{"grid-template-columns": "repeat(3,minmax(0,1fr))"}},
		{selector: `.education-domain[data-domain="cloud"]`, minWidth: 80, want: map[string]string{"grid-column": "1 / span 7", "grid-row": "1"}},
		{selector: `.education-domain[data-domain="microsoft"]`, minWidth: 80, want: map[string]string{"grid-column": "8 / -1", "grid-row": "1"}},
		{selector: `.education-domain[data-domain="linux"]`, minWidth: 80, want: map[string]string{"grid-column": "1 / span 3", "grid-row": "2"}},
		{selector: `.education-domain[data-domain="security"]`, minWidth: 80, want: map[string]string{"grid-column": "4 / span 3", "grid-row": "2"}},
		{selector: `.education-domain[data-domain="delivery"]`, minWidth: 80, want: map[string]string{"grid-column": "7 / -1", "grid-row": "2"}},
		{selector: `.education-domain[data-domain="cloud"] .education-credential-grid`, minWidth: 80, want: map[string]string{"grid-template-columns": "repeat(3,minmax(0,1fr))"}},
		{selector: `.education-domain[data-domain="microsoft"] .education-credential-grid`, minWidth: 80, want: map[string]string{"grid-template-columns": "repeat(2,minmax(0,1fr))"}},
		{selector: `.education-domain[data-domain="linux"] .education-credential-grid`, minWidth: 80, want: map[string]string{"grid-template-columns": "minmax(0,1fr)"}},
		{selector: `.education-domain[data-domain="security"] .education-credential-grid`, minWidth: 80, want: map[string]string{"grid-template-columns": "minmax(0,1fr)"}},
		{selector: `.education-domain[data-domain="delivery"] .education-credential-grid`, minWidth: 80, want: map[string]string{"grid-template-columns": "repeat(3,minmax(0,1fr))"}},
	}
	for _, requirement := range required {
		if !educationHasEffectiveRule(rules, requirement.selector, requirement.minWidth, requirement.want) {
			return fmt.Errorf("Education route CSS lacks %q at min-width %.0frem with %v", requirement.selector, requirement.minWidth, requirement.want)
		}
	}

	allowedLayoutSelectors := map[string]bool{
		".education-domain-mosaic":                                              true,
		".education-domain":                                                     true,
		".education-domain::after":                                              true,
		".education-credential-grid":                                            true,
		`.education-domain[data-domain="cloud"]`:                                true,
		`.education-domain[data-domain="microsoft"]`:                            true,
		`.education-domain[data-domain="linux"]`:                                true,
		`.education-domain[data-domain="security"]`:                             true,
		`.education-domain[data-domain="delivery"]`:                             true,
		`.education-domain[data-domain="cloud"] .education-credential-grid`:     true,
		`.education-domain[data-domain="microsoft"] .education-credential-grid`: true,
		`.education-domain[data-domain="linux"] .education-credential-grid`:     true,
		`.education-domain[data-domain="security"] .education-credential-grid`:  true,
		`.education-domain[data-domain="delivery"] .education-credential-grid`:  true,
	}
	allowedMinWidths := map[float64]bool{0: true, 30: true, 48: true, 70: true, 80: true}
	layoutProperties := map[string]bool{
		"align-items":           true,
		"align-self":            true,
		"grid-auto-flow":        true,
		"grid-column":           true,
		"grid-row":              true,
		"grid-template-columns": true,
		"height":                true,
	}
	for _, rule := range rules {
		hasLayoutDeclaration := false
		for property := range rule.declarations {
			if layoutProperties[property] {
				hasLayoutDeclaration = true
				break
			}
		}
		if !hasLayoutDeclaration {
			continue
		}
		for _, selector := range task2SplitTopLevel(rule.selector, ',') {
			selector = task2CanonicalCSS(selector)
			if !educationSelectorContainsClassToken(selector, ".education-domain") &&
				!educationSelectorContainsClassToken(selector, ".education-domain-mosaic") &&
				!educationSelectorContainsClassToken(selector, ".education-credential-grid") {
				continue
			}
			if !allowedLayoutSelectors[selector] {
				return fmt.Errorf("Education route CSS uses inferred or unsupported layout selector %q", selector)
			}
			if !allowedMinWidths[rule.minWidthRem] {
				return fmt.Errorf("Education route CSS uses noncanonical %.0frem layout breakpoint for %q", rule.minWidthRem, selector)
			}
		}
	}
	return nil
}

func educationHasEffectiveRule(rules []aboutCSSRule, selector string, targetWidthRem float64, want map[string]string) bool {
	effective := make(map[string]string)
	found := false
	for _, rule := range rules {
		if rule.forcedColors || rule.minWidthRem > targetWidthRem || !task2SelectorListContains(rule.selector, selector) {
			continue
		}
		found = true
		for property, value := range rule.declarations {
			effective[property] = value
		}
	}
	if !found {
		return false
	}
	for property, value := range want {
		if !task2CSSValueEqual(effective[property], value) {
			return false
		}
	}
	return true
}

func educationSelectorContainsClassToken(selector, target string) bool {
	for remainder := selector; ; {
		index := strings.Index(remainder, target)
		if index == -1 {
			return false
		}
		end := index + len(target)
		if end == len(remainder) || !educationCSSIdentifierByte(remainder[end]) {
			return true
		}
		remainder = remainder[end:]
	}
}

func educationCSSIdentifierByte(value byte) bool {
	return value == '-' || value == '_' || value >= '0' && value <= '9' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}
