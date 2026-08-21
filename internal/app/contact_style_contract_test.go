package app

import (
	"fmt"
	"strings"
	"testing"
)

func TestContactRouteCSSContract(t *testing.T) {
	css := readTask2Artifact(t, "cmd", "web", "tailwind", "pages", "contact.css")
	if err := validateContactRouteCSS(css); err != nil {
		t.Fatal(err)
	}
}

func TestContactRouteCSSValidatorRejectsRegressions(t *testing.T) {
	css := readTask2Artifact(t, "cmd", "web", "tailwind", "pages", "contact.css")
	assertContactCSSMutationRejected(t, css, "compact ticket becomes sticky", "position: static;", "position: sticky;")
	assertContactCSSMutationRejected(t, css, "tablet email stops spanning the channel row", `.contact-channel-email {
    grid-column: 1 / -1;
  }`, `.contact-channel-email {
    grid-column: auto;
  }`)
	assertContactCSSMutationRejected(t, css, "dense ticket loses sticky positioning", "position: sticky;", "position: static;")
	assertContactCSSMutationRejected(t, css, "dense channel column moves left", "grid-column: 5 / -1;", "grid-column: 1 / -1;")
	assertContactCSSMutationRejected(t, css, "long channel labels stop wrapping", `.contact-channel .wrap-anywhere {
  overflow-wrap: anywhere;`, `.contact-channel .wrap-anywhere {
  overflow-wrap: normal;`)
	assertContactCSSMutationRejected(t, css, "expertise becomes a false raised control", "transform: none;", "transform: translateY(-0.2rem);")
	assertContactCSSMutationRejected(t, css, "wide SVG never replaces compact rail", "display: block;", "display: none;")
}

func assertContactCSSMutationRejected(t *testing.T, css, name, old, replacement string) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		mutated := strings.Replace(css, old, replacement, 1)
		if mutated == css {
			t.Fatalf("mutation target %q not found", old)
		}
		if err := validateContactRouteCSS(mutated); err == nil {
			t.Fatal("validateContactRouteCSS() error = nil, want regression rejected")
		}
	})
}

func TestContactCSSIsImportedIntoPagesLayer(t *testing.T) {
	appCSS := readTask2Artifact(t, "cmd", "web", "tailwind", "app.css")
	const routeImport = `@import "./pages/contact.css" layer(pages);`
	if count := strings.Count(task2CanonicalCSS(appCSS), routeImport); count != 1 {
		t.Fatalf("Contact route import count = %d, want exactly one pages-layer import", count)
	}
}

func TestContactSelectorsHaveOneRouteOwner(t *testing.T) {
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
			if strings.Contains(selector, ".contact-") || strings.Contains(selector, ".page-kit-hero-contact") || strings.Contains(selector, ".page-hero-invitation") || strings.Contains(selector, ".page-kit-hero-variant-invitation") {
				t.Errorf("legacy stylesheet %d still owns Contact selector %q", index, rule.header)
			}
		}
	}
}

func validateContactRouteCSS(css string) error {
	rules, err := collectAboutCSSRules(css, 0, false)
	if err != nil {
		return fmt.Errorf("parse Contact route CSS: %w", err)
	}

	required := []struct {
		selector string
		width    float64
		forced   bool
		want     map[string]string
	}{
		{selector: ".contact-correspondence-route", want: map[string]string{"position": "relative", "isolation": "isolate", "display": "grid", "grid-template-columns": "minmax(0,1fr)", "min-width": "0"}},
		{selector: ".contact-availability-section", want: map[string]string{"z-index": "1", "position": "static", "inset": "auto", "grid-column": "auto", "grid-row": "auto", "align-self": "stretch", "transform": "none"}},
		{selector: ".contact-availability-grid", want: map[string]string{"display": "grid", "grid-template-columns": "minmax(0,1fr)"}},
		{selector: ".contact-channels-grid", want: map[string]string{"display": "grid", "grid-template-columns": "minmax(0,1fr)"}},
		{selector: ".contact-channel", want: map[string]string{"min-block-size": "2.75rem", "min-width": "0"}},
		{selector: ".contact-channel .wrap-anywhere", want: map[string]string{"overflow-wrap": "anywhere"}},
		{selector: ".contact-channel .page-kit-icon-frame", want: map[string]string{"flex-shrink": "0"}},
		{selector: ".contact-correspondence-trail", want: map[string]string{"position": "absolute", "z-index": "0", "display": "block", "width": "var(--line-signal)", "height": "auto", "margin": "0", "pointer-events": "none", "transform": "none"}},
		{selector: ".contact-correspondence-trail .signal-trail-svg", want: map[string]string{"display": "none"}},
		{selector: ".contact-channels-section", want: map[string]string{"position": "relative", "z-index": "1"}},
		{selector: ".contact-expertise-ribbon", want: map[string]string{"display": "grid", "grid-template-columns": "minmax(0,1fr)"}},
		{selector: ".contact-fit-card:hover", want: map[string]string{"transform": "none"}},
		{selector: ".contact-availability-grid", width: 30, want: map[string]string{"grid-template-columns": "repeat(2,minmax(0,1fr))"}},
		{selector: ".contact-channels-grid", width: 48, want: map[string]string{"grid-template-columns": "repeat(2,minmax(0,1fr))"}},
		{selector: ".contact-channel-email", width: 48, want: map[string]string{"grid-column": "1 / -1"}},
		{selector: ".contact-channel-linkedin", width: 48, want: map[string]string{"grid-column": "auto", "grid-row": "auto"}},
		{selector: ".contact-channel-github", width: 48, want: map[string]string{"grid-column": "auto", "grid-row": "auto"}},
		{selector: ".contact-availability-grid", width: 48, want: map[string]string{"grid-template-columns": "repeat(3,minmax(0,1fr))"}},
		{selector: ".contact-expertise-ribbon", width: 48, want: map[string]string{"grid-template-columns": "repeat(2,minmax(0,1fr))"}},
		{selector: ".contact-correspondence-route", width: 70, want: map[string]string{"grid-template-columns": "repeat(12,minmax(0,1fr))"}},
		{selector: ".contact-availability-header", width: 70, want: map[string]string{"grid-template-columns": "minmax(0,1fr)"}},
		{selector: ".contact-availability-grid", width: 70, want: map[string]string{"grid-template-columns": "minmax(0,1fr)"}},
		{selector: ".contact-availability-section", width: 70, want: map[string]string{"position": "sticky", "inset": "auto", "top": "calc(var(--header-height) + var(--space-lg))", "grid-column": "1 / span 4", "grid-row": "1", "align-self": "start"}},
		{selector: ".contact-channels-section", width: 70, want: map[string]string{"grid-column": "5 / -1", "grid-row": "1"}},
		{selector: ".contact-correspondence-trail", width: 70, want: map[string]string{"width": "auto", "height": "auto", "background": "transparent", "transform": "scaleX(-1)"}},
		{selector: ".contact-correspondence-trail .signal-trail-svg", width: 70, want: map[string]string{"display": "block"}},
		{selector: ".contact-expertise-ribbon", width: 70, want: map[string]string{"grid-template-columns": "repeat(4,minmax(0,1fr))"}},
		{selector: ".contact-correspondence-route", width: 80, want: map[string]string{"grid-template-columns": "repeat(12,minmax(0,1fr))"}},
		{selector: ".contact-availability-header", width: 80, want: map[string]string{"grid-template-columns": "minmax(0,1fr)"}},
		{selector: ".contact-availability-grid", width: 80, want: map[string]string{"grid-template-columns": "minmax(0,1fr)"}},
		{selector: ".contact-availability-section", width: 80, want: map[string]string{"grid-column": "1 / span 4", "grid-row": "1"}},
		{selector: ".contact-channels-section", width: 80, want: map[string]string{"grid-column": "5 / -1", "grid-row": "1"}},
		{selector: ".contact-channel-email", width: 80, want: map[string]string{"grid-column": "1 / -1", "grid-row": "auto"}},
		{selector: ".contact-channel-linkedin", width: 80, want: map[string]string{"grid-column": "auto", "grid-row": "auto"}},
		{selector: ".contact-channel-github", width: 80, want: map[string]string{"grid-column": "auto", "grid-row": "auto"}},
		{selector: ".contact-correspondence-trail", forced: true, want: map[string]string{"border-block-start": "0 !important", "border-inline-start": "var(--line-signal) solid CanvasText !important", "color": "CanvasText !important"}},
		{selector: ".contact-correspondence-trail", width: 70, forced: true, want: map[string]string{"border-block-start": "var(--line-signal) solid CanvasText !important", "border-inline-start": "0 !important"}},
		{selector: ".contact-correspondence-trail .signal-trail-svg", forced: true, want: map[string]string{"display": "none !important"}},
	}
	for _, requirement := range required {
		if !contactHasEffectiveRule(rules, requirement.selector, requirement.width, requirement.forced, requirement.want) {
			return fmt.Errorf("Contact route CSS lacks %q at min-width %.0frem forced=%t with %v", requirement.selector, requirement.width, requirement.forced, requirement.want)
		}
	}

	allowedBreakpoints := map[float64]bool{0: true, 30: true, 48: true, 70: true, 80: true}
	for _, rule := range rules {
		if !strings.Contains(task2CanonicalCSS(rule.selector), ".contact-") || rule.forcedColors {
			continue
		}
		if !allowedBreakpoints[rule.minWidthRem] {
			return fmt.Errorf("Contact route CSS uses noncanonical %.0frem breakpoint for %q", rule.minWidthRem, rule.selector)
		}
	}
	return nil
}

func contactHasEffectiveRule(rules []aboutCSSRule, selector string, width float64, forced bool, want map[string]string) bool {
	effective := make(map[string]string)
	found := false
	for _, rule := range rules {
		if rule.forcedColors != forced || rule.minWidthRem > width || !task2SelectorListContains(rule.selector, selector) {
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
