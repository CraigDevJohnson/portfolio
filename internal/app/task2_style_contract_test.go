package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"portfolio/cmd/web/partials"
)

type task2CSSBlock struct {
	header string
	body   string
}

func TestTask2ForcedColorValidatorRejectsBadFixtures(t *testing.T) {
	validRules := `
    :where(a[href]) { color: LinkText !important; background: transparent !important; }
    :where(a[href]:visited) { color: VisitedText !important; }
    :where(input:not([type='button']):not([type='submit']):not([type='reset']), select, textarea) {
      color: FieldText !important; background: Field !important;
    }
    :where(button, input:is([type='button'], [type='submit'], [type='reset']), [role='button'], .btn, .page-kit-action, .page-kit-icon-button, .site-chrome-menu-button, a.tech-pill) {
      border: var(--line-accent) solid ButtonBorder !important; color: ButtonText !important; background: ButtonFace !important;
    }
  `

	tests := []struct {
		name string
		css  string
		want string
	}{
		{
			name: "rules outside forced colors",
			css:  validRules,
			want: "forced-colors",
		},
		{
			name: "button action rule before visited and fields",
			css: `@media (forced-colors: active) {
        :where(a[href]) { color: LinkText !important; background: transparent !important; }
        :where(button, input:is([type='button'], [type='submit'], [type='reset']), [role='button'], .btn, .page-kit-action, .page-kit-icon-button, .site-chrome-menu-button, a.tech-pill) {
          border: var(--line-accent) solid ButtonBorder !important; color: ButtonText !important; background: ButtonFace !important;
        }
        :where(a[href]:visited) { color: VisitedText !important; }
        :where(input:not([type='button']):not([type='submit']):not([type='reset']), select, textarea) {
          color: FieldText !important; background: Field !important;
        }
      }`,
			want: "source order",
		},
		{
			name: "button action rule lacks button face",
			css: `@media (forced-colors: active) {` + strings.Replace(validRules,
				"background: ButtonFace !important;",
				"background: transparent !important;",
				1,
			) + `}`,
			want: "ButtonFace",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateTask2ForcedColorContract(test.css)
			if err == nil {
				t.Fatalf("validator accepted invalid CSS; want error containing %q", test.want)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validator error %q does not explain %q regression", err, test.want)
			}
		})
	}
}

func TestTask2ForcedColorValidatorRejectsDuplicateAndCrossCategoryFixtures(t *testing.T) {
	validPrefix := `@media (forced-colors: active) {
    :where(a[href]) { color: LinkText !important; background: transparent !important; }
    :where(a[href]:visited) { color: VisitedText !important; }
    :where(input:not([type='button']):not([type='submit']):not([type='reset']), select, textarea) {
      color: FieldText !important; background: Field !important;
    }
  `
	validAction := `
    :where(button, input:is([type='button'], [type='submit'], [type='reset']), [role='button'], .btn, .page-kit-action, .page-kit-icon-button, .site-chrome-menu-button, a.tech-pill) {
      border: var(--line-accent) solid ButtonBorder !important; color: ButtonText !important; background: ButtonFace !important;
    }
  `

	tests := []struct {
		name string
		css  string
		want string
	}{
		{
			name: "duplicate ordinary rule after actions",
			css: validPrefix + validAction + `
        :where(a[href]) { color: LinkText !important; background: transparent !important; }
      }`,
			want: "ordinary links",
		},
		{
			name: "duplicate visited rule after actions",
			css: validPrefix + validAction + `
        :where(a[href]:visited) { color: VisitedText !important; }
      }`,
			want: "visited links",
		},
		{
			name: "duplicate field rule after actions",
			css: validPrefix + validAction + `
        :where(input:not([type='button']):not([type='submit']):not([type='reset']), select, textarea) {
          color: FieldText !important; background: Field !important;
        }
      }`,
			want: "form fields",
		},
		{
			name: "action rule also targets ordinary links",
			css: validPrefix + strings.Replace(validAction,
				"a.tech-pill)",
				"a.tech-pill, a[href])",
				1,
			) + `}`,
			want: "extra selectors",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateTask2ForcedColorContract(test.css)
			if err == nil {
				t.Fatalf("validator accepted invalid CSS; want error containing %q", test.want)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validator error %q does not explain %q regression", err, test.want)
			}
		})
	}
}

func TestTask2ForcedColorValidatorAcceptsFormattingVariation(t *testing.T) {
	css := `@media( forced-colors : active ){
    :where( a[href] ){background:transparent!important;color:LinkText !important}
    :where(a[href]:visited){color : VisitedText!important}
    :where( input:not([type='button']):not([type='submit']):not([type='reset']) ,select,textarea ){
      background :Field !important;color:FieldText!important
    }
    :where(button,input:is([type='button'],[type='submit'],[type='reset']),[role='button'],.btn,.page-kit-action,.page-kit-icon-button,.site-chrome-menu-button,a.tech-pill){
      border:var(--line-accent) solid ButtonBorder!important;background:ButtonFace!important;color : ButtonText !important
    }
  }`

	if err := validateTask2ForcedColorContract(css); err != nil {
		t.Fatalf("validator rejected semantically valid compact CSS: %v", err)
	}
}

func TestTask2ForcedColorValidatorAcceptsCommentsOutsideStrings(t *testing.T) {
	validRules := `
    :where(a[href]) { --fixture: "/* keep quoted */"; color: LinkText !important; background: transparent !important; }
    :where(a[href]:visited) { color: VisitedText !important; }
    :where(input:not([type='button']):not([type='submit']):not([type='reset']), select, textarea) {
      color: FieldText !important; background: Field !important;
    }
    :where(button, input:is([type='button'], [type='submit'], [type='reset']), [role='button'], .btn, .page-kit-action, .page-kit-icon-button, .site-chrome-menu-button, a.tech-pill) {
      border: var(--line-accent) solid ButtonBorder !important; color: ButtonText !important; background: ButtonFace !important;
    }
  `

	tests := []struct {
		name string
		css  string
	}{
		{
			name: "leading comment",
			css:  `/* foundation */ @media (forced-colors: active) {` + validRules + `}`,
		},
		{
			name: "interstitial selector comments",
			css: `@media (forced-colors: active) {
          :where(a[href]) { color: LinkText !important; background: transparent !important; }
          :where(a[href]:visited) { color: VisitedText !important; }
          :where(input:not([type='button']):not([type='submit']):not([type='reset']), /* native lists */ select, textarea) {
            color: FieldText !important; background: Field !important;
          }
          :where(button, /* typed inputs */ input:is([type='button'], [type='submit'], [type='reset']), [role='button'], .btn, .page-kit-action, .page-kit-icon-button, .site-chrome-menu-button, a.tech-pill) {
            border: var(--line-accent) solid ButtonBorder !important; color: ButtonText !important; background: ButtonFace !important;
          }
        }`,
		},
		{
			name: "declaration and value comments",
			css: strings.NewReplacer(
				"color: LinkText !important", "color /* system foreground */ : LinkText /* link token */ !important",
				"background: ButtonFace !important", "background /* system surface */ : ButtonFace /* button token */ !important",
			).Replace(`@media (forced-colors: active) {` + validRules + `}`),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateTask2ForcedColorContract(test.css); err != nil {
				t.Fatalf("validator rejected semantically valid commented CSS: %v", err)
			}
		})
	}
}

func TestTask2CSSCommentStripperPreservesQuotedCommentText(t *testing.T) {
	css := `/* remove */ a::before { content: "/* keep */"; --single: '/* keep too */'; color: LinkText; }`
	stripped, err := task2StripCSSComments(css)
	if err != nil {
		t.Fatalf("strip CSS comments: %v", err)
	}
	for _, quoted := range []string{`"/* keep */"`, `'/* keep too */'`} {
		if !strings.Contains(stripped, quoted) {
			t.Fatalf("comment stripper removed quoted text %q from %q", quoted, stripped)
		}
	}
	if strings.Contains(stripped, "/* remove */") {
		t.Fatalf("comment stripper retained semantic comment in %q", stripped)
	}
}

func TestTask2ForcedColorsKeepLinksFieldsAndButtonsSemantic(t *testing.T) {
	css := readTask2Artifact(t, "cmd", "web", "tailwind", "base.css")
	if err := validateTask2ForcedColorContract(css); err != nil {
		t.Fatal(err)
	}
}

func TestTask12ForcedColorActionsKeepVisibleSystemBorder(t *testing.T) {
	css := readTask2Artifact(t, "cmd", "web", "tailwind", "base.css")
	rules, err := collectTask2ForcedColorRules(css)
	if err != nil {
		t.Fatalf("parse base CSS: %v", err)
	}
	_, actionRule, err := findTask2SemanticWhereRule(rules, "button/actions", []string{
		"button",
		"input:is([type='button'],[type='submit'],[type='reset'])",
		"[role='button']",
		".btn",
		".page-kit-action",
		".page-kit-icon-button",
		".site-chrome-menu-button",
		"a.tech-pill",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := task2Declarations(actionRule.body)["border"]; !task2CSSValueEqual(got, "var(--line-accent) solid ButtonBorder !important") {
		t.Fatalf("forced-color action border = %q; want visible system border", got)
	}
}

func TestTask12SharedSectionIntroKeepsIntrinsicWidth(t *testing.T) {
	css := readTask2Artifact(t, "cmd", "web", "tailwind", "components.css")
	rules, err := collectTask2StyleRules(css)
	if err != nil {
		t.Fatalf("parse component CSS: %v", err)
	}
	rule, ok := findTask2RuleWithDeclarations(rules, ".page-kit-section-intro", map[string]string{"min-width": "0"})
	if !ok || task2CanonicalCSS(rule.header) != ".page-kit-section-intro" {
		t.Fatal("canonical .page-kit-section-intro must retain min-width: 0")
	}
}

func TestTask13GradientTextHasDeterministicGlyphPaint(t *testing.T) {
	css := readTask2Artifact(t, "cmd", "web", "tailwind", "components.css")
	if err := validateTask13GradientTextPaint(css); err != nil {
		t.Fatal(err)
	}
}

func TestTask13GradientTextPaintMutationShield(t *testing.T) {
	valid := `.page-hero-title { text-shadow: 0 0.04em black; }
.text-gradient-brand { background: linear-gradient(90deg, white, orange); background-clip: text; -webkit-background-clip: text; -webkit-text-fill-color: transparent; text-shadow: none; }`
	for name, mutated := range map[string]string{
		"missing deterministic owner": strings.Replace(valid, " text-shadow: none;", "", 1),
		"brand shadow restored":       strings.Replace(valid, "text-shadow: none", "text-shadow: 0 0.04em red", 1),
		"higher specificity competitor": valid + `
.page-kit-page .text-gradient-brand { text-shadow: 0 0.04em red; }`,
		"attribute class competitor": valid + `
[class~="text-gradient-brand"] { text-shadow: 0 0.04em red; }`,
		"reset shorthand competitor": valid + `
.text-gradient-brand { all: revert; }`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateTask13GradientTextPaint(mutated); err == nil {
				t.Fatal("gradient text validator accepted nondeterministic alternate glyph paint")
			}
		})
	}
}

func validateTask13GradientTextPaint(css string) error {
	rules, err := collectTask2StyleRules(css)
	if err != nil {
		return fmt.Errorf("parse gradient text CSS: %w", err)
	}
	canonical := 0
	for _, rule := range rules {
		if !styleSelectorHasExactClass(rule.header, "text-gradient-brand") {
			continue
		}
		declarations := task2Declarations(rule.body)
		shadow, hasShadow := declarations["text-shadow"]
		_, hasReset := declarations["all"]
		if !hasShadow && !hasReset {
			continue
		}
		if task2CanonicalCSS(rule.header) != ".text-gradient-brand" || hasReset || !task2CSSValueEqual(shadow, "none") {
			return fmt.Errorf("gradient text has competing alternate glyph paint in %q", rule.header)
		}
		canonical++
	}
	if canonical != 1 {
		return fmt.Errorf("gradient text has %d deterministic text-shadow owners; want exactly one", canonical)
	}
	return nil
}

func TestTask12ForcedColorActiveNavHasNoDotMarker(t *testing.T) {
	sources := styleTailwindCSSSources(t, filepath.Join("..", "..", "cmd", "web", "tailwind"))
	if err := validateTask12ForcedNavMarker(sources); err != nil {
		t.Fatal(err)
	}
}

func TestTask12ForcedColorActiveNavRejectsMarkerOwners(t *testing.T) {
	valid := `@media (forced-colors: active) {}`
	mutations := map[string]string{
		"higher specificity shorthand": `@media (forced-colors: active) {
      body .site-chrome-nav-link-active::before { background: Canvas !important; }
    }`,
		"exact longhand": `@media (forced-colors: active) {
      .site-chrome-nav-link-active::before { background-color: Canvas !important; }
    }`,
		"generic nav pseudo": `@media (forced-colors: active) {
      .site-chrome-nav-link::before { background: Canvas !important; }
    }`,
		"compound forced context": `@media (forced-colors: active) and (width >= 30rem) {
      .site-chrome-nav-link::before { background-color: Canvas !important; }
    }`,
		"hidden active marker": `@media (forced-colors: active) {
      .site-chrome-nav-link-active::before { opacity: 0 !important; }
    }`,
		"aria current alias": `@media (forced-colors: active) {
      [aria-current='page']::before { background-color: Canvas !important; }
    }`,
		"single colon alias": `@media (forced-colors: active) {
      .site-chrome-nav-link-active:before { display: none !important; }
    }`,
		"positive context with unrelated not": `@media (forced-colors: active) and (not (width < 30rem)) {
      .site-chrome-nav-link::before { transform: scale(0) !important; }
    }`,
	}
	for name, mutation := range mutations {
		t.Run(name, func(t *testing.T) {
			sources := map[string]string{"base.css": valid, "components.css": mutation}
			if err := validateTask12ForcedNavMarker(sources); err == nil {
				t.Fatal("active navigation validator accepted a forced-color dot marker")
			}
		})
	}
}

func validateTask12ForcedNavMarker(sources map[string]string) error {
	for path, css := range sources {
		rules, err := collectTask12PositiveForcedColorRules(css)
		if err != nil {
			return fmt.Errorf("parse %s for forced-color navigation marker: %w", path, err)
		}
		for _, rule := range rules {
			for _, selector := range task2SplitTopLevel(rule.header, ',') {
				selector = task2CanonicalCSS(selector)
				markerPseudo := strings.Contains(selector, ":before")
				navMarkerOwner := strings.Contains(selector, ".site-chrome-nav-link") ||
					(strings.Contains(selector, "aria-current") && strings.Contains(selector, "page"))
				if !markerPseudo || !navMarkerOwner {
					continue
				}
				return fmt.Errorf("%s restores a forced-color active navigation marker through %q", path, rule.header)
			}
		}
	}
	return nil
}

func collectTask12PositiveForcedColorRules(css string) ([]task2CSSBlock, error) {
	blocks, err := parseTask2CSSBlocks(css)
	if err != nil {
		return nil, err
	}

	var rules []task2CSSBlock
	for _, block := range blocks {
		header := strings.ToLower(strings.NewReplacer(" ", "", "\t", "", "\r", "", "\n", "").Replace(task2CanonicalCSS(block.header)))
		negatesForcedColors := strings.Contains(header, "not(forced-colors:active)") || strings.HasPrefix(header, "@medianot")
		if strings.HasPrefix(header, "@media") && strings.Contains(header, "(forced-colors:active)") && !negatesForcedColors {
			mediaRules, err := collectTask2StyleRules(block.body)
			if err != nil {
				return nil, err
			}
			rules = append(rules, mediaRules...)
			continue
		}
		if strings.HasPrefix(header, "@") {
			nestedRules, err := collectTask12PositiveForcedColorRules(block.body)
			if err != nil {
				return nil, err
			}
			rules = append(rules, nestedRules...)
		}
	}
	return rules, nil
}

func TestTask2SharedShellTargetsMeetMinimumSize(t *testing.T) {
	css := readTask2Artifact(t, "cmd", "web", "tailwind", "components.css")
	if err := validateTask2SharedTargetContract(css); err != nil {
		t.Fatal(err)
	}
}

func TestTask2SharedTargetValidatorRejectsDecorativeTechPillMinimum(t *testing.T) {
	for _, selector := range []string{".tech-pill:not(a)", ".skill-cloud .tech-pill"} {
		t.Run(selector, func(t *testing.T) {
			css := fmt.Sprintf(`
        a.tech-pill { min-inline-size: 2.75rem; min-block-size: 2.75rem; }
        .site-chrome-nav-link { min-inline-size: 2.75rem; min-block-size: 2.75rem; }
        .site-chrome-footer-link { min-inline-size: 2.75rem; min-block-size: 2.75rem; }
        .site-logo { min-inline-size: 2.75rem; min-block-size: 2.75rem; }
        %s { min-inline-size: 2.75rem; min-block-size: 2.75rem; }
      `, selector)

			if err := validateTask2SharedTargetContract(css); err == nil {
				t.Fatalf("validator accepted minimum target sizes on non-link technology pill selector %q", selector)
			}
		})
	}
}

func TestTask2OperatorFooterHasOneCanonicalWidthContract(t *testing.T) {
	css := readTask2TailwindSources(t)
	rules, err := collectTask2StyleRules(css)
	if err != nil {
		t.Fatalf("parse Tailwind source CSS: %v", err)
	}

	var maxWidthRules []task2CSSBlock
	for _, rule := range rules {
		if !task2SelectorTargetsClass(rule.header, ".site-operator-footer-shell") {
			continue
		}
		if _, found := task2Declarations(rule.body)["max-width"]; found {
			maxWidthRules = append(maxWidthRules, rule)
		}
	}

	if len(maxWidthRules) != 1 {
		t.Fatalf("Tailwind sources have %d max-width rules affecting .site-operator-footer-shell; want exactly the canonical rule", len(maxWidthRules))
	}
	canonicalRule := maxWidthRules[0]
	if !task2SelectorListContains(canonicalRule.header, "body[data-shell='operator'] .site-operator-footer-shell") {
		t.Fatalf("operator footer max-width comes from %q, not the operator shell selector", canonicalRule.header)
	}
	if got := task2Declarations(canonicalRule.body)["max-width"]; !task2CSSValueEqual(got, "var(--composition-wide)") {
		t.Fatalf("operator footer max-width = %q; want var(--composition-wide)", got)
	}

	var rendered bytes.Buffer
	if err := partials.OperatorFooter().Render(context.Background(), &rendered); err != nil {
		t.Fatalf("render operator footer: %v", err)
	}
	classes, found := task2ElementClasses(rendered.String(), "site-operator-footer-shell")
	if !found {
		t.Fatal("rendered operator footer lacks .site-operator-footer-shell")
	}
	if err := validateTask2OperatorFooterClasses(classes); err != nil {
		t.Fatal(err)
	}
}

func TestTask2OperatorFooterClassValidatorRejectsAnyMaxWidthUtility(t *testing.T) {
	for _, className := range []string{
		"max-w-[var(--container-max)]",
		"max-w-6xl",
		"max-w-full",
		"max-w-[72rem]",
		"md:max-w-full",
	} {
		t.Run(className, func(t *testing.T) {
			err := validateTask2OperatorFooterClasses([]string{"site-operator-footer-shell", "mx-auto", className})
			if err == nil {
				t.Fatalf("validator accepted competing operator footer width utility %q", className)
			}
			if !strings.Contains(err.Error(), className) {
				t.Fatalf("validator error %q does not name competing class %q", err, className)
			}
		})
	}
}

func TestTask2OperatorFooterClassValidatorAllowsUnrelatedClasses(t *testing.T) {
	classes := []string{"site-operator-footer-shell", "not-max-w-full", "max-widgets", "footer-max-w-copy"}
	if err := validateTask2OperatorFooterClasses(classes); err != nil {
		t.Fatalf("validator rejected unrelated class: %v", err)
	}
}

func validateTask2SharedTargetContract(css string) error {
	rules, err := collectTask2StyleRules(css)
	if err != nil {
		return fmt.Errorf("parse component CSS: %w", err)
	}

	for _, selector := range []string{
		"a.tech-pill",
		".site-chrome-nav-link",
		".site-chrome-footer-link",
		".site-logo",
	} {
		rule, ok := findTask2RuleWithDeclarations(rules, selector,
			map[string]string{
				"min-inline-size": "2.75rem",
				"min-block-size":  "2.75rem",
			},
		)
		if !ok {
			return fmt.Errorf("component CSS has no %q rule with 2.75rem minimum inline and block sizes", selector)
		}
		if task2CanonicalCSS(rule.header) != task2CanonicalCSS(selector) {
			return fmt.Errorf("minimum target size for %q is attached to unexpected selector %q", selector, rule.header)
		}
	}

	for _, rule := range rules {
		declarations := task2Declarations(rule.body)
		_, hasMinimumInlineSize := declarations["min-inline-size"]
		_, hasMinimumBlockSize := declarations["min-block-size"]
		if !hasMinimumInlineSize && !hasMinimumBlockSize {
			continue
		}

		for _, selector := range task2SplitTopLevel(rule.header, ',') {
			selector = task2CanonicalCSS(selector)
			if selector == "a" || strings.Contains(selector, ".tech-pill") && selector != "a.tech-pill" {
				return fmt.Errorf("%q rule unexpectedly enlarges ordinary prose/decorative pills", rule.header)
			}
		}
	}

	return nil
}

func validateTask2OperatorFooterClasses(classes []string) error {
	for _, className := range classes {
		utilityName := className
		if separator := strings.LastIndexByte(utilityName, ':'); separator >= 0 {
			utilityName = utilityName[separator+1:]
		}
		if strings.HasPrefix(utilityName, "max-w-") {
			return fmt.Errorf("operator footer carries competing width utility %q", className)
		}
	}
	return nil
}

func validateTask2ForcedColorContract(css string) error {
	rules, err := collectTask2ForcedColorRules(css)
	if err != nil {
		return fmt.Errorf("parse CSS for forced-colors contract: %w", err)
	}
	if len(rules) == 0 {
		return fmt.Errorf("forced-colors media block not found")
	}

	ordinaryIndex, ordinaryRule, err := findTask2SemanticWhereRule(rules, "ordinary links", []string{"a[href]"})
	if err != nil {
		return err
	}
	visitedIndex, visitedRule, err := findTask2SemanticWhereRule(rules, "visited links", []string{"a[href]:visited"})
	if err != nil {
		return err
	}
	fieldIndex, fieldRule, err := findTask2SemanticWhereRule(rules, "form fields", []string{
		"input:not([type='button']):not([type='submit']):not([type='reset'])",
		"select",
		"textarea",
	})
	if err != nil {
		return err
	}
	actionIndex, actionRule, err := findTask2SemanticWhereRule(rules, "button/actions", []string{
		"button",
		"input:is([type='button'],[type='submit'],[type='reset'])",
		"[role='button']",
		".btn",
		".page-kit-action",
		".page-kit-icon-button",
		".site-chrome-menu-button",
		"a.tech-pill",
	})
	if err != nil {
		return err
	}

	if !(ordinaryIndex < visitedIndex && visitedIndex < fieldIndex && fieldIndex < actionIndex) {
		return fmt.Errorf(
			"forced-colors semantic source order is ordinary(%d), visited(%d), fields(%d), actions(%d); want ordinary < visited < fields < actions so visited action links end as buttons",
			ordinaryIndex,
			visitedIndex,
			fieldIndex,
			actionIndex,
		)
	}

	checks := []struct {
		label        string
		rule         task2CSSBlock
		property     string
		value        string
		displayValue string
	}{
		{"ordinary links", ordinaryRule, "background", "transparent !important", "transparent"},
		{"ordinary links", ordinaryRule, "color", "LinkText !important", "LinkText"},
		{"visited links", visitedRule, "color", "VisitedText !important", "VisitedText"},
		{"form fields", fieldRule, "background", "Field !important", "Field"},
		{"form fields", fieldRule, "color", "FieldText !important", "FieldText"},
		{"button/actions", actionRule, "background", "ButtonFace !important", "ButtonFace"},
		{"button/actions", actionRule, "color", "ButtonText !important", "ButtonText"},
		{"button/actions", actionRule, "border", "var(--line-accent) solid ButtonBorder !important", "ButtonBorder"},
	}
	for _, check := range checks {
		got := task2Declarations(check.rule.body)[check.property]
		if !task2CSSValueEqual(got, check.value) {
			return fmt.Errorf("forced-colors %s %s = %q; want %s with !important", check.label, check.property, got, check.displayValue)
		}
	}

	return nil
}

func collectTask2ForcedColorRules(css string) ([]task2CSSBlock, error) {
	blocks, err := parseTask2CSSBlocks(css)
	if err != nil {
		return nil, err
	}

	var rules []task2CSSBlock
	for _, block := range blocks {
		header := strings.ToLower(task2CanonicalCSS(block.header))
		mediaCondition := strings.NewReplacer(" ", "", "\t", "", "\r", "", "\n", "").Replace(header)
		if mediaCondition == "@media(forced-colors:active)" {
			mediaRules, err := collectTask2StyleRules(block.body)
			if err != nil {
				return nil, err
			}
			rules = append(rules, mediaRules...)
			continue
		}
		if strings.HasPrefix(header, "@") {
			nestedRules, err := collectTask2ForcedColorRules(block.body)
			if err != nil {
				return nil, err
			}
			rules = append(rules, nestedRules...)
		}
	}
	return rules, nil
}

func collectTask2StyleRules(css string) ([]task2CSSBlock, error) {
	blocks, err := parseTask2CSSBlocks(css)
	if err != nil {
		return nil, err
	}

	var rules []task2CSSBlock
	for _, block := range blocks {
		if strings.HasPrefix(strings.TrimSpace(block.header), "@") {
			nestedRules, err := collectTask2StyleRules(block.body)
			if err != nil {
				return nil, err
			}
			rules = append(rules, nestedRules...)
			continue
		}
		rules = append(rules, block)
	}
	return rules, nil
}

func parseTask2CSSBlocks(css string) ([]task2CSSBlock, error) {
	commentFreeCSS, err := task2StripCSSComments(css)
	if err != nil {
		return nil, err
	}
	css = commentFreeCSS

	var blocks []task2CSSBlock
	headerStart := 0

	for index := 0; index < len(css); {
		switch {
		case strings.HasPrefix(css[index:], "/*"):
			end := strings.Index(css[index+2:], "*/")
			if end < 0 {
				return nil, fmt.Errorf("unterminated CSS comment at byte %d", index)
			}
			index += end + 4
		case css[index] == '\'' || css[index] == '"':
			next, err := task2SkipCSSString(css, index)
			if err != nil {
				return nil, err
			}
			index = next
		case css[index] == ';':
			headerStart = index + 1
			index++
		case css[index] == '{':
			header := strings.TrimSpace(css[headerStart:index])
			if header == "" {
				return nil, fmt.Errorf("empty CSS block header at byte %d", index)
			}
			end, err := task2FindClosingBrace(css, index)
			if err != nil {
				return nil, err
			}
			blocks = append(blocks, task2CSSBlock{header: header, body: css[index+1 : end]})
			index = end + 1
			headerStart = index
		case css[index] == '}':
			return nil, fmt.Errorf("unexpected closing CSS brace at byte %d", index)
		default:
			index++
		}
	}

	return blocks, nil
}

func task2StripCSSComments(css string) (string, error) {
	var stripped strings.Builder
	stripped.Grow(len(css))

	for index := 0; index < len(css); {
		switch {
		case css[index] == '\'' || css[index] == '"':
			next, err := task2SkipCSSString(css, index)
			if err != nil {
				return "", err
			}
			stripped.WriteString(css[index:next])
			index = next
		case strings.HasPrefix(css[index:], "/*"):
			end := strings.Index(css[index+2:], "*/")
			if end < 0 {
				return "", fmt.Errorf("unterminated CSS comment at byte %d", index)
			}
			stripped.WriteByte(' ')
			index += end + 4
		default:
			stripped.WriteByte(css[index])
			index++
		}
	}

	return stripped.String(), nil
}

func task2FindClosingBrace(css string, opening int) (int, error) {
	depth := 1
	for index := opening + 1; index < len(css); {
		switch {
		case strings.HasPrefix(css[index:], "/*"):
			end := strings.Index(css[index+2:], "*/")
			if end < 0 {
				return 0, fmt.Errorf("unterminated CSS comment at byte %d", index)
			}
			index += end + 4
		case css[index] == '\'' || css[index] == '"':
			next, err := task2SkipCSSString(css, index)
			if err != nil {
				return 0, err
			}
			index = next
		case css[index] == '{':
			depth++
			index++
		case css[index] == '}':
			depth--
			if depth == 0 {
				return index, nil
			}
			index++
		default:
			index++
		}
	}
	return 0, fmt.Errorf("unclosed CSS block at byte %d", opening)
}

func task2SkipCSSString(css string, opening int) (int, error) {
	quote := css[opening]
	for index := opening + 1; index < len(css); index++ {
		if css[index] == '\\' {
			index++
			continue
		}
		if css[index] == quote {
			return index + 1, nil
		}
	}
	return 0, fmt.Errorf("unterminated CSS string at byte %d", opening)
}

func findTask2SemanticWhereRule(rules []task2CSSBlock, label string, required []string) (int, task2CSSBlock, error) {
	requiredSet := make(map[string]bool, len(required))
	for _, selector := range required {
		requiredSet[task2CanonicalCSS(selector)] = true
	}

	type occurrence struct {
		index int
		rule  task2CSSBlock
	}
	var occurrences []occurrence

	for index, rule := range rules {
		topLevelSelectors := task2SplitTopLevel(rule.header, ',')
		for _, selector := range topLevelSelectors {
			selector = task2CanonicalCSS(selector)
			if !strings.HasPrefix(selector, ":where(") || !strings.HasSuffix(selector, ")") {
				continue
			}

			arguments := task2SplitTopLevel(selector[len(":where("):len(selector)-1], ',')
			argumentSet := make(map[string]bool, len(arguments))
			for _, argument := range arguments {
				argumentSet[task2CanonicalCSS(argument)] = true
			}

			containsAll := true
			for requiredSelector := range requiredSet {
				if !argumentSet[requiredSelector] {
					containsAll = false
					break
				}
			}
			if !containsAll {
				continue
			}
			if len(argumentSet) != len(requiredSet) || len(topLevelSelectors) != 1 {
				return 0, task2CSSBlock{}, fmt.Errorf("forced-colors %s rule %q has extra selectors outside its exact semantic set", label, rule.header)
			}
			occurrences = append(occurrences, occurrence{index: index, rule: rule})
		}
	}

	if len(occurrences) != 1 {
		return 0, task2CSSBlock{}, fmt.Errorf("forced-colors %s have %d exact semantic rules; want exactly one", label, len(occurrences))
	}
	return occurrences[0].index, occurrences[0].rule, nil
}

func findTask2RuleWithDeclarations(rules []task2CSSBlock, selector string, want map[string]string) (task2CSSBlock, bool) {
	for _, rule := range rules {
		if !task2SelectorListContains(rule.header, selector) {
			continue
		}
		declarations := task2Declarations(rule.body)
		matches := true
		for property, value := range want {
			if !task2CSSValueEqual(declarations[property], value) {
				matches = false
				break
			}
		}
		if matches {
			return rule, true
		}
	}
	return task2CSSBlock{}, false
}

func task2Declarations(body string) map[string]string {
	declarations := make(map[string]string)
	for _, declaration := range task2SplitTopLevel(body, ';') {
		colon := strings.Index(declaration, ":")
		if colon < 0 {
			continue
		}
		property := strings.ToLower(strings.TrimSpace(declaration[:colon]))
		if property == "" || strings.ContainsAny(property, "{}") {
			continue
		}
		declarations[property] = strings.TrimSpace(declaration[colon+1:])
	}
	return declarations
}

func task2SplitTopLevel(value string, separator byte) []string {
	var values []string
	start := 0
	parentheses := 0
	brackets := 0
	braces := 0

	for index := 0; index < len(value); index++ {
		if value[index] == '\'' || value[index] == '"' {
			next, err := task2SkipCSSString(value, index)
			if err != nil {
				break
			}
			index = next - 1
			continue
		}
		switch value[index] {
		case '(':
			parentheses++
		case ')':
			parentheses--
		case '[':
			brackets++
		case ']':
			brackets--
		case '{':
			braces++
		case '}':
			braces--
		case separator:
			if parentheses == 0 && brackets == 0 && braces == 0 {
				values = append(values, strings.TrimSpace(value[start:index]))
				start = index + 1
			}
		}
	}
	values = append(values, strings.TrimSpace(value[start:]))
	return values
}

func task2SelectorListContains(selectorList, want string) bool {
	want = task2CanonicalCSS(want)
	for _, selector := range task2SplitTopLevel(selectorList, ',') {
		if task2CanonicalCSS(selector) == want {
			return true
		}
	}
	return false
}

func task2SelectorTargetsClass(selectorList, className string) bool {
	for _, selector := range task2SplitTopLevel(selectorList, ',') {
		if strings.Contains(task2CanonicalCSS(selector), className) {
			return true
		}
	}
	return false
}

func task2CanonicalCSS(value string) string {
	value = strings.Join(strings.FieldsFunc(value, unicode.IsSpace), " ")
	for _, replacement := range [][2]string{
		{" ,", ","},
		{", ", ","},
		{"( ", "("},
		{" )", ")"},
		{"[ ", "["},
		{" ]", "]"},
		{" = ", "="},
		{" =", "="},
		{"= ", "="},
	} {
		value = strings.ReplaceAll(value, replacement[0], replacement[1])
	}
	return strings.TrimSpace(value)
}

func task2CSSValueEqual(got, want string) bool {
	normalize := func(value string) string {
		value = strings.ReplaceAll(value, "! important", "!important")
		value = strings.ReplaceAll(value, "!important", " !important")
		return task2CanonicalCSS(value)
	}
	return strings.EqualFold(normalize(got), normalize(want))
}

func task2ElementClasses(html, requiredClass string) ([]string, bool) {
	const marker = `class="`
	for remaining := html; ; {
		start := strings.Index(remaining, marker)
		if start < 0 {
			return nil, false
		}
		remaining = remaining[start+len(marker):]
		end := strings.IndexByte(remaining, '"')
		if end < 0 {
			return nil, false
		}
		classes := strings.Fields(remaining[:end])
		for _, className := range classes {
			if className == requiredClass {
				return classes, true
			}
		}
		remaining = remaining[end+1:]
	}
}

func readTask2Artifact(t *testing.T, pathParts ...string) string {
	t.Helper()

	path := filepath.Join(append([]string{"..", ".."}, pathParts...)...)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}

func readTask2TailwindSources(t *testing.T) string {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join("..", "..", "cmd", "web", "tailwind", "*.css"))
	if err != nil {
		t.Fatalf("list Tailwind CSS sources: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no Tailwind CSS sources found")
	}

	var combined strings.Builder
	for _, path := range paths {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		combined.Write(contents)
		combined.WriteByte('\n')
	}
	return combined.String()
}
