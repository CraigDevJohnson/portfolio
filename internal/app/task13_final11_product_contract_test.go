package app

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

const final11DangerBorder = "var(--line-hairline) solid color-mix(in srgb, var(--rosehip) 72%, transparent)"

var final11DangerDependencies = []string{
	"--line-hairline",
	"--rosehip",
	"--candle-oat",
	"--cocoa-cedar",
}

func final11CanonicalDangerEffects() []final10Effect {
	return []final10Effect{
		{source: "cmd/web/tailwind/components.css", selector: ".ui-action-danger", modes: final10Both, property: "border", value: final11DangerBorder + " !important"},
		{source: "cmd/web/tailwind/components.css", selector: ".ui-action-danger", modes: final10Both, property: "background", value: "color-mix(in srgb, var(--rosehip) 16%, var(--cocoa-cedar)) !important"},
		{source: "cmd/web/tailwind/components.css", selector: ".ui-action-danger", modes: final10Both, property: "color", value: "var(--candle-oat) !important"},
		{source: "cmd/web/tailwind/components.css", selector: ".ui-action-danger:hover:not(:disabled)", modes: final10Both, property: "border-color", value: "var(--rosehip) !important"},
		{source: "cmd/web/tailwind/components.css", selector: ".ui-action-danger:hover:not(:disabled)", modes: final10Both, property: "background", value: "color-mix(in srgb, var(--rosehip) 28%, var(--cocoa-cedar)) !important"},
		{source: "cmd/web/tailwind/components.css", selector: ".ui-action-danger:hover:not(:disabled)", modes: final10Both, property: "color", value: "var(--candle-oat) !important"},
	}
}

func final11CanonicalDangerDependencyEffects() []final10Effect {
	return []final10Effect{
		{source: "cmd/web/tailwind/shared.css", selector: ":root", modes: final10Both, property: "--line-hairline", value: "0.0625rem"},
		{source: "cmd/web/tailwind/theme.css", selector: ":root", modes: final10Both, property: "--rosehip", value: "#FF7FA8"},
		{source: "cmd/web/tailwind/theme.css", selector: ":root", modes: final10Both, property: "--candle-oat", value: "#FFF0D8"},
		{source: "cmd/web/tailwind/theme.css", selector: ":root", modes: final10Both, property: "--cocoa-cedar", value: "#2E2130"},
	}
}

func final11CanonicalWebkitTextFillEffects() []final10Effect {
	return []final10Effect{
		{source: "cmd/web/tailwind/components.css", selector: ".text-gradient-brand", modes: final10Both, property: "-webkit-text-fill-color", value: "transparent"},
		{source: "cmd/web/tailwind/base.css", selector: ".text-gradient-brand, .page-hero-title, .page-section-title", modes: final10Forced, property: "-webkit-text-fill-color", value: "CanvasText !important"},
		{source: "cmd/web/tailwind/portal.css", selector: ".portal-state", modes: final10Forced, property: "-webkit-text-fill-color", value: "CanvasText !important"},
		{source: "cmd/web/tailwind/portal.css", selector: ".portal-error-code", modes: final10Forced, property: "-webkit-text-fill-color", value: "CanvasText !important"},
	}
}

func TestTask13Final11DangerActionPaintContract(t *testing.T) {
	if err := validateFinal11DangerActionPaint(readFinal10Graph(t)); err != nil {
		t.Fatal(err)
	}
}

func TestTask13Final11DangerCanonicalMutationGate(t *testing.T) {
	canonical := readFinal10Graph(t)
	mutations := []struct {
		name   string
		path   string
		old    string
		new    string
		append string
	}{
		{
			name: "danger boundary loses its nonzero line",
			path: "cmd/web/tailwind/components.css",
			old:  "border: " + final11DangerBorder + " !important;",
			new:  "border-color: color-mix(in srgb, var(--rosehip) 72%, transparent) !important;",
		},
		{
			name: "danger boundary loses semantic importance",
			path: "cmd/web/tailwind/components.css",
			old:  "border: " + final11DangerBorder + " !important;",
			new:  "border: " + final11DangerBorder + ";",
		},
		{
			name: "danger boundary drops below Rose72",
			path: "cmd/web/tailwind/components.css",
			old:  "border: " + final11DangerBorder + " !important;",
			new:  "border: var(--line-hairline) solid color-mix(in srgb, var(--rosehip) 58%, transparent) !important;",
		},
		{
			name: "danger hover text changes",
			path: "cmd/web/tailwind/components.css",
			old:  "  color: var(--candle-oat) !important;\n}\n\n.page-kit-action:disabled",
			new:  "  color: var(--night-mulberry) !important;\n}\n\n.page-kit-action:disabled",
		},
		{
			name: "canonical danger base moves into reduced motion",
			path: "cmd/web/tailwind/components.css",
			old: ".ui-action-danger {\n" +
				"  border: " + final11DangerBorder + " !important;\n" +
				"  background: color-mix(in srgb, var(--rosehip) 16%, var(--cocoa-cedar)) !important;\n" +
				"  color: var(--candle-oat) !important;\n" +
				"}",
			new: "@media (prefers-reduced-motion: reduce) {\n" +
				"  .ui-action-danger {\n" +
				"    border: " + final11DangerBorder + " !important;\n" +
				"    background: color-mix(in srgb, var(--rosehip) 16%, var(--cocoa-cedar)) !important;\n" +
				"    color: var(--candle-oat) !important;\n" +
				"  }\n" +
				"}",
		},
		{
			name:   "canonical danger owner is duplicated",
			path:   "cmd/web/tailwind/components.css",
			append: "\n.ui-action-danger { color: var(--candle-oat) !important; }\n",
		},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			mutated, changed := canonical.mutate(mutation.path, mutation.old, mutation.new, mutation.append)
			if !changed {
				t.Fatalf("danger mutation target %q is missing from %s", mutation.old, mutation.path)
			}
			if err := validateFinal11DangerActionPaint(mutated); err == nil {
				t.Fatal("danger action validator accepted a canonical paint regression")
			}
		})
	}
}

func TestTask13Final11DominantLayerPaintMutationGate(t *testing.T) {
	canonical := readFinal10Graph(t)
	fixtures := []struct {
		name string
		path string
		css  string
	}{
		{name: "base unrelated important color", path: "cmd/web/tailwind/base.css", css: "\n.unrelated-control { color: var(--night-mulberry) !important; }\n"},
		{name: "shared nested universal important background", path: "cmd/web/tailwind/shared.css", css: "\n.games-section-cta-group * { background: var(--candle-oat) !important; }\n"},
		{name: "theme important border apply", path: "cmd/web/tailwind/theme.css", css: "\n.unrelated-control { @apply !border-0; }\n"},
		{name: "base statement-important text apply", path: "cmd/web/tailwind/base.css", css: "\n.unrelated-control { @apply text-night-mulberry !important; }\n"},
		{name: "components important all reset", path: "cmd/web/tailwind/components.css", css: "\n.unrelated-control { all: unset !important; }\n"},
		{name: "base important color separates importance tokens", path: "cmd/web/tailwind/base.css", css: "\n.unrelated-control { color: transparent ! important; }\n"},
		{name: "base apply separates importance tokens with a comment", path: "cmd/web/tailwind/base.css", css: "\n.unrelated-control { @apply text-night-mulberry !/**/important; }\n"},
		{name: "direct app nested base paint", path: "cmd/web/tailwind/app.css", css: "\n@layer base { .games-section-cta-group * { border: 0 !important; } }\n"},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			mutated, changed := canonical.mutate(fixture.path, "", "", fixture.css)
			if !changed {
				t.Fatal("could not append dominant-layer paint mutation")
			}
			if err := validateFinal11DangerActionPaint(mutated); err == nil {
				t.Fatal("dominant-layer paint audit accepted a competing important effect")
			}
		})
	}
}

func TestTask13Final11LowerPriorityPaintRemainsOutOfScope(t *testing.T) {
	canonical := readFinal10Graph(t)
	fixtures := []struct {
		name string
		path string
		css  string
	}{
		{name: "page important paint is weaker than component important owner", path: "cmd/web/tailwind/portal.css", css: "\n.ui-action-danger { border: 0 !important; }\n"},
		{name: "base nonimportant paint cannot beat important owner", path: "cmd/web/tailwind/base.css", css: "\n.games-section-cta-group * { background: var(--candle-oat); }\n"},
		{name: "forced-only paint remains owned by the forced oracle", path: "cmd/web/tailwind/base.css", css: "\n@media (forced-colors: active) { .btn-primary { color: Canvas !important; } }\n"},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			mutated, changed := canonical.mutate(fixture.path, "", "", fixture.css)
			if !changed {
				t.Fatal("could not append lower-priority paint fixture")
			}
			if err := validateFinal11DangerActionPaint(mutated); err != nil {
				t.Fatalf("lower-priority paint entered the dominant audit: %v", err)
			}
		})
	}
}

func TestTask13Final11DependencyMutationGate(t *testing.T) {
	canonical := readFinal10Graph(t)
	mutations := []struct {
		name   string
		path   string
		old    string
		new    string
		append string
	}{
		{name: "line owner changes", path: "cmd/web/tailwind/shared.css", old: "--line-hairline: 0.0625rem;", new: "--line-hairline: 0;"},
		{name: "rose owner changes", path: "cmd/web/tailwind/theme.css", old: "--rosehip: #FF7FA8;", new: "--rosehip: transparent;"},
		{name: "text owner changes", path: "cmd/web/tailwind/theme.css", old: "--candle-oat: #FFF0D8;", new: "--candle-oat: var(--night-mulberry);"},
		{name: "surface owner changes", path: "cmd/web/tailwind/theme.css", old: "--cocoa-cedar: #2E2130;", new: "--cocoa-cedar: var(--candle-oat);"},
		{name: "ancestor adds a direct dependency owner", path: "cmd/web/tailwind/portal.css", append: "\n.games-section-cta-group { --candle-oat: var(--night-mulberry); }\n"},
		{name: "ancestor adds an arbitrary dependency owner", path: "cmd/web/tailwind/portal.css", append: "\n.games-section-cta-group { @apply [--cocoa-cedar:#fff0d8]; }\n"},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			mutated, changed := canonical.mutate(mutation.path, mutation.old, mutation.new, mutation.append)
			if !changed {
				t.Fatalf("dependency mutation target %q is missing from %s", mutation.old, mutation.path)
			}
			if err := validateFinal11DangerActionPaint(mutated); err == nil {
				t.Fatal("danger action validator accepted a dependency regression")
			}
		})
	}
}

func TestTask13Final11WebkitTextFillMutationGate(t *testing.T) {
	canonical := readFinal10Graph(t)
	mutations := []struct {
		name   string
		path   string
		old    string
		new    string
		append string
	}{
		{name: "canonical ordinary owner changes", path: "cmd/web/tailwind/components.css", old: "-webkit-text-fill-color: transparent;", new: "-webkit-text-fill-color: var(--night-mulberry);"},
		{name: "canonical forced owner changes", path: "cmd/web/tailwind/base.css", old: "-webkit-text-fill-color: CanvasText !important;", new: "-webkit-text-fill-color: Canvas !important;"},
		{name: "canonical forced owner gains a print condition", path: "cmd/web/tailwind/base.css", old: "@media (forced-colors: active) {", new: "@media print and (forced-colors: active) {"},
		{name: "root adds an inherited fill", path: "cmd/web/tailwind/portal.css", append: "\n:root { -webkit-text-fill-color: var(--night-mulberry); }\n"},
		{name: "forced ancestor adds an inherited fill", path: "cmd/web/tailwind/portal.css", append: "\n@media (forced-colors: active) { .games-section-cta-group { -webkit-text-fill-color: Canvas; } }\n"},
		{name: "ancestor applies an inherited fill", path: "cmd/web/tailwind/portal.css", append: "\n.games-section-cta-group { @apply [-webkit-text-fill-color:var(--night-mulberry)]; }\n"},
		{name: "first line paints existing text", path: "cmd/web/tailwind/portal.css", append: "\n.ui-action-danger::first-line { -webkit-text-fill-color: var(--night-mulberry); }\n"},
		{name: "first letter applies existing text paint", path: "cmd/web/tailwind/portal.css", append: "\nbutton:first-letter { @apply [-webkit-text-fill-color:var(--night-mulberry)]; }\n"},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			mutated, changed := canonical.mutate(mutation.path, mutation.old, mutation.new, mutation.append)
			if !changed {
				t.Fatalf("webkit fill mutation target %q is missing from %s", mutation.old, mutation.path)
			}
			if err := validateFinal11DangerActionPaint(mutated); err == nil {
				t.Fatal("danger action validator accepted inherited glyph paint")
			}
		})
	}
}

func TestTask13Final11GeneratedWebkitFillOwnersRemainOutOfScope(t *testing.T) {
	graph := readFinal10Graph(t)
	graph, _ = graph.mutate(
		"cmd/web/tailwind/portal.css",
		"",
		"",
		"\n.fixture::before { -webkit-text-fill-color: var(--night-mulberry); }\n.fixture:after { @apply [-webkit-text-fill-color:var(--night-mulberry)]; }\n",
	)
	if err := validateFinal11DangerActionPaint(graph); err != nil {
		t.Fatal(err)
	}
}

func validateFinal11DangerActionPaint(graph final10Graph) error {
	roles, err := validateFinal13LayerManifest(graph)
	if err != nil {
		return err
	}
	rules, err := collectFinal10Rules(graph)
	if err != nil {
		return err
	}
	topLevel, directForced, err := final13CollectDirectOwnerRules(graph)
	if err != nil {
		return err
	}
	effects := final11CanonicalDangerEffects()
	for effectIndex := range effects {
		effect := &effects[effectIndex]
		count := 0
		for ruleIndex := range rules {
			rule := &rules[ruleIndex]
			if value, ok := rule.declarations[effect.property]; ok && final13ExactEffect(rule, effect.property, value, effect) {
				count++
			}
		}
		if count != 1 {
			return fmt.Errorf("canonical danger effect %s %s has %d exact important owners, want 1", effect.selector, effect.property, count)
		}
		if count := final13DirectOwnerCount(topLevel, directForced, effect); count != 1 {
			return fmt.Errorf("canonical danger effect %s %s has %d exact direct-context owners, want 1", effect.selector, effect.property, count)
		}
	}
	if err := validateFinal11DangerDependencies(graph, rules); err != nil {
		return err
	}
	if err := validateFinal11WebkitTextFillOwners(rules, topLevel, directForced); err != nil {
		return err
	}
	return validateFinal11DominantImportantPaint(rules, roles)
}

func validateFinal11DominantImportantPaint(rules []final10Rule, roles map[string]string) error {
	for ruleIndex := range rules {
		rule := &rules[ruleIndex]
		if rule.modes&final10Ordinary == 0 || !final11DominantImportantRole(roles[rule.source]) {
			continue
		}
		for property, value := range rule.declarations {
			if !final11DangerPaintProperty(property) || !final11ImportantCSS(value) {
				continue
			}
			if !final11CanonicalDangerEffect(rule, property, value) {
				return fmt.Errorf("dominant layer has competing important %s:%s in %s %q", property, value, rule.source, rule.selector)
			}
		}
		if utility := final11ImportantDangerPaintApply(rule.body); utility != "" {
			return fmt.Errorf("dominant layer has competing important @apply %s in %s %q", utility, rule.source, rule.selector)
		}
	}
	return nil
}

func final11DominantImportantRole(role string) bool {
	return role == "theme" || role == "base" || role == "components"
}

func final11CanonicalDangerEffect(rule *final10Rule, property, value string) bool {
	effects := final11CanonicalDangerEffects()
	for effectIndex := range effects {
		if final13ExactEffect(rule, property, value, &effects[effectIndex]) {
			return true
		}
	}
	return false
}

func validateFinal11WebkitTextFillOwners(rules, topLevel, directForced []final10Rule) error {
	effects := final11CanonicalWebkitTextFillEffects()
	ownerCounts := make([]int, len(effects))
	for ruleIndex := range rules {
		rule := &rules[ruleIndex]
		if value, ok := rule.declarations["-webkit-text-fill-color"]; ok {
			owner := -1
			for index := range effects {
				if final13ExactEffect(rule, "-webkit-text-fill-color", value, &effects[index]) {
					owner = index
					break
				}
			}
			if owner >= 0 {
				ownerCounts[owner]++
			} else if final11WebkitTextFillHasRealElementOwner(rule.selector) {
				return fmt.Errorf("danger action inherited webkit text fill has extra or changed owner %s in %s %q", value, rule.source, rule.selector)
			}
		}
		if utility := final11DangerWebkitTextFillApply(rule.body); utility != "" && final11WebkitTextFillHasRealElementOwner(rule.selector) {
			return fmt.Errorf("danger action inherited webkit text fill has extra @apply owner %s in %s %q", utility, rule.source, rule.selector)
		}
	}
	for index := range effects {
		effect := &effects[index]
		if ownerCounts[index] != 1 {
			return fmt.Errorf("canonical webkit text fill owner %s %q has %d exact owners, want 1", effect.source, effect.selector, ownerCounts[index])
		}
		if count := final13DirectOwnerCount(topLevel, directForced, effect); count != 1 {
			return fmt.Errorf("canonical webkit text fill owner %s %q has %d exact direct-context owners, want 1", effect.source, effect.selector, count)
		}
	}
	return nil
}

func final11WebkitTextFillHasRealElementOwner(selector string) bool {
	branches, err := final10ExpandSelectorList(selector)
	if err != nil {
		return true
	}
	for _, branch := range branches {
		if !final11WebkitTextFillGeneratedPseudoOwner(final10SelectorSubject(branch)) {
			return true
		}
	}
	return false
}

func final11WebkitTextFillGeneratedPseudoOwner(subject string) bool {
	normalized := strings.ToLower(task2CanonicalCSS(subject))
	return strings.HasSuffix(normalized, "::before") ||
		strings.HasSuffix(normalized, "::after") ||
		strings.HasSuffix(normalized, ":before") ||
		strings.HasSuffix(normalized, ":after")
}

type final11DangerDependencyOwner struct {
	source   string
	selector string
	property string
	value    string
}

func validateFinal11DangerDependencies(graph final10Graph, rules []final10Rule) error {
	ownerCounts := map[string]int{}
	owners, err := final11DangerDependencyOwners(graph)
	if err != nil {
		return err
	}
	for _, owner := range owners {
		if !final11CanonicalDangerDependencyOwner(owner) {
			return fmt.Errorf("danger action dependency has extra or changed owner %s:%s in %s %q", owner.property, owner.value, owner.source, owner.selector)
		}
		ownerCounts[owner.property]++
	}
	for _, rule := range rules {
		if utility := final11DangerDependencyApply(rule.body); utility != "" {
			return fmt.Errorf("danger action dependency has extra @apply owner %s in %s %q", utility, rule.source, rule.selector)
		}
	}
	for _, effect := range final11CanonicalDangerDependencyEffects() {
		if count := ownerCounts[effect.property]; count != 1 {
			return fmt.Errorf("canonical danger dependency %s has %d exact owners, want 1", effect.property, count)
		}
	}
	return nil
}

func final11CanonicalDangerDependencyOwner(owner final11DangerDependencyOwner) bool {
	for _, effect := range final11CanonicalDangerDependencyEffects() {
		if owner.source == effect.source &&
			task2CanonicalCSS(owner.selector) == task2CanonicalCSS(effect.selector) &&
			owner.property == effect.property &&
			task2CSSValueEqual(owner.value, effect.value) {
			return true
		}
	}
	return false
}

func final11DangerDependencyOwners(graph final10Graph) ([]final11DangerDependencyOwner, error) {
	var owners []final11DangerDependencyOwner
	for _, source := range graph.sources {
		collected, err := final11DangerDependencyOwnersInCSS(source.path, source.css)
		if err != nil {
			return nil, fmt.Errorf("parse danger dependencies in %s: %w", source.path, err)
		}
		owners = append(owners, collected...)
	}
	return owners, nil
}

func final11DangerDependencyOwnersInCSS(source, css string) ([]final11DangerDependencyOwner, error) {
	blocks, err := parseTask2CSSBlocks(css)
	if err != nil {
		return nil, err
	}
	var owners []final11DangerDependencyOwner
	for _, block := range blocks {
		header := strings.TrimSpace(block.header)
		selector := header
		if strings.EqualFold(task2CanonicalCSS(header), "@theme") {
			selector = ":root"
		}
		for _, declaration := range task2SplitTopLevel(block.body, ';') {
			colon := strings.Index(declaration, ":")
			if colon < 0 {
				continue
			}
			property := strings.ToLower(strings.TrimSpace(declaration[:colon]))
			if strings.ContainsAny(property, "{}") || !final11DangerDependencyProperty(property) {
				continue
			}
			owners = append(owners, final11DangerDependencyOwner{
				source:   source,
				selector: selector,
				property: property,
				value:    strings.TrimSpace(declaration[colon+1:]),
			})
		}
		if strings.HasPrefix(header, "@") {
			nested, nestedErr := final11DangerDependencyOwnersInCSS(source, block.body)
			if nestedErr != nil {
				return nil, nestedErr
			}
			owners = append(owners, nested...)
		}
	}
	return owners, nil
}

func final11DangerDependencyProperty(property string) bool {
	for _, dependency := range final11DangerDependencies {
		if property == dependency {
			return true
		}
	}
	return false
}

func final11DangerDependencyApply(body string) string {
	for _, match := range final10ApplyPattern.FindAllStringSubmatch(body, -1) {
		for _, utility := range strings.Fields(match[1]) {
			if final11DangerDependencyUtility(utility) {
				return utility
			}
		}
	}
	return ""
}

func final11DangerDependencyUtility(utility string) bool {
	lower := strings.ToLower(final10UtilityBase(utility))
	for _, dependency := range final11DangerDependencies {
		if strings.HasPrefix(lower, "["+dependency+":") {
			return true
		}
	}
	return false
}

func final11DangerWebkitTextFillApply(body string) string {
	for _, match := range final10ApplyPattern.FindAllStringSubmatch(body, -1) {
		for _, utility := range strings.Fields(match[1]) {
			if strings.HasPrefix(strings.ToLower(final10UtilityBase(utility)), "[-webkit-text-fill-color:") {
				return utility
			}
		}
	}
	return ""
}

func final11ImportantDangerPaintApply(body string) string {
	for _, match := range final10ApplyPattern.FindAllStringSubmatch(body, -1) {
		statementImportant := final11ImportantCSS(match[1])
		for _, utility := range strings.Fields(match[1]) {
			if strings.EqualFold(strings.TrimSpace(utility), "!important") || !final11DangerPaintUtility(utility) {
				continue
			}
			utilityImportant := strings.HasPrefix(utility, "!") || strings.HasSuffix(utility, "!") || strings.Contains(utility, ":!")
			if statementImportant || utilityImportant {
				return utility
			}
		}
	}
	return ""
}

var final11ImportantPattern = regexp.MustCompile(`(?i)!\s*important\b`)

func final11ImportantCSS(value string) bool {
	return final11ImportantPattern.MatchString(value)
}

func final11DangerPaintUtility(utility string) bool {
	lower := strings.ToLower(final10UtilityBase(utility))
	return final11DangerDependencyUtility(utility) ||
		lower == "all" ||
		strings.HasPrefix(lower, "all-") ||
		lower == "border" ||
		strings.HasPrefix(lower, "border-") ||
		strings.HasPrefix(lower, "bg-") ||
		strings.HasPrefix(lower, "text-") ||
		strings.HasPrefix(lower, "[color:") ||
		strings.HasPrefix(lower, "[-webkit-text-fill-color:") ||
		strings.HasPrefix(lower, "[background") ||
		strings.HasPrefix(lower, "[border")
}

func final11DangerPaintProperty(property string) bool {
	return property == "all" ||
		final11DangerDependencyProperty(property) ||
		property == "color" ||
		property == "-webkit-text-fill-color" ||
		property == "background" ||
		strings.HasPrefix(property, "background-") ||
		property == "border" ||
		strings.HasPrefix(property, "border-") && !strings.HasPrefix(property, "border-radius")
}
