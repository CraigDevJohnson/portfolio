package app

import (
	"fmt"
	"strings"
	"testing"
)

func TestPortalRouteCSSContract(t *testing.T) {
	css := readTask2Artifact(t, "cmd", "web", "tailwind", "portal.css")
	if err := validatePortalRouteCSS(css); err != nil {
		t.Fatal(err)
	}
}

func TestPortalRouteCSSValidatorRejectsCardTableRegressions(t *testing.T) {
	css := readTask2Artifact(t, "cmd", "web", "tailwind", "portal.css")
	mutations := []struct {
		name        string
		old         string
		replacement string
	}{
		{"mobile header removed from accessibility tree", "clip-path: inset(50%);", "display: none;"},
		{"mobile table keeps desktop width", "min-width: 0;", "min-width: 66rem;"},
		{"long values stop wrapping", "overflow-wrap: anywhere;\n  text-align: start;", "overflow-wrap: normal;\n  text-align: start;"},
		{"tablet table transition drifts", "min-width: 48rem", "min-width: 44rem"},
		{"wide table never restores semantics", "display: table;", "display: block;"},
		{"expanded desktop detail is not a table row", "display: table-row;", "display: block;"},
		{"compact actions stop stacking", "grid-template-columns: minmax(0, 1fr);", "grid-template-columns: repeat(3, minmax(0, 1fr));"},
		{"portal actions shrink below the minimum target width", "min-inline-size: 2.75rem;", "min-inline-size: 0;"},
		{"desktop overflow region stops revealing the first keyboard target", "scroll-padding-inline: var(--space-lg);", "scroll-padding-inline: 0;"},
		{"later desktop rule overrides keyboard reveal padding", "__append__", "\n@media (min-width: 48rem) {\n  .portal-instance-table-shell .overflow-region {\n    scroll-padding-inline: 0;\n  }\n}\n"},
		{"later desktop shorthand overrides keyboard reveal padding", "__append__", "\n@media (min-width: 48rem) {\n  .portal-instance-table-shell .overflow-region {\n    scroll-padding: 0;\n  }\n}\n"},
		{"higher specificity desktop rule overrides keyboard reveal padding", "__append__", "\n@media (min-width: 48rem) {\n  .portal-page .portal-instance-table-shell .overflow-region {\n    scroll-padding-inline: 0;\n  }\n}\n"},
		{"direct child desktop rule overrides keyboard reveal padding", "__append__", "\n@media (min-width: 48rem) {\n  .portal-instance-table-shell > .overflow-region {\n    scroll-padding-inline: 0;\n  }\n}\n"},
		{"id alias overrides keyboard reveal padding", "__append__", "\n@media (min-width: 48rem) {\n  #portal-instance-overflow {\n    scroll-padding-inline: 0;\n  }\n}\n"},
		{"dashboard feedback stops containing diagnostics", ".portal-console-feedback {\n  min-width: 0;\n  max-width: 54rem;\n  overflow-wrap: anywhere;", ".portal-console-feedback {\n  max-width: 54rem;"},
		{"error feedback stops containing diagnostics", ".portal-error-feedback {\n  min-width: 0;\n  max-width: 46rem;\n  overflow-wrap: anywhere;", ".portal-error-feedback {\n  max-width: 46rem;"},
		{"operator title escapes its grid column", "width: 100%;\n  max-width: min(12ch, 100%);\n  font-size: clamp(3rem, 5.2vw, 5rem);\n  overflow-wrap: anywhere;", "max-width: 12ch;"},
		{"operator title splits at the wide-shell seam", "font-size: clamp(3rem, 5.2vw, 5rem);", "font-size: inherit;"},
		{"compact forced row loses opaque Canvas ownership", "background: Canvas !important;\n    color: CanvasText !important;\n    box-shadow: none !important;", "background: transparent !important;\n    color: CanvasText !important;\n    box-shadow: none !important;"},
		{"later compact forced row restores transparent paint", "__append__", "\n@media (forced-colors: active) { .portal-instance-row { background: transparent !important; } }\n"},
		{"higher-specificity compact forced row restores transparent paint", "__append__", "\n@media (forced-colors: active) { .portal-page .portal-instance-row { background: transparent !important; } }\n"},
		{"later compact forced row resets every required paint", "__append__", "\n@media (forced-colors: active) { .portal-instance-row { all: revert !important; } }\n"},
		{"compact forced row owner moves behind the desktop breakpoint", ".portal-instance-row {\n    border-color: CanvasText !important;\n    background: Canvas !important;\n    color: CanvasText !important;\n    box-shadow: none !important;\n  }", "@media (min-width: 48rem) {\n    .portal-instance-row {\n      border-color: CanvasText !important;\n      background: Canvas !important;\n      color: CanvasText !important;\n      box-shadow: none !important;\n    }\n  }"},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			mutated := css + mutation.replacement
			if mutation.old != "__append__" {
				mutated = strings.Replace(css, mutation.old, mutation.replacement, 1)
			}
			if mutated == css {
				t.Fatalf("mutation target %q not found", mutation.old)
			}
			if err := validatePortalRouteCSS(mutated); err == nil {
				t.Fatal("validatePortalRouteCSS() error = nil, want regression rejected")
			}
		})
	}
}

func validatePortalRouteCSS(css string) error {
	rules, err := collectAboutCSSRules(css, 0, false)
	if err != nil {
		return fmt.Errorf("parse Portal route CSS: %w", err)
	}
	required := []struct {
		selector string
		width    float64
		want     map[string]string
	}{
		{selector: ".portal-page", want: map[string]string{"position": "relative", "isolation": "isolate", "min-width": "0"}},
		{selector: ".portal-operator-header", want: map[string]string{"position": "relative", "z-index": "1"}},
		{selector: ".portal-console-main", want: map[string]string{"position": "relative", "z-index": "1"}},
		{selector: ".portal-operator-trail", want: map[string]string{"position": "absolute", "z-index": "0", "display": "block", "width": "var(--line-signal)", "height": "auto", "pointer-events": "none", "transform": "none"}},
		{selector: ".portal-operator-trail .signal-trail-svg", want: map[string]string{"display": "none"}},
		{selector: ".portal-instance-table-shell .overflow-region-hint", want: map[string]string{"display": "none"}},
		{selector: ".portal-instance-table-shell .overflow-region", want: map[string]string{"overflow": "visible", "scrollbar-gutter": "auto"}},
		{selector: ".portal-table", want: map[string]string{"display": "block", "width": "100%", "min-width": "0"}},
		{selector: ".portal-table thead", want: map[string]string{"position": "absolute", "width": "1px", "height": "1px", "overflow": "hidden", "clip-path": "inset(50%)", "white-space": "nowrap"}},
		{selector: ".portal-table tbody", want: map[string]string{"display": "block"}},
		{selector: ".portal-instance-row", want: map[string]string{"display": "grid", "grid-template-columns": "minmax(0,1fr)", "min-width": "0"}},
		{selector: ".portal-instance-row > :is(th, td)", want: map[string]string{"display": "grid", "grid-template-columns": "minmax(4.5rem,0.32fr) minmax(0,1fr)", "min-width": "0", "overflow-wrap": "anywhere"}},
		{selector: ".portal-control", want: map[string]string{"min-block-size": "2.75rem", "min-inline-size": "2.75rem", "max-width": "100%"}},
		{selector: ".portal-action-controls", want: map[string]string{"grid-template-columns": "minmax(0,1fr)"}},
		{selector: ".portal-console-feedback", want: map[string]string{"min-width": "0", "overflow-wrap": "anywhere"}},
		{selector: ".portal-error-feedback", want: map[string]string{"min-width": "0", "overflow-wrap": "anywhere"}},
		{selector: ".portal-console-feedback :is(.ui-feedback-title, .ui-feedback-message)", want: map[string]string{"min-width": "0", "overflow-wrap": "anywhere"}},
		{selector: ".portal-error-feedback :is(.ui-feedback-title, .ui-feedback-message)", want: map[string]string{"min-width": "0", "overflow-wrap": "anywhere"}},
		{selector: ".portal-console-context .page-hero-title", want: map[string]string{"width": "100%", "max-width": "min(12ch,100%)", "font-size": "clamp(3rem,5.2vw,5rem)", "overflow-wrap": "anywhere"}},
		{selector: ".portal-instance-detail-row", want: map[string]string{"display": "none"}},
		{selector: ".portal-instance-detail-row[data-portal-detail-open='true']", want: map[string]string{"display": "block"}},
		{selector: ".portal-instance-table-shell .overflow-region-hint", width: 48, want: map[string]string{"display": "block"}},
		{selector: ".portal-operator-trail", width: 48, want: map[string]string{"width": "auto", "height": "18rem", "background": "transparent"}},
		{selector: ".portal-operator-trail .signal-trail-svg", width: 48, want: map[string]string{"display": "block"}},
		{selector: ".portal-instance-table-shell .overflow-region", width: 48, want: map[string]string{"overflow-x": "auto", "scrollbar-gutter": "stable", "scroll-padding-inline": "var(--space-lg)"}},
		{selector: ".portal-table", width: 48, want: map[string]string{"display": "table", "min-width": "66rem"}},
		{selector: ".portal-table thead", width: 48, want: map[string]string{"position": "static", "width": "auto", "height": "auto", "overflow": "visible", "clip-path": "none", "white-space": "normal"}},
		{selector: ".portal-table tbody", width: 48, want: map[string]string{"display": "table-row-group"}},
		{selector: ".portal-instance-row", width: 48, want: map[string]string{"display": "table-row"}},
		{selector: ".portal-instance-row > :is(th, td)", width: 48, want: map[string]string{"display": "table-cell"}},
		{selector: ".portal-instance-detail-row[data-portal-detail-open='true']", width: 48, want: map[string]string{"display": "table-row"}},
	}
	for _, requirement := range required {
		if !aboutHasRule(rules, requirement.selector, requirement.width, false, requirement.want) {
			return fmt.Errorf("Portal route CSS lacks %q at min-width %.0frem with %v", requirement.selector, requirement.width, requirement.want)
		}
	}

	for _, rule := range rules {
		if !strings.Contains(task2CanonicalCSS(rule.selector), ".portal-") || rule.forcedColors {
			continue
		}
		if rule.minWidthRem != 0 && rule.minWidthRem != 48 {
			return fmt.Errorf("Portal route CSS uses noncanonical %.0frem breakpoint for %q", rule.minWidthRem, rule.selector)
		}
	}
	portalScrollPaddingRules := 0
	for _, rule := range rules {
		for property, value := range rule.declarations {
			switch property {
			case "scroll-padding", "scroll-padding-inline", "scroll-padding-inline-start", "scroll-padding-inline-end", "scroll-padding-left", "scroll-padding-right":
				portalScrollPaddingRules++
				if rule.minWidthRem != 48 || rule.forcedColors || property != "scroll-padding-inline" || !task2CSSValueEqual(value, "var(--space-lg)") || !task2SelectorListContains(rule.selector, ".portal-instance-table-shell .overflow-region") {
					return fmt.Errorf("Portal desktop overflow region has competing %s %q", property, value)
				}
			}
		}
	}
	if portalScrollPaddingRules != 1 {
		return fmt.Errorf("Portal desktop overflow region has %d scroll-padding-inline owners; want exactly one", portalScrollPaddingRules)
	}
	if err := validatePortalForcedRowSurface(rules); err != nil {
		return err
	}
	return nil
}

func validatePortalForcedRowSurface(rules []aboutCSSRule) error {
	want := map[string]string{
		"border-color": "CanvasText !important",
		"background":   "Canvas !important",
		"color":        "CanvasText !important",
		"box-shadow":   "none !important",
	}
	counts := map[string]int{}
	for _, rule := range rules {
		if !rule.forcedColors || !styleSelectorHasExactClass(rule.selector, "portal-instance-row") {
			continue
		}
		if task2CanonicalCSS(rule.selector) != ".portal-instance-row" {
			return fmt.Errorf("Portal compact forced row has competing selector %q", rule.selector)
		}
		if rule.minWidthRem != 0 {
			return fmt.Errorf("Portal compact forced row owner is gated behind min-width %.0frem", rule.minWidthRem)
		}
		for property, value := range rule.declarations {
			expected, relevant := want[property]
			if !relevant {
				return fmt.Errorf("Portal compact forced row has unapproved declaration %s:%s in %q", property, value, rule.selector)
			}
			if !task2CSSValueEqual(value, expected) {
				return fmt.Errorf("Portal compact forced row has %s:%s, want %s", property, value, expected)
			}
			counts[property]++
		}
	}
	for property := range want {
		if counts[property] != 1 {
			return fmt.Errorf("Portal compact forced row has %d %s owners; want exactly one", counts[property], property)
		}
	}
	return nil
}
