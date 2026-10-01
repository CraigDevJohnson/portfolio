package app

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// A raw length is a number, signed or not, with a rem, em or px unit written
// into a rule instead of a design token: -0.125rem, 0 -2px, mt-[-0.5rem] and
// calc(100%-2px) all match. The number or its sign must not continue an
// identifier, so tokens such as --space-2xl and classes such as px-5 never do.
var designTokenRawLengthPattern = regexp.MustCompile(`(?i)(?:^|[^a-z0-9_.-])(-?[0-9]*\.?[0-9]+(?:rem|em|px))\b`)

// The Soccer output choice, player removal and history notice, and the
// account navigation, are sized only with design tokens
// (.github/instructions/tailwind.instructions.md, "Design tokens").
func TestSoccerAndAccountNavigationStylesUseDesignTokens(t *testing.T) {
	sources := map[string]string{
		"cmd/web/tailwind/soccer.css":     readTask2Artifact(t, "cmd", "web", "tailwind", "soccer.css"),
		"cmd/web/tailwind/components.css": readTask2Artifact(t, "cmd", "web", "tailwind", "components.css"),
	}
	for path, css := range sources {
		if err := validateDesignTokenSizes(css, designTokenOwnedClasses); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
}

// The Import Let's Play Soccer access dialog shows two uppercase mono
// eyebrows: "Kept indefinitely" on the history notice and "Sensitive access"
// on the security notice. They are styled as a pair, so both resolve to the
// same type.
func TestSoccerImportDialogEyebrowsMatch(t *testing.T) {
	rules, err := collectExperienceCSSRules(readTask2Artifact(t, "cmd", "web", "tailwind", "soccer.css"), 0, false, false)
	if err != nil {
		t.Fatalf("parse Soccer CSS: %v", err)
	}
	tokens := soccerThemeTokens(readTask2Artifact(t, "cmd", "web", "tailwind", "shared.css"))
	sensitive := soccerEffectiveDeclarations(rules, ".soccer-security-notice::before", 0, false)
	history := soccerEffectiveDeclarations(rules, ".soccer-history-notice .soccer-history-notice-title", 0, false)
	for _, property := range []string{"font-family", "font-size", "font-weight", "letter-spacing", "text-transform"} {
		want := designTokenResolve(tokens, sensitive[property])
		if want == "" {
			t.Errorf(".soccer-security-notice::before sets no %s", property)
			continue
		}
		if got := designTokenResolve(tokens, history[property]); !task2CSSValueEqual(got, want) {
			t.Errorf("history notice title %s is %q (%s), want the Sensitive access label's %q (%s)", property, history[property], got, sensitive[property], want)
		}
	}
}

// designTokenResolve replaces each var() reference to a known token with its
// value, following tokens that refer to other tokens.
func designTokenResolve(tokens map[string]string, value string) string {
	for range 8 {
		resolved := soccerVarRefPattern.ReplaceAllStringFunc(value, func(ref string) string {
			if token, ok := tokens[soccerVarRefPattern.FindStringSubmatch(ref)[1]]; ok {
				return token
			}
			return ref
		})
		if resolved == value {
			break
		}
		value = resolved
	}
	return value
}

func TestDesignTokenSizeValidatorRejectsRawLengths(t *testing.T) {
	classes := []string{"fixture-notice"}
	for _, valid := range []string{
		`.fixture-notice { padding: var(--space-sm) calc(var(--space-md) + var(--space-xs)) 0; max-width: calc(var(--space-3xl) * 2.5); gap: 0; width: 100%; }`,
		`.fixture-notice { padding-block: clamp(var(--space-xl), 5vw, var(--space-2xl)); letter-spacing: var(--tracking-widest); }`,
		`.fixture-notice { @apply overflow-hidden px-5 text-2xl; }`,
		`.fixture-notice { margin-top: calc(-1 * var(--space-xs)); @apply -mt-2 w-2px; }`,
		`/* keeps a 44px target */ .fixture-notice { padding-block: var(--space-md); }`,
		`.unrelated { padding: 0.75rem; }`,
	} {
		if err := validateDesignTokenSizes(valid, classes); err != nil {
			t.Errorf("validator rejected tokenized CSS %q: %v", valid, err)
		}
	}
	for _, invalid := range []string{
		`.fixture-notice { padding-block: 0.75rem; }`,
		`.fixture-notice p { font-size: 0.68rem; }`,
		`.fixture-notice { letter-spacing: 0.12em; }`,
		`.fixture-notice { border-width: 2px; }`,
		`.fixture-notice { padding: var(--space-sm) 1.25rem 0; }`,
		`.fixture-notice { margin: calc(var(--space-sm) + .5rem); }`,
		`.fixture-notice { @apply tracking-[0.12em]; }`,
		`@media (max-width: 69.999rem) { .fixture-notice { padding-inline: 1.25rem; } }`,
		`@media (forced-colors: active) { .other, .fixture-notice { outline: 1px solid CanvasText; } }`,
		`.fixture-notice { margin-top: -0.125rem; }`,
		`.fixture-notice { translate: 0 -2px; }`,
		`.fixture-notice { @apply mt-[-0.5rem]; }`,
		`.fixture-notice { @apply w-[calc(100%-2px)]; }`,
	} {
		if err := validateDesignTokenSizes(invalid, classes); err == nil {
			t.Errorf("validator accepted raw length in %q", invalid)
		}
	}
}

// designTokenOwnedClasses are the component classes the Soccer planner and
// site account navigation introduced; every rule that names one is checked.
var designTokenOwnedClasses = []string{
	"soccer-output-choice",
	"soccer-output-options",
	"soccer-output-option",
	"soccer-player-removal",
	"soccer-player-removal-body",
	"soccer-player-removal-list",
	"soccer-history-notice",
	"soccer-history-notice-title",
	"site-account-state",
	"site-account-email",
	"site-account-link",
}

func validateDesignTokenSizes(css string, classes []string) error {
	blocks, err := parseTask2CSSBlocks(css)
	if err != nil {
		return err
	}
	var failures []string
	for _, block := range blocks {
		if strings.HasPrefix(block.header, "@") {
			if nestedErr := validateDesignTokenSizes(block.body, classes); nestedErr != nil {
				failures = append(failures, nestedErr.Error())
			}
			continue
		}
		if !designTokenSelectorNamesClass(block.header, classes) {
			continue
		}
		for _, match := range designTokenRawLengthPattern.FindAllStringSubmatch(block.body, -1) {
			failures = append(failures, fmt.Sprintf("%q uses raw length %s instead of a design token", task2CanonicalCSS(block.header), match[1]))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("%s", strings.Join(failures, "; "))
	}
	return nil
}

func designTokenSelectorNamesClass(selector string, classes []string) bool {
	for _, className := range classes {
		if styleSelectorHasExactClass(selector, className) {
			return true
		}
	}
	return false
}
