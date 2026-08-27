package app

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type final13ImportRole struct {
	source string
	role   string
}

var final13ImportManifest = []final13ImportRole{
	{source: "cmd/web/tailwind/theme.css", role: "theme"},
	{source: "cmd/web/tailwind/shared.css", role: "theme"},
	{source: "cmd/web/tailwind/base.css", role: "base"},
	{source: "cmd/web/tailwind/components.css", role: "components"},
	{source: "cmd/web/tailwind/pages/home.css", role: "pages"},
	{source: "cmd/web/tailwind/pages/about.css", role: "pages"},
	{source: "cmd/web/tailwind/pages/experience.css", role: "pages"},
	{source: "cmd/web/tailwind/pages/skills.css", role: "pages"},
	{source: "cmd/web/tailwind/pages/projects.css", role: "pages"},
	{source: "cmd/web/tailwind/pages/education.css", role: "pages"},
	{source: "cmd/web/tailwind/pages/contact.css", role: "pages"},
	{source: "cmd/web/tailwind/soccer.css", role: "pages"},
	{source: "cmd/web/tailwind/portal.css", role: "pages"},
}

type final13SourceDirective struct {
	statement string
	root      string
}

var final13SourceManifest = []final13SourceDirective{
	{statement: `@source "../layouts";`, root: filepath.Join("..", "..", "cmd", "web", "layouts")},
	{statement: `@source "../pages";`, root: filepath.Join("..", "..", "cmd", "web", "pages")},
	{statement: `@source "../partials";`, root: filepath.Join("..", "..", "cmd", "web", "partials")},
	{statement: `@source "../static/js";`, root: filepath.Join("..", "..", "cmd", "web", "static", "js")},
}

type final13ShadowOwner struct {
	effect final10Effect
	role   string
}

func final13CanonicalShadowOwners() []final13ShadowOwner {
	return []final13ShadowOwner{
		{effect: final10Effect{source: "cmd/web/tailwind/components.css", selector: ".page-hero-title", modes: final10Both, property: "text-shadow", value: "none !important"}, role: "components"},
		{effect: final10Effect{source: "cmd/web/tailwind/components.css", selector: ".text-gradient-brand", modes: final10Both, property: "text-shadow", value: "none"}, role: "components"},
		{effect: final10Effect{source: "cmd/web/tailwind/components.css", selector: ".site-chrome-menu-label", modes: final10Both, property: "text-shadow", value: "none"}, role: "components"},
		{effect: final10Effect{source: "cmd/web/tailwind/pages/home.css", selector: ".home-build-caption", modes: final10Both, property: "text-shadow", value: "0 0.125rem 0.75rem var(--night-mulberry)"}, role: "pages"},
		{effect: final10Effect{source: "cmd/web/tailwind/base.css", selector: ".text-gradient-brand, .page-hero-title, .page-section-title", modes: final10Forced, property: "text-shadow", value: "none !important"}, role: "base"},
		{effect: final10Effect{source: "cmd/web/tailwind/portal.css", selector: ".portal-error-code", modes: final10Forced, property: "text-shadow", value: "none !important"}, role: "pages"},
	}
}

func final13CanonicalWebkitTextFillOwners() []final10Effect {
	return []final10Effect{
		{source: "cmd/web/tailwind/components.css", selector: ".text-gradient-brand", modes: final10Both, property: "-webkit-text-fill-color", value: "transparent"},
		{source: "cmd/web/tailwind/base.css", selector: ".text-gradient-brand, .page-hero-title, .page-section-title", modes: final10Forced, property: "-webkit-text-fill-color", value: "CanvasText !important"},
		{source: "cmd/web/tailwind/portal.css", selector: ".portal-state", modes: final10Forced, property: "-webkit-text-fill-color", value: "CanvasText !important"},
		{source: "cmd/web/tailwind/portal.css", selector: ".portal-error-code", modes: final10Forced, property: "-webkit-text-fill-color", value: "CanvasText !important"},
	}
}

func TestTask13Final13ManifestContracts(t *testing.T) {
	graph := readFinal10Graph(t)
	if err := validateFinal13BoundedCSSGrammar(graph); err != nil {
		t.Fatal(err)
	}
	if _, err := validateFinal13LayerManifest(graph); err != nil {
		t.Fatal(err)
	}
	if err := validateFinal13TextShadowInventory(graph); err != nil {
		t.Fatal(err)
	}
	if err := validateFinal13WebkitTextFillInventory(graph); err != nil {
		t.Fatal(err)
	}
	if err := validateFinal13GlyphPseudoPaint(graph); err != nil {
		t.Fatal(err)
	}
}

func TestTask13Final13LayerManifestMutationGate(t *testing.T) {
	canonical := readFinal10Graph(t)
	mutations := []struct {
		name   string
		path   string
		old    string
		new    string
		append string
	}{
		{name: "named layer order changes", path: "cmd/web/tailwind/app.css", old: "@layer theme, base, components, pages, utilities;", new: "@layer theme, components, base, pages, utilities;"},
		{name: "page import moves into base", path: "cmd/web/tailwind/app.css", old: `@import "./pages/home.css" layer(pages);`, new: `@import "./pages/home.css" layer(base);`},
		{name: "page import moves into components", path: "cmd/web/tailwind/app.css", old: `@import "./pages/home.css" layer(pages);`, new: `@import "./pages/home.css" layer(components);`},
		{name: "base and component import order changes", path: "cmd/web/tailwind/app.css", old: "@import \"./base.css\" layer(base);\n@import \"./components.css\" layer(components);", new: "@import \"./components.css\" layer(components);\n@import \"./base.css\" layer(base);"},
		{name: "base and component roles swap", path: "cmd/web/tailwind/app.css", old: "@import \"./base.css\" layer(base);\n@import \"./components.css\" layer(components);", new: "@import \"./base.css\" layer(components);\n@import \"./components.css\" layer(base);"},
		{name: "component import gains a print condition", path: "cmd/web/tailwind/app.css", old: `@import "./components.css" layer(components);`, new: `@import "./components.css" layer(components) print;`},
		{name: "direct app nested base layer appears", path: "cmd/web/tailwind/app.css", append: "\n@layer base { .nested-reset { color: transparent !important; } }\n"},
		{name: "page source opens a dominant theme layer", path: "cmd/web/tailwind/pages/home.css", append: "\n@layer theme { .nested-reset { color: transparent !important; } }\n"},
		{name: "non-local theme import appears", path: "cmd/web/tailwind/app.css", append: "\n@import 'package.css' layer(theme);\n"},
		{name: "source directive appears", path: "cmd/web/tailwind/app.css", append: "\n@source \"../unknown\";\n"},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			mutated, changed := canonical.mutate(mutation.path, mutation.old, mutation.new, mutation.append)
			if !changed {
				t.Fatalf("layer mutation target %q is missing from %s", mutation.old, mutation.path)
			}
			if _, err := validateFinal13LayerManifest(mutated); err == nil {
				t.Fatal("layer manifest accepted authored cascade drift")
			}
		})
	}
}

func TestTask13Final13TextShadowInventoryMutationGate(t *testing.T) {
	canonical := readFinal10Graph(t)
	mutations := []struct {
		name   string
		path   string
		old    string
		new    string
		append string
	}{
		{name: "canonical title neutralizer is deleted", path: "cmd/web/tailwind/components.css", old: "  text-shadow: none !important;\n}", new: "}"},
		{name: "canonical title neutralizer loses importance", path: "cmd/web/tailwind/components.css", old: "  text-shadow: none !important;\n}", new: "  text-shadow: none;\n}"},
		{
			name: "canonical title neutralizer moves into print",
			path: "cmd/web/tailwind/components.css",
			old: ".page-hero-title {\n" +
				"  max-width: 12ch;\n" +
				"  color: var(--candle-oat);\n" +
				"  font-family: var(--font-display);\n" +
				"  font-size: clamp(3.35rem, 6.8vw, 6.8rem);\n" +
				"  font-weight: 720;\n" +
				"  letter-spacing: -0.065em;\n" +
				"  line-height: 0.88;\n" +
				"  text-wrap: balance;\n" +
				"  text-shadow: none !important;\n" +
				"}",
			new: "@media print {\n" +
				"  .page-hero-title {\n" +
				"    max-width: 12ch;\n" +
				"    color: var(--candle-oat);\n" +
				"    font-family: var(--font-display);\n" +
				"    font-size: clamp(3.35rem, 6.8vw, 6.8rem);\n" +
				"    font-weight: 720;\n" +
				"    letter-spacing: -0.065em;\n" +
				"    line-height: 0.88;\n" +
				"    text-wrap: balance;\n" +
				"    text-shadow: none !important;\n" +
				"  }\n" +
				"}",
		},
		{name: "home caption shadow changes", path: "cmd/web/tailwind/pages/home.css", old: "text-shadow: 0 0.125rem 0.75rem var(--night-mulberry);", new: "text-shadow: none;"},
		{name: "new direct shadow owner", path: "cmd/web/tailwind/portal.css", append: "\n.unrelated { text-shadow: 0 0.1rem var(--rosehip); }\n"},
		{name: "new pseudo shadow owner", path: "cmd/web/tailwind/portal.css", append: "\n.unrelated::before { text-shadow: none; }\n"},
		{name: "new apply shadow owner", path: "cmd/web/tailwind/portal.css", append: "\n.unrelated { @apply text-shadow-sm; }\n"},
		{name: "new arbitrary apply shadow owner", path: "cmd/web/tailwind/portal.css", append: "\n.unrelated { @apply [text-shadow:0_0.1rem_var(--rosehip)]; }\n"},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			mutated, changed := canonical.mutate(mutation.path, mutation.old, mutation.new, mutation.append)
			if !changed {
				t.Fatalf("shadow mutation target %q is missing from %s", mutation.old, mutation.path)
			}
			if err := validateFinal13TextShadowInventory(mutated); err == nil {
				t.Fatal("text-shadow inventory accepted an unowned effect")
			}
		})
	}
}

func TestTask13Final13WebkitTextFillInventoryMutationGate(t *testing.T) {
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
		{name: "new direct owner", path: "cmd/web/tailwind/portal.css", append: "\n.fixture { -webkit-text-fill-color: var(--night-mulberry); }\n"},
		{name: "new apply owner", path: "cmd/web/tailwind/portal.css", append: "\n.fixture { @apply [-webkit-text-fill-color:var(--night-mulberry)]; }\n"},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			mutated, changed := canonical.mutate(mutation.path, mutation.old, mutation.new, mutation.append)
			if !changed {
				t.Fatalf("webkit text-fill mutation target %q is missing from %s", mutation.old, mutation.path)
			}
			if err := validateFinal13WebkitTextFillInventory(mutated); err == nil {
				t.Fatal("webkit text-fill inventory accepted an unowned effect")
			}
		})
	}
}

func TestTask13Final13GlyphPseudoPaintMutationGate(t *testing.T) {
	canonical := readFinal10Graph(t)
	fixtures := []struct {
		name string
		css  string
	}{
		{name: "first line color", css: "\n.ui-action-secondary::first-line { color: var(--night-mulberry); }\n"},
		{name: "legacy first letter background", css: "\nbutton:first-letter { background: var(--candle-oat); }\n"},
		{name: "first line background component", css: "\n.title::first-line { background-color: var(--rosehip); }\n"},
		{name: "first line webkit fill", css: "\n.title::first-line { -webkit-text-fill-color: transparent; }\n"},
		{name: "first letter text shadow", css: "\n.title::first-letter { text-shadow: none; }\n"},
		{name: "legacy first line all reset", css: "\n.title:first-line { all: revert; }\n"},
		{name: "first letter apply paint", css: "\n.title:first-letter { @apply text-night-mulberry bg-candle-oat; }\n"},
		{name: "first line arbitrary apply paint", css: "\n.title::first-line { @apply [background-color:transparent]; }\n"},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			mutated, changed := canonical.mutate("cmd/web/tailwind/app.css", "", "", fixture.css)
			if !changed {
				t.Fatal("could not append glyph pseudo mutation")
			}
			if err := validateFinal13GlyphPseudoPaint(mutated); err == nil {
				t.Fatal("glyph pseudo audit accepted protected paint")
			}
		})
	}
}

func TestTask13Final13GeneratedPseudosRemainOutsideGlyphAudit(t *testing.T) {
	graph := readFinal10Graph(t)
	graph, _ = graph.mutate(
		"cmd/web/tailwind/app.css",
		"",
		"",
		"\n.fixture::before { color: var(--night-mulberry); background: transparent; }\n.fixture:after { @apply text-night-mulberry bg-transparent; }\n",
	)
	if err := validateFinal13GlyphPseudoPaint(graph); err != nil {
		t.Fatal(err)
	}
}

func TestTask13Final13UnsupportedCSSGrammarMutationGate(t *testing.T) {
	canonical := readFinal10Graph(t)
	fixtures := []struct {
		name string
		path string
		css  string
	}{
		{name: "native nested title shadow", path: "cmd/web/tailwind/app.css", css: "\n.scope { .page-hero-title { text-shadow: red !important; } }\n"},
		{name: "native nested action paint", path: "cmd/web/tailwind/base.css", css: "\n.scope { .ui-action-secondary { color: var(--night-mulberry) !important; } }\n"},
		{name: "local utility aliases a title shadow", path: "cmd/web/tailwind/app.css", css: "\n@utility shadow-alias { text-shadow: red; }\n.page-hero-title { @apply shadow-alias; }\n"},
		{name: "protected dependency is registered", path: "cmd/web/tailwind/app.css", css: "\n@property --candle-oat { syntax: '<color>'; inherits: false; initial-value: #17121b; }\n"},
		{name: "custom variant is declared", path: "cmd/web/tailwind/app.css", css: "\n@variant task13-glyph { .fixture { color: transparent; } }\n"},
		{name: "supports block is unowned", path: "cmd/web/tailwind/base.css", css: "\n@supports (display: grid) { .fixture { color: transparent; } }\n"},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			mutated, changed := canonical.mutate(fixture.path, "", "", fixture.css)
			if !changed {
				t.Fatal("could not append unsupported CSS grammar mutation")
			}
			if err := validateFinal13BoundedCSSGrammar(mutated); err == nil {
				t.Fatal("bounded grammar accepted a construct outside the effect manifests")
			}
		})
	}
}

func TestTask13Final13GeneratedGlyphVariantContract(t *testing.T) {
	if err := validateFinal13GeneratedGlyphVariants(); err != nil {
		t.Fatal(err)
	}
}

func TestTask13Final13GeneratedGlyphVariantMutationGate(t *testing.T) {
	for _, fixture := range []string{
		`class="first-line:text-night-mulberry"`,
		`class="sm:first-letter:bg-candle-oat"`,
		`class="[&::first-line]:text-night-mulberry"`,
		`class="[&_.ui-action-secondary::first-line]:text-night-mulberry"`,
		`class="text-shadow-sm"`,
		`class="[text-shadow:0_0.1rem_var(--rosehip)]"`,
		`class="[-webkit-text-fill-color:var(--night-mulberry)]"`,
		`class="[&_[data-task13-contrast-host]]:[-webkit-text-fill-color:var(--night-mulberry)]"`,
		`class="[--line-hairline:0] [--rosehip:transparent] [--candle-oat:#17121b] [--cocoa-cedar:#fff0d8]"`,
	} {
		if final13GeneratedGlyphVariant(fixture) == "" {
			t.Fatalf("generated glyph variant audit accepted %q", fixture)
		}
	}
}

func validateFinal13BoundedCSSGrammar(graph final10Graph) error {
	for _, source := range graph.sources {
		commentFree, err := task2StripCSSComments(source.css)
		if err != nil {
			return fmt.Errorf("parse bounded CSS grammar in %s: %w", source.path, err)
		}
		if source.path == "cmd/web/tailwind/app.css" {
			if err := validateStyleDirectiveGraph(commentFree); err != nil {
				return err
			}
			continue
		}
		atRules, err := final13CSSAtRules(commentFree)
		if err != nil {
			return fmt.Errorf("scan bounded at-rules in %s: %w", source.path, err)
		}
		for _, atRule := range atRules {
			switch atRule {
			case "apply", "keyframes", "media", "theme":
			default:
				return fmt.Errorf("%s uses unsupported @%s outside the exact effect manifests", source.path, atRule)
			}
		}
		if err := final13RejectNativeNesting(source.path, commentFree); err != nil {
			return err
		}
	}
	return nil
}

func final13CSSAtRules(css string) ([]string, error) {
	var atRules []string
	for index := 0; index < len(css); {
		if css[index] == '\'' || css[index] == '"' {
			next, err := task2SkipCSSString(css, index)
			if err != nil {
				return nil, err
			}
			index = next
			continue
		}
		if css[index] != '@' {
			index++
			continue
		}
		end := index + 1
		for end < len(css) && (css[end] >= 'a' && css[end] <= 'z' || css[end] >= 'A' && css[end] <= 'Z' || css[end] == '-') {
			end++
		}
		if end > index+1 {
			atRules = append(atRules, strings.ToLower(css[index+1:end]))
		}
		index = end
	}
	return atRules, nil
}

func final13RejectNativeNesting(source, css string) error {
	blocks, err := parseTask2CSSBlocks(css)
	if err != nil {
		return fmt.Errorf("parse bounded CSS grammar in %s: %w", source, err)
	}
	for _, block := range blocks {
		header := strings.TrimSpace(block.header)
		if strings.HasPrefix(header, "@") {
			if err := final13RejectNativeNesting(source, block.body); err != nil {
				return err
			}
			continue
		}
		nested, nestedErr := parseTask2CSSBlocks(block.body)
		if nestedErr != nil {
			return fmt.Errorf("parse declaration body in %s %q: %w", source, header, nestedErr)
		}
		if len(nested) != 0 {
			return fmt.Errorf("%s %q contains native nesting outside the exact effect manifests", source, header)
		}
	}
	return nil
}

func validateFinal13GeneratedGlyphVariants() error {
	for _, source := range final13SourceManifest {
		err := filepath.Walk(source.root, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() {
				return nil
			}
			switch strings.ToLower(filepath.Ext(path)) {
			case ".go", ".js", ".templ":
			default:
				return fmt.Errorf("configured @source file %s has an unaudited type", path)
			}
			contents, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			if token := final13GeneratedGlyphVariant(string(contents)); token != "" {
				return fmt.Errorf("%s contains unsupported generated glyph pseudo variant %q", path, token)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func final13GeneratedGlyphVariant(source string) string {
	return final13GeneratedPaintVariantPattern.FindString(source)
}

var (
	final13GlyphPseudoPattern           = regexp.MustCompile(`(?i)::?first-(?:line|letter)(?:[^-a-z0-9_]|$)`)
	final13GeneratedPaintVariantPattern = regexp.MustCompile(
		`(?i)(?:^|[^-a-z0-9_])first-(?:line|letter):|\[[^\r\n]*::?first-(?:line|letter)[^\r\n]*\]|(?:^|[^-a-z0-9_])text-shadow(?:-|:)|\[\s*-webkit-text-fill-color\s*:|\[\s*--(?:line-hairline|rosehip|candle-oat|cocoa-cedar)\s*:`,
	)
)

func validateFinal13LayerManifest(graph final10Graph) (map[string]string, error) {
	if err := validateFinal13BoundedCSSGrammar(graph); err != nil {
		return nil, err
	}
	appCSS, err := final13SourceCSS(graph, "cmd/web/tailwind/app.css")
	if err != nil {
		return nil, err
	}
	if err := validateStyleDirectiveGraph(appCSS); err != nil {
		return nil, err
	}

	roles := map[string]string{"cmd/web/tailwind/app.css": "unlayered"}
	for _, want := range final13ImportManifest {
		roles[want.source] = want.role
	}

	sourceCounts := map[string]int{}
	for _, source := range graph.sources {
		sourceCounts[source.path]++
	}
	if len(graph.sources) != len(final13ImportManifest)+1 {
		return nil, fmt.Errorf("authored graph has %d sources, want %d", len(graph.sources), len(final13ImportManifest)+1)
	}
	for source := range roles {
		if sourceCounts[source] != 1 {
			return nil, fmt.Errorf("authored source %s occurs %d times, want 1", source, sourceCounts[source])
		}
	}
	return roles, nil
}

func final13SourceCSS(graph final10Graph, path string) (string, error) {
	found := ""
	count := 0
	for _, source := range graph.sources {
		if source.path == path {
			found = source.css
			count++
		}
	}
	if count != 1 {
		return "", fmt.Errorf("authored graph has %d copies of %s, want 1", count, path)
	}
	return found, nil
}

func validateFinal13TextShadowInventory(graph final10Graph) error {
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
	owners := final13CanonicalShadowOwners()
	counts := make([]int, len(owners))
	for ruleIndex := range rules {
		rule := &rules[ruleIndex]
		if value, ok := rule.declarations["text-shadow"]; ok {
			owner := -1
			for index := range owners {
				want := &owners[index]
				if roles[rule.source] == want.role && final13ExactEffect(rule, "text-shadow", value, &want.effect) {
					owner = index
					break
				}
			}
			if owner < 0 {
				return fmt.Errorf("text-shadow inventory has extra or changed owner %s in %s %q", value, rule.source, rule.selector)
			}
			counts[owner]++
		}
		if utility := final13TextShadowApply(rule.body); utility != "" {
			return fmt.Errorf("text-shadow inventory has extra @apply owner %s in %s %q", utility, rule.source, rule.selector)
		}
	}
	for index := range owners {
		owner := &owners[index]
		if counts[index] != 1 {
			return fmt.Errorf("canonical text-shadow owner %s %q has %d exact owners, want 1", owner.effect.source, owner.effect.selector, counts[index])
		}
		if count := final13DirectOwnerCount(topLevel, directForced, &owner.effect); count != 1 {
			return fmt.Errorf("canonical text-shadow owner %s %q has %d exact direct-context owners, want 1", owner.effect.source, owner.effect.selector, count)
		}
	}
	return nil
}

func validateFinal13WebkitTextFillInventory(graph final10Graph) error {
	rules, err := collectFinal10Rules(graph)
	if err != nil {
		return err
	}
	topLevel, directForced, err := final13CollectDirectOwnerRules(graph)
	if err != nil {
		return err
	}
	owners := final13CanonicalWebkitTextFillOwners()
	counts := make([]int, len(owners))
	for ruleIndex := range rules {
		rule := &rules[ruleIndex]
		if utility := final13WebkitTextFillApply(rule.body); utility != "" && !final13GeneratedPseudoOwner(rule.selector) {
			return fmt.Errorf("webkit text-fill inventory has extra @apply owner %s in %s %q", utility, rule.source, rule.selector)
		}
		value, ok := rule.declarations["-webkit-text-fill-color"]
		if !ok {
			continue
		}
		owner := -1
		for index := range owners {
			if final13ExactEffect(rule, "-webkit-text-fill-color", value, &owners[index]) {
				owner = index
				break
			}
		}
		if owner < 0 {
			if final13GeneratedPseudoOwner(rule.selector) {
				continue
			}
			return fmt.Errorf("webkit text-fill inventory has extra or changed owner %s in %s %q", value, rule.source, rule.selector)
		}
		counts[owner]++
	}
	for index := range owners {
		owner := &owners[index]
		if counts[index] != 1 {
			return fmt.Errorf("canonical webkit text-fill owner %s %q has %d exact owners, want 1", owner.source, owner.selector, counts[index])
		}
		if count := final13DirectOwnerCount(topLevel, directForced, owner); count != 1 {
			return fmt.Errorf("canonical webkit text-fill owner %s %q has %d exact direct-context owners, want 1", owner.source, owner.selector, count)
		}
	}
	return nil
}

func final13GeneratedPseudoOwner(selector string) bool {
	branches, err := final10ExpandSelectorList(selector)
	if err != nil {
		return false
	}
	for _, branch := range branches {
		subject := strings.ToLower(task2CanonicalCSS(final10SelectorSubject(branch)))
		if !strings.HasSuffix(subject, "::before") &&
			!strings.HasSuffix(subject, "::after") &&
			!strings.HasSuffix(subject, ":before") &&
			!strings.HasSuffix(subject, ":after") {
			return false
		}
	}
	return true
}

func final13WebkitTextFillApply(body string) string {
	for _, match := range final10ApplyPattern.FindAllStringSubmatch(body, -1) {
		for _, utility := range strings.Fields(match[1]) {
			if strings.HasPrefix(strings.ToLower(final10UtilityBase(utility)), "[-webkit-text-fill-color:") {
				return utility
			}
		}
	}
	return ""
}

func final13CollectDirectOwnerRules(graph final10Graph) ([]final10Rule, []final10Rule, error) {
	var topLevel []final10Rule
	var directForced []final10Rule
	for _, source := range graph.sources {
		blocks, err := parseTask2CSSBlocks(source.css)
		if err != nil {
			return nil, nil, fmt.Errorf("parse direct owner contexts in %s: %w", source.path, err)
		}
		for _, block := range blocks {
			header := strings.TrimSpace(block.header)
			if !strings.HasPrefix(header, "@") {
				topLevel = append(topLevel, final13DirectRule(source.path, header, block.body, final10Both))
				continue
			}
			if task2CanonicalCSS(header) != task2CanonicalCSS("@media (forced-colors: active)") {
				continue
			}
			nested, nestedErr := parseTask2CSSBlocks(block.body)
			if nestedErr != nil {
				return nil, nil, fmt.Errorf("parse direct forced owners in %s: %w", source.path, nestedErr)
			}
			for _, child := range nested {
				selector := strings.TrimSpace(child.header)
				if !strings.HasPrefix(selector, "@") {
					directForced = append(directForced, final13DirectRule(source.path, selector, child.body, final10Forced))
				}
			}
		}
	}
	return topLevel, directForced, nil
}

func final13DirectRule(source, selector, body string, modes final10Mode) final10Rule {
	return final10Rule{
		source:       source,
		selector:     selector,
		body:         body,
		declarations: task2Declarations(body),
		maxWidthRem:  math.Inf(1),
		modes:        modes,
	}
}

func final13DirectOwnerCount(topLevel, directForced []final10Rule, effect *final10Effect) int {
	rules := topLevel
	if effect.modes == final10Forced {
		rules = directForced
	}
	count := 0
	for ruleIndex := range rules {
		rule := &rules[ruleIndex]
		if value, ok := rule.declarations[effect.property]; ok && final13ExactEffect(rule, effect.property, value, effect) {
			count++
		}
	}
	return count
}

func final13ExactEffect(rule *final10Rule, property, value string, effect *final10Effect) bool {
	return rule.source == effect.source &&
		task2CanonicalCSS(rule.selector) == task2CanonicalCSS(effect.selector) &&
		math.Abs(rule.minWidthRem-effect.minWidthRem) < 0.0001 &&
		math.IsInf(rule.maxWidthRem, 1) &&
		rule.modes == effect.modes &&
		!rule.unsupportedMedia &&
		!rule.resetLayer &&
		property == effect.property &&
		task2CSSValueEqual(value, effect.value)
}

func final13TextShadowApply(body string) string {
	for _, match := range final10ApplyPattern.FindAllStringSubmatch(body, -1) {
		for _, utility := range strings.Fields(match[1]) {
			lower := strings.ToLower(final10UtilityBase(utility))
			if lower == "text-shadow" || strings.HasPrefix(lower, "text-shadow-") || strings.HasPrefix(lower, "[text-shadow:") {
				return utility
			}
		}
	}
	return ""
}

func validateFinal13GlyphPseudoPaint(graph final10Graph) error {
	rules, err := collectFinal10Rules(graph)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if !final13GlyphPseudoPattern.MatchString(rule.selector) {
			continue
		}
		for property, value := range rule.declarations {
			if final13GlyphPaintProperty(property) {
				return fmt.Errorf("glyph pseudo has competing %s:%s in %s %q", property, value, rule.source, rule.selector)
			}
		}
		if utility := final13GlyphPaintApply(rule.body); utility != "" {
			return fmt.Errorf("glyph pseudo has competing @apply %s in %s %q", utility, rule.source, rule.selector)
		}
	}
	return nil
}

func final13GlyphPaintProperty(property string) bool {
	return property == "all" ||
		property == "color" ||
		property == "-webkit-text-fill-color" ||
		property == "text-shadow" ||
		property == "background" ||
		strings.HasPrefix(property, "background-")
}

func final13GlyphPaintApply(body string) string {
	for _, match := range final10ApplyPattern.FindAllStringSubmatch(body, -1) {
		for _, utility := range strings.Fields(match[1]) {
			lower := strings.ToLower(final10UtilityBase(utility))
			switch {
			case lower == "all", strings.HasPrefix(lower, "all-"):
				return utility
			case strings.HasPrefix(lower, "text-"), strings.HasPrefix(lower, "bg-"):
				return utility
			case strings.HasPrefix(lower, "[color:"), strings.HasPrefix(lower, "[-webkit-text-fill-color:"):
				return utility
			case strings.HasPrefix(lower, "[text-shadow:"), strings.HasPrefix(lower, "[background"):
				return utility
			}
		}
	}
	return ""
}
