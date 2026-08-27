package app

import (
	"fmt"
	"strings"
	"testing"
)

func TestSoccerCSSIsImportedOnceIntoPagesLayer(t *testing.T) {
	appCSS := readTask2Artifact(t, "cmd", "web", "tailwind", "app.css")
	const routeImport = `@import "./soccer.css" layer(pages);`
	if got := strings.Count(task2CanonicalCSS(appCSS), routeImport); got != 1 {
		t.Fatalf("Soccer route import count = %d, want exactly one pages-layer import", got)
	}
}

func TestSoccerRouteCSSUsesMatchdayCompositionContract(t *testing.T) {
	css := readTask2Artifact(t, "cmd", "web", "tailwind", "soccer.css")
	sharedCSS := readTask2Artifact(t, "cmd", "web", "tailwind", "shared.css")
	if err := validateSoccerJWTFocusClearanceCSS(css, sharedCSS); err != nil {
		t.Fatal(err)
	}
	rules, err := collectExperienceCSSRules(css, 0, false, false)
	if err != nil {
		t.Fatalf("parse Soccer CSS: %v", err)
	}

	required := []struct {
		selector string
		width    float64
		forced   bool
		reduced  bool
		want     map[string]string
	}{
		{selector: ".soccer-planner-layout", want: map[string]string{"display": "grid"}},
		{selector: ".soccer-workflow-header", want: map[string]string{"display": "grid"}},
		{selector: ".main-content:has(.soccer-page)", want: map[string]string{"isolation": "auto"}},
		{selector: ".soccer-source-grid", want: map[string]string{"display": "grid"}},
		{selector: ".soccer-manual-controls", want: map[string]string{"grid-template-columns": "minmax(0,1fr)"}},
		{selector: ".soccer-manual-controls .btn", want: map[string]string{"width": "100%"}},
		{selector: ".games-section-top", want: map[string]string{"position": "relative", "background": "var(--games-toolbar-surface)"}},
		{selector: ".soccer-match-list", want: map[string]string{"display": "grid", "overflow": "visible"}},
		{selector: ".soccer-match-row", want: map[string]string{"display": "grid", "min-width": "0"}},
		{selector: ".soccer-action-feedback", want: map[string]string{"min-height": "3rem"}},
		{selector: ".soccer-modal-close", want: map[string]string{"width": "2.75rem", "height": "2.75rem"}},
		{selector: ".soccer-login-jwt-field", want: map[string]string{"display": "flex", "flex-direction": "column", "gap": "var(--space-sm)"}},
		{selector: ".soccer-inline-link", want: map[string]string{"min-height": "2.75rem", "margin-block": "0", "line-height": "2.75rem"}},
		{selector: "#team_codes", want: map[string]string{"scroll-margin-block-start": "calc(var(--header-height) + var(--space-xl))"}},
		{selector: "#soccer-team-stage-content", want: map[string]string{"scroll-margin-block-start": "calc(var(--header-height) + var(--space-xl))"}},
		{selector: ".soccer-source-grid", width: 48, want: map[string]string{"grid-template-columns": "repeat(2,minmax(0,1fr))"}},
		{selector: ".soccer-match-row", width: 48, want: map[string]string{"grid-template-columns": "3rem minmax(0,0.9fr) minmax(0,1.35fr) minmax(0,1.2fr) minmax(0,0.7fr)"}},
		{selector: ".soccer-connections-grid", width: 70, want: map[string]string{"grid-template-columns": "repeat(2,minmax(0,1fr))"}},
		{selector: ".soccer-stage-legend", width: 70, want: map[string]string{"grid-template-columns": "repeat(4,minmax(0,1fr))"}},
		{selector: ".soccer-matchday-workspace", width: 70, want: map[string]string{"grid-template-columns": "repeat(2,minmax(0,1fr))"}},
		{selector: ".soccer-stage-source", width: 70, want: map[string]string{"grid-column": "1/-1"}},
		{selector: ".soccer-page *", reduced: true, want: map[string]string{"animation-duration": "0.01ms !important", "animation-delay": "0ms !important", "transition-duration": "0.01ms !important", "transition-delay": "0ms !important"}},
		{selector: ".soccer-matchday-trail .signal-trail-svg", forced: true, want: map[string]string{"display": "none"}},
		{selector: ".soccer-matchday-trail", forced: true, want: map[string]string{"height": "auto", "--signal-trail-forced-block-border": "0", "--signal-trail-forced-inline-border": "var(--line-signal) solid CanvasText", "border-block-start": "0 !important", "border-inline-start": "var(--line-signal) solid CanvasText !important"}},
		{selector: ".soccer-matchday-trail", width: 80, forced: true, want: map[string]string{"height": "var(--line-hairline)", "--signal-trail-forced-block-border": "var(--line-signal) solid CanvasText", "--signal-trail-forced-inline-border": "0", "border-inline-start": "0 !important", "border-block-start": "var(--line-signal) solid CanvasText !important"}},
	}
	for _, requirement := range required {
		if !experienceHasEffectiveRule(rules, requirement.selector, requirement.width, requirement.forced, requirement.reduced, requirement.want) {
			t.Errorf("Soccer CSS lacks %q at %.0frem forced=%t reduced=%t with %v", requirement.selector, requirement.width, requirement.forced, requirement.reduced, requirement.want)
		}
	}
	for _, rule := range rules {
		if task2CanonicalCSS(rule.selector) == ".soccer-connections-grid" && task2CSSValueEqual(rule.declarations["grid-template-columns"], "repeat(2,minmax(0,1fr))") && rule.minWidthRem != 70 {
			t.Errorf("Soccer connections split at %.0frem, want the roomy 70rem desktop breakpoint", rule.minWidthRem)
		}
	}

	allowedBreakpoints := map[float64]bool{0: true, 30: true, 48: true, 70: true, 80: true}
	for _, rule := range rules {
		if !allowedBreakpoints[rule.minWidthRem] {
			t.Errorf("Soccer CSS uses noncanonical %.3grem breakpoint for %q", rule.minWidthRem, rule.selector)
		}
	}
	canonical := task2CanonicalCSS(css)
	for _, forbidden := range []string{".table-wrapper{", ".games-table{", ".games-table th", "overflow-x:auto", ".soccer-planner-rail{position:sticky", "position:sticky;top:calc(var(--header-height)+var(--space-sm))"} {
		if strings.Contains(canonical, forbidden) {
			t.Errorf("Soccer CSS retains forbidden horizontal-scroll/sticky schedule pattern %q", forbidden)
		}
	}
	for _, forbidden := range []string{".soccer-hero::after", "480px", "768px", "1024px", "(width >= 40rem)", "(min-width: 40rem)", "(width >= 68rem)", "(min-width: 68rem)"} {
		if strings.Contains(canonical, forbidden) {
			t.Errorf("Soccer CSS contains forbidden legacy pattern %q", forbidden)
		}
	}
	for _, rule := range rules {
		if !rule.forcedColors {
			continue
		}
		for _, selector := range strings.Split(task2CanonicalCSS(rule.selector), ",") {
			if selector == ".loading-indicator" || selector == ".empty-state" || selector == ".no-results" {
				t.Errorf("Soccer route CSS leaks unscoped forced-color selector %q", selector)
			}
		}
	}
	modal := readTask2Artifact(t, "cmd", "web", "partials", "soccer_login_modal.templ")
	for _, conflictingUtility := range []string{" p-4", "sm:p-6", "max-h-[", "sm:max-h-["} {
		if strings.Contains(modal, conflictingUtility) {
			t.Errorf("Soccer modal template overrides safe-area CSS with utility %q", conflictingUtility)
		}
	}
	for _, path := range [][]string{
		{"cmd", "web", "pages", "soccer.templ"},
		{"cmd", "web", "partials", "soccer_login_state.templ"},
		{"cmd", "web", "partials", "soccer_player_select.templ"},
		{"cmd", "web", "partials", "soccer_team_select.templ"},
		{"cmd", "web", "partials", "soccer_table_fragment.templ"},
	} {
		source := readTask2Artifact(t, path...)
		if strings.Contains(source, "max-sm:") || strings.Contains(source, " sm:") {
			t.Errorf("%s uses the noncanonical 40rem Tailwind seam", strings.Join(path, "/"))
		}
	}
}

func TestSoccerJWTFocusClearanceCSSValidatorRejectsRegressions(t *testing.T) {
	const validRoute = `.soccer-login-jwt-field { display:flex; flex-direction:column; gap:var(--space-sm); }
    .soccer-login-note { margin:0; }`
	const validShared = `:root { --space-sm:0.5rem; }`
	type fixture struct {
		route  string
		shared string
	}
	mutations := map[string]fixture{
		"higher specificity clears gap":      {route: validRoute + `.soccer-login-form .soccer-login-jwt-field { gap:0; }`, shared: validShared},
		"row gap clears vertical space":      {route: strings.Replace(validRoute, `gap:var(--space-sm);`, `gap:var(--space-sm); row-gap:0;`, 1), shared: validShared},
		"hint uses negative margin":          {route: validRoute + `#soccer-import-jwt-hint { margin-block-start:-0.5rem; }`, shared: validShared},
		"hint is positioned over field":      {route: validRoute + `#soccer-import-jwt-hint { position:relative; inset-block-start:-0.5rem; }`, shared: validShared},
		"hint class uses negative margin":    {route: strings.Replace(validRoute, `margin:0;`, `margin-block-start:-0.5rem;`, 1), shared: validShared},
		"hint class is translated upward":    {route: strings.Replace(validRoute, `margin:0;`, `position:relative; transform:translateY(-0.5rem);`, 1), shared: validShared},
		"wrapper class attribute clears gap": {route: validRoute + `[class~="soccer-login-jwt-field"] { gap:0; }`, shared: validShared},
		"hint class attribute is displaced":  {route: validRoute + `[class~="soccer-login-note"] { position:relative; inset-block-start:-0.5rem; }`, shared: validShared},
		"hint ID attribute is displaced":     {route: validRoute + `[id="soccer-import-jwt-hint"] { margin-block-start:-0.5rem; }`, shared: validShared},
		"route overrides spacing token":      {route: validRoute + `.soccer-login-form { --space-sm:0; }`, shared: validShared},
		"shared spacing token collapses":     {route: validRoute, shared: strings.Replace(validShared, `0.5rem`, `0`, 1)},
	}
	if err := validateSoccerJWTFocusClearanceCSS(validRoute, validShared); err != nil {
		t.Fatalf("valid JWT focus-clearance CSS rejected: %v", err)
	}
	for name, mutation := range mutations {
		t.Run(name, func(t *testing.T) {
			if err := validateSoccerJWTFocusClearanceCSS(mutation.route, mutation.shared); err == nil {
				t.Fatal("JWT focus-clearance CSS regression was accepted")
			}
		})
	}
}

func validateSoccerJWTFocusClearanceCSS(css, sharedCSS string) error {
	rules, err := collectExperienceCSSRules(css, 0, false, false)
	if err != nil {
		return fmt.Errorf("parse Soccer JWT focus-clearance CSS: %w", err)
	}
	ownerCount := 0
	for _, rule := range rules {
		if value := rule.declarations["--space-sm"]; value != "" {
			return fmt.Errorf("Soccer route overrides shared JWT focus-clearance token --space-sm:%s in %q", value, rule.selector)
		}
		selector := task2CanonicalCSS(rule.selector)
		for _, item := range strings.Split(selector, ",") {
			item = strings.TrimSpace(item)
			if styleSelectorHasExactClass(item, "soccer-login-jwt-field") {
				ownerCount++
				if selector != ".soccer-login-jwt-field" || item != ".soccer-login-jwt-field" || rule.minWidthRem != 0 || rule.forcedColors || rule.reducedMotion {
					return fmt.Errorf("Soccer JWT focus-clearance stack has competing selector or conditional owner %q", rule.selector)
				}
				want := map[string]string{
					"display":        "flex",
					"flex-direction": "column",
					"gap":            "var(--space-sm)",
				}
				if len(rule.declarations) != len(want) {
					return fmt.Errorf("Soccer JWT focus-clearance owner has uncontracted declarations: %v", rule.declarations)
				}
				for property, value := range want {
					if !task2CSSValueEqual(rule.declarations[property], value) {
						return fmt.Errorf("Soccer JWT focus-clearance owner %s = %q, want %q", property, rule.declarations[property], value)
					}
				}
			}
			if skillsSelectorHasExactID(item, "soccer-import-jwt-hint") {
				return fmt.Errorf("Soccer JWT hint has a displacement-capable route override %q", rule.selector)
			}
			if styleSelectorHasExactClass(item, "soccer-login-note") {
				for _, property := range []string{
					"margin", "margin-top", "margin-block", "margin-block-start", "margin-block-end",
					"position", "inset", "inset-block", "inset-block-start", "inset-block-end", "top", "bottom",
					"translate", "transform",
				} {
					value, present := rule.declarations[property]
					if present && !soccerJWTFocusHintDisplacementIsNeutral(property, value) {
						return fmt.Errorf("Soccer JWT hint selector %q displaces the focus-clearance stack with %s:%s", rule.selector, property, value)
					}
				}
			}
		}
	}
	if ownerCount != 1 {
		return fmt.Errorf("Soccer JWT focus-clearance owner count = %d, want 1", ownerCount)
	}
	sharedBlocks, err := collectStyleBlocksWithDepth(sharedCSS, 0)
	if err != nil {
		return fmt.Errorf("parse shared JWT focus-clearance token CSS: %w", err)
	}
	spaceTokenOwners := 0
	for _, entry := range sharedBlocks {
		if strings.HasPrefix(strings.TrimSpace(entry.block.header), "@") {
			continue
		}
		value := task2Declarations(entry.block.body)["--space-sm"]
		if value == "" {
			continue
		}
		spaceTokenOwners++
		if task2CanonicalCSS(entry.block.header) != ":root" || entry.depth != 0 || !task2CSSValueEqual(value, "0.5rem") {
			return fmt.Errorf("shared JWT focus-clearance token has competing owner/value --space-sm:%s in %q at depth %d", value, entry.block.header, entry.depth)
		}
	}
	if spaceTokenOwners != 1 {
		return fmt.Errorf("shared JWT focus-clearance token owner count = %d, want 1", spaceTokenOwners)
	}
	return nil
}

func soccerJWTFocusHintDisplacementIsNeutral(property, value string) bool {
	value = strings.ToLower(task2CanonicalCSS(value))
	zero := value == "0" || value == "0px" || value == "0rem" || value == "0em"
	switch property {
	case "margin", "margin-top", "margin-block", "margin-block-start", "margin-block-end":
		return zero
	case "position":
		return value == "static"
	case "inset", "inset-block", "inset-block-start", "inset-block-end", "top", "bottom":
		return value == "auto" || zero
	case "translate", "transform":
		return value == "none"
	default:
		return false
	}
}

func TestSoccerSelectorsHaveOneRouteOwner(t *testing.T) {
	legacyPaths := [][]string{
		{"cmd", "web", "tailwind", "shared.css"},
		{"cmd", "web", "tailwind", "base.css"},
		{"cmd", "web", "tailwind", "components.css"},
	}
	for _, path := range legacyPaths {
		css := readTask2Artifact(t, path...)
		rules, err := collectTask2StyleRules(css)
		if err != nil {
			t.Fatalf("parse %s: %v", strings.Join(path, "/"), err)
		}
		for _, rule := range rules {
			selector := task2CanonicalCSS(rule.header)
			if soccerOwnedSelector(selector) {
				t.Errorf("%s still owns Soccer selector %q", strings.Join(path, "/"), rule.header)
			}
		}
		if strings.Contains(task2CanonicalCSS(css), ".page-kit-hero-soccer") {
			t.Errorf("%s still owns the Soccer-specific page-kit hero alias", strings.Join(path, "/"))
		}
	}
}

func soccerOwnedSelector(selector string) bool {
	for _, marker := range []string{
		".soccer-", ".games-", ".game-", ".games", ".table-wrapper", ".loading-indicator", ".no-results",
	} {
		if strings.Contains(selector, marker) {
			return true
		}
	}
	return false
}
