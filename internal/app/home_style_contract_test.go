package app

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const homeWideCompositionRem = 80

var (
	homeMinWidthPattern = regexp.MustCompile(`(?i)min-width\s*:\s*([0-9]+(?:\.[0-9]+)?)rem`)
	homeTrailSelector   = regexp.MustCompile(`(^|[^a-zA-Z0-9_-])\.home-topology-trail([^a-zA-Z0-9_-]|$)`)
	homeCardPseudo      = regexp.MustCompile(`\.home-capability(?:-card|-[a-z-]+)[^,{}]*::(?:before|after)`)
)

type homeCSSRule struct {
	selector              string
	declarations          map[string]string
	appliesBelowWideWidth bool
}

func TestHomeTopologyTrailCSSContract(t *testing.T) {
	css := readTask2Artifact(t, "cmd", "web", "tailwind", "pages", "home.css")
	if err := validateHomeTopologyTrailCSS(css); err != nil {
		t.Fatal(err)
	}
}

func TestHomeTopologyTrailCSSValidatorRejectsRegressions(t *testing.T) {
	const valid = `
    .home-topology-trail { display: block; position: absolute; }
    .home-capability-card { position: relative; }
    @media (min-width: 80rem) {
      .home-topology-trail { inset: 0; }
    }
  `

	tests := []struct {
		name string
		css  string
	}{
		{
			name: "hidden by default",
			css:  strings.Replace(valid, "display: block", "display : none !important", 1),
		},
		{
			name: "hidden at tablet width",
			css: valid + `
        @media (min-width: 48rem) {
          .home-topology-map .home-topology-trail { display: none; }
        }
      `,
		},
		{
			name: "card after owns connector",
			css: valid + `
        .home-capability-card::after { content: ''; width: 1rem; background: red; }
      `,
		},
		{
			name: "named card before owns connector",
			css: valid + `
        .home-capability-cloud::before { content: ''; height: 1rem; background: red; }
      `,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateHomeTopologyTrailCSS(test.css); err == nil {
				t.Fatal("validateHomeTopologyTrailCSS() error = nil, want regression rejected")
			}
		})
	}

	if err := validateHomeTopologyTrailCSS(valid); err != nil {
		t.Fatalf("validateHomeTopologyTrailCSS(valid) error = %v", err)
	}
}

func validateHomeTopologyTrailCSS(css string) error {
	rules, err := collectHomeCSSRules(css, 0)
	if err != nil {
		return fmt.Errorf("parse Home route CSS: %w", err)
	}

	visibleBelowWide := false
	var violations []string
	for _, rule := range rules {
		canonicalSelector := task2CanonicalCSS(rule.selector)
		if homeCardPseudo.MatchString(canonicalSelector) {
			violations = append(violations, fmt.Sprintf("capability card pseudo-element %q owns topology decoration", rule.selector))
		}
		if !rule.appliesBelowWideWidth || !homeSelectorTargetsTrailRoot(canonicalSelector) {
			continue
		}

		display, declaresDisplay := rule.declarations["display"]
		if !declaresDisplay {
			continue
		}
		if homeCSSValue(display) == "none" {
			violations = append(violations, fmt.Sprintf("typed topology trail is hidden below 80rem by %q", rule.selector))
			continue
		}
		visibleBelowWide = true
	}

	if !visibleBelowWide {
		violations = append(violations, "typed topology trail has no explicit visible display below 80rem")
	}
	if len(violations) > 0 {
		return fmt.Errorf("Home topology CSS contract: %s", strings.Join(violations, "; "))
	}
	return nil
}

func homeSelectorTargetsTrailRoot(selectorList string) bool {
	for _, selector := range task2SplitTopLevel(selectorList, ',') {
		selector = task2CanonicalCSS(selector)
		lastCombinator := -1
		for _, combinator := range []string{" ", ">", "+", "~"} {
			if index := strings.LastIndex(selector, combinator); index > lastCombinator {
				lastCombinator = index
			}
		}
		target := strings.TrimSpace(selector[lastCombinator+1:])
		if !strings.Contains(target, "::") && homeTrailSelector.MatchString(target) {
			return true
		}
	}
	return false
}

func collectHomeCSSRules(css string, inheritedMinWidth float64) ([]homeCSSRule, error) {
	blocks, err := parseTask2CSSBlocks(css)
	if err != nil {
		return nil, err
	}

	var rules []homeCSSRule
	for _, block := range blocks {
		header := strings.TrimSpace(block.header)
		if strings.HasPrefix(header, "@") {
			minWidth := inheritedMinWidth
			for _, match := range homeMinWidthPattern.FindAllStringSubmatch(header, -1) {
				value, parseErr := strconv.ParseFloat(match[1], 64)
				if parseErr != nil {
					return nil, fmt.Errorf("parse media min-width %q: %w", match[1], parseErr)
				}
				if value > minWidth {
					minWidth = value
				}
			}
			nested, nestedErr := collectHomeCSSRules(block.body, minWidth)
			if nestedErr != nil {
				return nil, nestedErr
			}
			rules = append(rules, nested...)
			continue
		}

		rules = append(rules, homeCSSRule{
			selector:              header,
			declarations:          task2Declarations(block.body),
			appliesBelowWideWidth: inheritedMinWidth < homeWideCompositionRem,
		})
	}
	return rules, nil
}

func homeCSSValue(value string) string {
	value = strings.ToLower(task2CanonicalCSS(value))
	value = strings.TrimSpace(strings.TrimSuffix(value, "!important"))
	return value
}
