package app

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const (
	final10StrongBoundary = "color-mix(in srgb, var(--candle-oat) 48%, transparent)"
	final10RoseBoundary   = "color-mix(in srgb, var(--rosehip) 72%, transparent)"
	final10ApricotBorder  = "color-mix(in srgb, var(--campfire-apricot) 58%, transparent)"
	final10MintBorder     = "color-mix(in srgb, var(--pond-mint) 52%, transparent)"
)

type final10Source struct {
	path string
	css  string
}

type final10Graph struct {
	sources []final10Source
	imports []string
}

func TestTask13Final10OrdinaryProductContrastContract(t *testing.T) {
	if err := validateFinal10OrdinaryContract(readFinal10Graph(t)); err != nil {
		t.Fatal(err)
	}
}

func TestTask13Final10FocusClearanceContract(t *testing.T) {
	if err := validateFinal10FocusContract(readFinal10Graph(t)); err != nil {
		t.Fatal(err)
	}
}

func TestTask13Final10ForcedProductPaintContract(t *testing.T) {
	if err := validateFinal10ForcedContract(readFinal10Graph(t)); err != nil {
		t.Fatal(err)
	}
}

func TestTask13Final10UsesAuthoredAppCascadeOrder(t *testing.T) {
	graph := readFinal10Graph(t)
	want := []string{
		"./theme.css", "./shared.css", "./base.css", "./components.css",
		"./pages/home.css", "./pages/about.css", "./pages/experience.css",
		"./pages/skills.css", "./pages/projects.css", "./pages/education.css",
		"./pages/contact.css", "./soccer.css", "./portal.css",
	}
	if strings.Join(graph.imports, "\n") != strings.Join(want, "\n") {
		t.Fatalf("local app.css imports = %v, want %v", graph.imports, want)
	}
	if got := graph.sources[len(graph.sources)-1].path; got != "cmd/web/tailwind/app.css" {
		t.Fatalf("direct app.css rules are not last in authored graph: got %s", got)
	}
	if len(graph.sources) != len(want)+1 {
		t.Fatalf("authored graph has %d sources, want %d", len(graph.sources), len(want)+1)
	}
}

func TestTask13Final10ContrastTargetsUseCanonicalMarkup(t *testing.T) {
	checks := []struct {
		path string
		want string
	}{
		{"cmd/web/partials/skills_grid.templ", `id="skills-search"`},
		{"cmd/web/partials/skills_grid.templ", `data-skills-search-input`},
		{"cmd/web/partials/ui_types.go", `return "ui-action-secondary"`},
		{"cmd/web/partials/ui_types.go", `return "ui-feedback-error"`},
		{"cmd/web/partials/soccer_login_modal.templ", `role="dialog"`},
		{"cmd/web/partials/soccer_table_fragment.templ", `<div class="soccer-match-detail soccer-match-location" data-label="Location">`},
		{"cmd/web/partials/soccer_table_fragment.templ", `<span class="field-badge">`},
		{"cmd/web/partials/soccer_table_fragment.templ", `<div class="soccer-match-detail soccer-match-meta" data-label={ soccerGameMetaLabel(isPast) }>`},
		{"cmd/web/partials/soccer_table_fragment.templ", `<span class="season-badge">`},
		{"cmd/web/partials/soccer_table_fragment.templ", `<div class="games-table-section games-table-section--past">`},
		{"cmd/web/pages/portal_mgmt.templ", `ID:         "portal-instance-overflow"`},
		{"cmd/web/partials/portal_fragments.templ", `mergeClasses("portal-state", stateView.Class)`},
	}
	for _, check := range checks {
		parts := strings.Split(check.path, "/")
		source := readTask2Artifact(t, parts...)
		if count := strings.Count(source, check.want); count != 1 {
			t.Errorf("%s contract count for %q = %d, want 1", check.path, check.want, count)
		}
	}
	viewmodels := readTask2Artifact(t, "cmd", "web", "partials", "portal_viewmodels.go")
	for _, state := range []string{"pending", "running", "stopping", "stopped", "shutting-down", "terminated"} {
		if strings.Count(viewmodels, `"portal-state-`+state+`"`) != 1 {
			t.Errorf("Portal known status %q is not bound exactly once", state)
		}
	}
}

func TestTask13Final10RoseTokenModelClearsRenderedCompositingFloor(t *testing.T) {
	rose := final10RGB{255, 127, 168}
	cocoa := final10RGB{46, 33, 48}
	night := final10RGB{23, 18, 27}
	surfaces := map[string]final10RGB{
		"error feedback Rose12 interior": final10Mix(rose, cocoa, 0.12),
		"modal Rose14 interior":          final10Mix(rose, cocoa, 0.14),
		"past table Night58 interior":    final10Mix(night, cocoa, 0.58),
	}
	for label, surface := range surfaces {
		renderedBorder := final10Mix(rose, surface, 0.72)
		if ratio := final10Contrast(renderedBorder, surface); ratio < 3 {
			t.Errorf("Rose72 %s modeled adjacent contrast = %.3f, want >= 3", label, ratio)
		}
	}
}

func TestTask13Final10BoundedMutationGate(t *testing.T) {
	canonical := readFinal10Graph(t)
	type mutation struct {
		name     string
		path     string
		old      string
		new      string
		append   string
		validate func(final10Graph) error
	}
	ordinary := validateFinal10OrdinaryContract
	focus := validateFinal10FocusContract
	forced := validateFinal10ForcedContract
	mutations := []mutation{
		{name: "Cocoa palette owner changes", path: "cmd/web/tailwind/theme.css", old: "--cocoa-cedar: #2E2130;", new: "--cocoa-cedar: #17121B;", validate: ordinary},
		{name: "strong boundary token weakens", path: "cmd/web/tailwind/theme.css", old: final10StrongBoundary, new: "color-mix(in srgb, var(--candle-oat) 30%, transparent)", validate: ordinary},
		{name: "line token is neutralized", path: "cmd/web/tailwind/shared.css", old: "--line-hairline: 0.0625rem;", new: "--line-hairline: 0;", validate: ordinary},
		{name: "error Rose line weakens", path: "cmd/web/tailwind/components.css", old: ".ui-feedback-error {\n  --feedback-signal: var(--rosehip);\n  border-color: " + final10RoseBoundary, new: ".ui-feedback-error {\n  --feedback-signal: var(--rosehip);\n  border-color: color-mix(in srgb, var(--rosehip) 62%, transparent)", validate: ordinary},
		{name: "modal Rose line weakens", path: "cmd/web/tailwind/soccer.css", old: "overscroll-behavior: contain;\n  border: var(--line-hairline) solid " + final10RoseBoundary, new: "overscroll-behavior: contain;\n  border: var(--line-hairline) solid color-mix(in srgb, var(--rosehip) 62%, transparent)", validate: ordinary},
		{name: "past table modifier weakens", path: "cmd/web/tailwind/soccer.css", old: ".games-table-section--past {\n  border-color: " + final10RoseBoundary, new: ".games-table-section--past {\n  border-color: color-mix(in srgb, var(--rosehip) 30%, transparent)", validate: ordinary},
		{name: "Skills data alias loses boundary", path: "cmd/web/tailwind/app.css", append: "\n[data-skills-search-input] { border: 0; }\n", validate: ordinary},
		{name: "secondary action alias uses border-none", path: "cmd/web/tailwind/app.css", append: "\n.ui-action-secondary { @apply border-none; }\n", validate: ordinary},
		{name: "Skills submit ID loses action boundary", path: "cmd/web/tailwind/app.css", append: "\n#skills-search-submit { border-color: transparent !important; }\n", validate: ordinary},
		{name: "Skills submit class loses action boundary", path: "cmd/web/tailwind/app.css", append: "\n.skills-search-submit { border-color: transparent !important; }\n", validate: ordinary},
		{name: "Portal control alias loses action boundary", path: "cmd/web/tailwind/app.css", append: "\n.portal-control { border-color: transparent !important; }\n", validate: ordinary},
		{name: "Portal action alias loses action boundary", path: "cmd/web/tailwind/app.css", append: "\n.portal-action-control { border-color: transparent !important; }\n", validate: ordinary},
		{name: "dominant button type loses action boundary", path: "cmd/web/tailwind/app.css", append: "\nbutton { border-color: transparent !important; }\n", validate: ordinary},
		{name: "dominant universal reset loses boundaries", path: "cmd/web/tailwind/app.css", append: "\n@layer reset { * { border: 0; } }\n", validate: ordinary},
		{name: "feedback base alias loses boundary", path: "cmd/web/tailwind/app.css", append: "\n.ui-feedback { border-color: transparent; }\n", validate: ordinary},
		{name: "feedback error alias uses arbitrary transparent", path: "cmd/web/tailwind/app.css", append: "\n.ui-feedback-error { @apply border-[transparent]; }\n", validate: ordinary},
		{name: "alert role alias hides boundary", path: "cmd/web/tailwind/app.css", append: "\n[role='alert'] { border-style: hidden; }\n", validate: ordinary},
		{name: "dialog role alias loses boundary", path: "cmd/web/tailwind/app.css", append: "\n[role='dialog'] { border: none; }\n", validate: ordinary},
		{name: "field cell child alias loses boundary", path: "cmd/web/tailwind/app.css", append: "\n.col-field > span { border-color: transparent; }\n", validate: ordinary},
		{name: "season cell child alias loses boundary", path: "cmd/web/tailwind/app.css", append: "\n.col-season > span { border-width: 0; }\n", validate: ordinary},
		{name: "past modifier is overridden", path: "cmd/web/tailwind/app.css", append: "\n.games-table-section--past { border-color: transparent; }\n", validate: ordinary},
		{name: "Portal overflow ID loses wide boundary", path: "cmd/web/tailwind/app.css", append: "\n@media (min-width: 90rem) { #portal-instance-overflow { border: 0; } }\n", validate: ordinary},
		{name: "one-level is alias loses boundary", path: "cmd/web/tailwind/app.css", append: "\n:is(.never, .ui-feedback-error) { border-color: transparent; }\n", validate: ordinary},
		{name: "unsupported protected has selector fails closed", path: "cmd/web/tailwind/app.css", append: "\n.ui-feedback-error:has(.ui-feedback-message) { border-color: transparent; }\n", validate: ordinary},
		{name: "ordinary branch in mixed media loses boundary", path: "cmd/web/tailwind/app.css", append: "\n@media (forced-colors: active), (min-width: 0rem) { .ui-feedback-error { border-color: transparent; } }\n", validate: ordinary},
		{name: "not forced colors branch loses boundary", path: "cmd/web/tailwind/app.css", append: "\n@media not (forced-colors: active) { .ui-feedback-error { border-color: transparent; } }\n", validate: ordinary},
		{name: "not all forced query is ordinary", path: "cmd/web/tailwind/app.css", append: "\n@media not all and (forced-colors: active) { .ui-feedback-error { border-color: transparent; } }\n", validate: ordinary},
		{name: "not screen forced query is ordinary", path: "cmd/web/tailwind/app.css", append: "\n@media not screen and (forced-colors: active) { .ui-feedback-error { border-color: transparent; } }\n", validate: ordinary},
		{name: "unterminated duplicate token overrides boundary", path: "cmd/web/tailwind/app.css", append: "\n:root { --color-border-strong: transparent }\n", validate: ordinary},
		{name: "focus token gets competing root owner", path: "cmd/web/tailwind/app.css", append: "\n:root { --color-focus-ring: transparent }\n", validate: ordinary},
		{name: "global focus paint is removed", path: "cmd/web/tailwind/app.css", append: "\n:focus-visible { outline: none; }\n", validate: focus},
		{name: "global focus paint is initialized away", path: "cmd/web/tailwind/app.css", append: "\n:focus-visible { outline: initial; }\n", validate: focus},
		{name: "Skills target gets nonpositive offset", path: "cmd/web/tailwind/app.css", append: "\n.filter-tabs:focus-visible { outline-offset: 0; }\n", validate: focus},
		{name: "skip focus is hidden", path: "cmd/web/tailwind/app.css", append: "\n.site-skip-link:focus-visible { display: none !important; }\n", validate: focus},
		{name: "skip focus returns under header", path: "cmd/web/tailwind/base.css", old: "inset-block-start: calc(var(--header-height) + var(--space-sm));", new: "inset-block-start: 0;", validate: focus},
		{name: "forced header keeps webkit filter", path: "cmd/web/tailwind/base.css", old: "-webkit-backdrop-filter: none !important;", new: "-webkit-backdrop-filter: blur(1rem) !important;", validate: forced},
		{name: "forced status keeps transparent text fill", path: "cmd/web/tailwind/portal.css", old: ".portal-state {\n    -webkit-text-fill-color: CanvasText !important;", new: ".portal-state {\n    -webkit-text-fill-color: transparent !important;", validate: forced},
		{name: "Both-mode status overrides forced text fill", path: "cmd/web/tailwind/app.css", append: "\n.portal-state { -webkit-text-fill-color: transparent !important; }\n", validate: forced},
		{name: "Both-mode status all reset defeats forced paint", path: "cmd/web/tailwind/app.css", append: "\n.portal-state { all: unset !important; }\n", validate: forced},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			mutated, changed := canonical.mutate(test.path, test.old, test.new, test.append)
			if !changed {
				t.Fatalf("mutation target was not found in %s", test.path)
			}
			if err := test.validate(mutated); err == nil {
				t.Fatal("bounded validator accepted protected mutation")
			}
		})
	}
}

func TestTask13Final10BoundedSelectorSafety(t *testing.T) {
	graph := readFinal10Graph(t)
	safe := `
.ui-feedback-error::before { border: 0; }
.ui-feedback-error:not(.ui-feedback-error) { border-color: transparent; }
.ui-feedback-error:not(.ui-feedback-error.ui-feedback-error) { border-color: transparent; }
.ui-feedback-error:not(:is(.ui-feedback-error, .never)) { border-color: transparent; }
.ui-feedback-error:not(:where(.never, .ui-feedback-error)) { border-color: transparent; }
.ui-feedback-error:first-line { border: 0; }
.ui-feedback-error:first-letter { border: 0; }
.site-skip-link::after { inset-block-start: 0; outline: none; }
@media (forced-colors: active) { .ui-feedback-error { border-color: transparent; } }
`
	graph, _ = graph.mutate("cmd/web/tailwind/app.css", "", "", safe)
	if err := validateFinal10OrdinaryContract(graph); err != nil {
		t.Fatalf("bounded selector safety fixture failed: %v", err)
	}
	if err := validateFinal10FocusContract(graph); err != nil {
		t.Fatalf("pseudo-element focus geometry fixture failed: %v", err)
	}
}

func TestTask13Final10LocalImportForms(t *testing.T) {
	css := `
@import url("./theme.css") layer(theme);
@import url('../tailwind/shared.css') layer(theme);
@import "tailwindcss/utilities.css";
`
	refs, err := final10LocalImportRefs(css)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"./theme.css", "../tailwind/shared.css"}
	if strings.Join(refs, "\n") != strings.Join(want, "\n") {
		t.Fatalf("local url imports = %v, want %v", refs, want)
	}
	root := filepath.Join("..", "..", "cmd", "web", "tailwind")
	if _, err := final10ResolveLocalImport(root, filepath.Join(root, "app.css"), "../tailwind/theme.css"); err != nil {
		t.Fatalf("resolve in-root parent import: %v", err)
	}
	if _, err := final10ResolveLocalImport(root, filepath.Join(root, "app.css"), "../../escape.css"); err == nil {
		t.Fatal("local import resolver accepted a path outside the Tailwind source root")
	}
}

func readFinal10Graph(t *testing.T) final10Graph {
	t.Helper()
	graph, err := loadFinal10Graph(filepath.Join("..", "..", "cmd", "web", "tailwind", "app.css"))
	if err != nil {
		t.Fatalf("load authored app.css graph: %v", err)
	}
	return graph
}

var final10ImportPattern = regexp.MustCompile(`(?i)@import\s+(?:url\(\s*)?(?:"([^"]+)"|'([^']+)'|([^\s);]+))\s*\)?[^;]*;`)

func final10LocalImportRefs(css string) ([]string, error) {
	css, err := task2StripCSSComments(css)
	if err != nil {
		return nil, err
	}
	var refs []string
	for _, match := range final10ImportPattern.FindAllStringSubmatch(css, -1) {
		ref := ""
		for _, candidate := range match[1:] {
			if candidate != "" {
				ref = candidate
				break
			}
		}
		if strings.HasPrefix(ref, "./") || strings.HasPrefix(ref, "../") {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}

func final10ResolveLocalImport(root, current, ref string) (string, error) {
	root = filepath.Clean(root)
	target := filepath.Clean(filepath.Join(filepath.Dir(current), filepath.FromSlash(ref)))
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("local CSS import %q escapes %s", ref, root)
	}
	return target, nil
}

func loadFinal10Graph(root string) (final10Graph, error) {
	var graph final10Graph
	tailwindRoot := filepath.Dir(filepath.Clean(root))
	seen := map[string]bool{}
	var visit func(string, bool) error
	visit = func(path string, isRoot bool) error {
		path = filepath.Clean(path)
		if seen[path] {
			return fmt.Errorf("duplicate or cyclic local CSS import %s", path)
		}
		seen[path] = true
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		css := string(contents)
		refs, importErr := final10LocalImportRefs(css)
		if importErr != nil {
			return importErr
		}
		for _, ref := range refs {
			if isRoot {
				graph.imports = append(graph.imports, ref)
			}
			resolved, resolveErr := final10ResolveLocalImport(tailwindRoot, path, ref)
			if resolveErr != nil {
				return resolveErr
			}
			if err := visit(resolved, false); err != nil {
				return err
			}
		}
		rel, err := filepath.Rel(filepath.Join("..", ".."), path)
		if err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("CSS import escapes workspace: %s", path)
		}
		graph.sources = append(graph.sources, final10Source{path: filepath.ToSlash(rel), css: css})
		return nil
	}
	if err := visit(root, true); err != nil {
		return final10Graph{}, err
	}
	return graph, nil
}

func (graph final10Graph) mutate(path, old, replacement, appendCSS string) (final10Graph, bool) {
	mutated := final10Graph{imports: append([]string(nil), graph.imports...), sources: append([]final10Source(nil), graph.sources...)}
	for index := range mutated.sources {
		if mutated.sources[index].path != path {
			continue
		}
		if appendCSS != "" {
			mutated.sources[index].css += appendCSS
			return mutated, true
		}
		if old != "" && strings.Count(mutated.sources[index].css, old) == 1 {
			mutated.sources[index].css = strings.Replace(mutated.sources[index].css, old, replacement, 1)
			return mutated, true
		}
		return graph, false
	}
	return graph, false
}

type final10Mode uint8

const (
	final10Ordinary final10Mode = 1 << iota
	final10Forced
	final10Both = final10Ordinary | final10Forced
)

type final10Rule struct {
	source           string
	selector         string
	body             string
	declarations     map[string]string
	minWidthRem      float64
	maxWidthRem      float64
	modes            final10Mode
	unsupportedMedia bool
	resetLayer       bool
}

type final10Context struct {
	minWidthRem      float64
	maxWidthRem      float64
	modes            final10Mode
	unsupportedMedia bool
	resetLayer       bool
}

type final10Effect struct {
	source      string
	selector    string
	minWidthRem float64
	modes       final10Mode
	property    string
	value       string
}

type final10Target struct {
	label      string
	aliases    []string
	types      []string
	guaranteed []string
	excluded   []string
	allowed    []final10Effect
}

var (
	final10MinWidthPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)min-width\s*:\s*([0-9]+(?:\.[0-9]+)?)rem`),
		regexp.MustCompile(`(?i)width\s*>=\s*([0-9]+(?:\.[0-9]+)?)rem`),
	}
	final10MaxWidthPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)max-width\s*:\s*([0-9]+(?:\.[0-9]+)?)rem`),
		regexp.MustCompile(`(?i)width\s*<\s*([0-9]+(?:\.[0-9]+)?)rem`),
	}
)

func validateFinal10OrdinaryContract(graph final10Graph) error {
	rules, err := collectFinal10Rules(graph)
	if err != nil {
		return err
	}
	if err := validateFinal10CoreTokens(graph); err != nil {
		return err
	}
	effects := final10BoundaryEffects()
	for effectIndex := range effects {
		effect := &effects[effectIndex]
		if count := final10EffectCount(rules, effect); count == 0 {
			return fmt.Errorf("%s %s has no canonical owner", effect.selector, effect.property)
		}
	}
	targets := []final10Target{
		{label: "Skills search", aliases: []string{"#skills-search", "[data-skills-search-input]"}, types: []string{"input"}, guaranteed: []string{"#skills-search", "[data-skills-search-input]"}, allowed: effects[0:1]},
		{label: "Soccer text field", aliases: []string{"#soccer-import-jwt", ".text-input"}, types: []string{"textarea"}, guaranteed: []string{"#soccer-import-jwt", ".text-input"}, excluded: []string{"[aria-invalid=true]", "[aria-invalid=false]"}, allowed: effects[1:3]},
		{label: "secondary action", aliases: []string{".page-kit-action", ".ui-action-secondary", ".portal-action-control", ".portal-control", "#skills-search-submit", ".skills-search-submit"}, types: []string{"button"}, guaranteed: []string{".page-kit-action", ".ui-action-secondary", ".portal-action-control", ".portal-control", "#skills-search-submit", ".skills-search-submit"}, excluded: []string{".page-kit-action-primary", ".ui-action-primary", ".ui-action-danger", ".ui-action-quiet", ".btn-primary"}, allowed: effects[3:5]},
		{label: "error feedback", aliases: []string{".ui-feedback", ".ui-feedback-error", "[role=alert]"}, types: []string{"div"}, guaranteed: []string{".ui-feedback", ".ui-feedback-error", "[role=alert]"}, allowed: effects[5:7]},
		{label: "Soccer modal", aliases: []string{".soccer-login-dialog", "[role=dialog]"}, types: []string{"div"}, guaranteed: []string{".soccer-login-dialog", "[role=dialog]"}, allowed: effects[7:8]},
		{label: "Soccer field badge", aliases: []string{".field-badge", ".col-field>span"}, types: []string{"span"}, guaranteed: []string{".field-badge", ".col-field>span"}, allowed: effects[8:9]},
		{label: "Soccer season badge", aliases: []string{".season-badge", ".col-season>span"}, types: []string{"span"}, guaranteed: []string{".season-badge", ".col-season>span"}, allowed: effects[9:11]},
		{label: "Soccer past table", aliases: []string{".games-table-section", ".games-table-section--past"}, types: []string{"div"}, guaranteed: []string{".games-table-section", ".games-table-section--past"}, allowed: effects[11:13]},
		{label: "Portal wide table", aliases: []string{"#portal-instance-overflow", ".portal-instance-table-shell .overflow-region"}, types: []string{"div"}, guaranteed: []string{"#portal-instance-overflow", ".portal-instance-table-shell .overflow-region"}, allowed: effects[13:14]},
	}
	for targetIndex := range targets {
		if err := validateFinal10BoundaryTarget(rules, &targets[targetIndex]); err != nil {
			return err
		}
	}
	return nil
}

func final10BoundaryEffects() []final10Effect {
	return []final10Effect{
		{"cmd/web/tailwind/pages/skills.css", "#skills-search", 0, final10Both, "border", "var(--line-hairline) solid var(--color-border-strong)"},
		{"cmd/web/tailwind/components.css", ".text-input", 0, final10Both, "border-color", "var(--color-border-strong)"},
		{"cmd/web/tailwind/components.css", ".text-input:focus-visible", 0, final10Both, "border-color", "var(--campfire-apricot)"},
		{"cmd/web/tailwind/components.css", ".page-kit-action", 0, final10Both, "border-color", final10StrongBoundary},
		{"cmd/web/tailwind/components.css", ".page-kit-action:hover", 0, final10Both, "border-color", "color-mix(in srgb, var(--pond-mint) 62%, transparent)"},
		{"cmd/web/tailwind/components.css", ".ui-feedback", 0, final10Both, "border", "var(--line-hairline) solid color-mix(in srgb, var(--feedback-signal) 46%, transparent)"},
		{"cmd/web/tailwind/components.css", ".ui-feedback-error", 0, final10Both, "border-color", final10RoseBoundary},
		{"cmd/web/tailwind/soccer.css", ".soccer-login-dialog", 0, final10Both, "border", "var(--line-hairline) solid " + final10RoseBoundary},
		{"cmd/web/tailwind/soccer.css", ".field-badge", 0, final10Both, "border", "var(--line-hairline) solid " + final10ApricotBorder},
		{"cmd/web/tailwind/soccer.css", ".season-badge", 0, final10Both, "border", "var(--line-hairline) solid " + final10ApricotBorder},
		{"cmd/web/tailwind/soccer.css", ".season-badge", 0, final10Both, "border-color", final10MintBorder},
		{"cmd/web/tailwind/soccer.css", ".games-table-section", 0, final10Both, "border", "var(--line-hairline) solid " + final10MintBorder},
		{"cmd/web/tailwind/soccer.css", ".games-table-section--past", 0, final10Both, "border-color", final10RoseBoundary},
		{"cmd/web/tailwind/portal.css", ".portal-instance-table-shell .overflow-region", 48, final10Both, "border", "var(--line-hairline) solid var(--color-border-strong)"},
	}
}

func validateFinal10CoreTokens(graph final10Graph) error {
	type owner struct{ source, value string }
	expected := map[string][]owner{
		"--night-mulberry":       {{"cmd/web/tailwind/theme.css", "#17121B"}},
		"--cocoa-cedar":          {{"cmd/web/tailwind/theme.css", "#2E2130"}},
		"--candle-oat":           {{"cmd/web/tailwind/theme.css", "#FFF0D8"}},
		"--campfire-apricot":     {{"cmd/web/tailwind/theme.css", "#FFA677"}},
		"--rosehip":              {{"cmd/web/tailwind/theme.css", "#FF7FA8"}},
		"--pond-mint":            {{"cmd/web/tailwind/theme.css", "#78E3C3"}},
		"--color-border-strong":  {{"cmd/web/tailwind/theme.css", final10StrongBoundary}},
		"--color-success-border": {{"cmd/web/tailwind/theme.css", final10MintBorder}},
		"--color-danger-border":  {{"cmd/web/tailwind/theme.css", "color-mix(in srgb, var(--rosehip) 52%, transparent)"}},
		"--color-focus-ring": {
			{"cmd/web/tailwind/theme.css", "var(--candle-oat)"},
			{"cmd/web/tailwind/base.css", "Highlight"},
			{"cmd/web/tailwind/components.css", "var(--page-kit-accent)"},
		},
		"--line-hairline": {{"cmd/web/tailwind/shared.css", "0.0625rem"}},
		"--line-accent":   {{"cmd/web/tailwind/shared.css", "0.125rem"}},
		"--line-signal":   {{"cmd/web/tailwind/shared.css", "0.1875rem"}},
		"--line-focus":    {{"cmd/web/tailwind/shared.css", "0.25rem"}},
	}
	for token, owners := range expected {
		pattern := regexp.MustCompile(`(?m)(?:^|[;{])\s*` + regexp.QuoteMeta(token) + `\s*:\s*([^;{}]+?)\s*(?:;|})`)
		count := 0
		for _, source := range graph.sources {
			css, err := task2StripCSSComments(source.css)
			if err != nil {
				return err
			}
			for _, match := range pattern.FindAllStringSubmatch(css, -1) {
				count++
				approved := false
				for _, want := range owners {
					approved = approved || source.path == want.source && task2CSSValueEqual(match[1], want.value)
				}
				if !approved {
					return fmt.Errorf("core token %s has competing owner/value %s:%s", token, source.path, match[1])
				}
			}
		}
		if count != len(owners) {
			return fmt.Errorf("core token %s owner count = %d, want %d", token, count, len(owners))
		}
	}
	return nil
}

func validateFinal10BoundaryTarget(rules []final10Rule, target *final10Target) error {
	for ruleIndex := range rules {
		rule := &rules[ruleIndex]
		if rule.modes&final10Ordinary == 0 {
			continue
		}
		matched, err := final10RuleTargets(rule.selector, target)
		if err != nil {
			return fmt.Errorf("%s: %w", target.label, err)
		}
		if !matched && !final10DominantBoundarySubject(rule, target) {
			continue
		}
		if rule.unsupportedMedia {
			return fmt.Errorf("%s uses unsupported media for protected selector %q", target.label, rule.selector)
		}
		if utility := final10UnsafeApply(rule.body, "border"); utility != "" {
			return fmt.Errorf("%s has unapproved @apply %s in %q", target.label, utility, rule.selector)
		}
		for property, value := range rule.declarations {
			if !final10BoundaryProperty(property) {
				continue
			}
			allowed := false
			for effectIndex := range target.allowed {
				if final10EffectMatches(rule, &target.allowed[effectIndex], property, value) {
					allowed = true
					break
				}
			}
			if !allowed {
				return fmt.Errorf("%s has competing %s:%s in %s %q", target.label, property, value, rule.source, rule.selector)
			}
		}
	}
	return nil
}

// Plain type and universal selectors participate only when their protected
// declaration can outrank the authored component rule without pretending to
// be a full specificity engine. The contract recognizes the two authored
// dominance mechanisms it permits: !important and an explicit reset layer.
func final10DominantBoundarySubject(rule *final10Rule, target *final10Target) bool {
	dominant := rule.resetLayer
	for property, value := range rule.declarations {
		if final10BoundaryProperty(property) && strings.Contains(strings.ToLower(value), "!important") {
			dominant = true
		}
	}
	if !dominant {
		return false
	}
	for _, branch := range task2SplitTopLevel(rule.selector, ',') {
		branch = strings.ToLower(strings.TrimSpace(branch))
		if branch == "*" {
			return true
		}
		for _, element := range target.types {
			if branch == element {
				return true
			}
		}
	}
	return false
}

func validateFinal10FocusContract(graph final10Graph) error {
	rules, err := collectFinal10Rules(graph)
	if err != nil {
		return err
	}
	required := []final10Effect{
		{"cmd/web/tailwind/base.css", ":focus-visible", 0, final10Both, "outline", "2px solid var(--color-focus-ring)"},
		{"cmd/web/tailwind/base.css", ":focus-visible", 0, final10Both, "outline-offset", "2px"},
		{"cmd/web/tailwind/base.css", ":where(:focus-visible, .focus-ring:focus-visible)", 0, final10Forced, "outline", "0.2rem solid Highlight !important"},
		{"cmd/web/tailwind/base.css", ":where(:focus-visible, .focus-ring:focus-visible)", 0, final10Forced, "outline-offset", "0.2rem !important"},
	}
	for effectIndex := range required {
		effect := &required[effectIndex]
		if final10EffectCount(rules, effect) != 1 {
			return fmt.Errorf("focus owner missing: %s %s", effect.selector, effect.property)
		}
	}
	focusTargets := []final10Target{
		{label: "skip focus", aliases: []string{".site-skip-link"}, guaranteed: []string{".site-skip-link", ":focus-visible"}},
		{label: "Skills scroller focus", aliases: []string{".filter-tabs"}, guaranteed: []string{".filter-tabs", ":focus-visible"}},
		{label: "Skills tab focus", aliases: []string{".skills-filter-tab"}, guaranteed: []string{".skills-filter-tab", ":focus-visible"}},
	}
	for _, rule := range rules {
		protected := final10GlobalFocusSelector(rule.selector)
		for targetIndex := range focusTargets {
			matched, matchErr := final10RuleTargets(rule.selector, &focusTargets[targetIndex])
			if matchErr != nil {
				return matchErr
			}
			protected = protected || matched
		}
		if !protected {
			continue
		}
		if utility := final10UnsafeApply(rule.body, "focus"); utility != "" {
			return fmt.Errorf("focus target has unapproved @apply %s in %q", utility, rule.selector)
		}
		for property, value := range rule.declarations {
			if final10UnsafeFocusValue(property, value) {
				return fmt.Errorf("focus target has unsafe %s:%s in %q", property, value, rule.selector)
			}
		}
	}
	skip := final10Target{label: "skip focus", aliases: []string{".site-skip-link"}, guaranteed: []string{".site-skip-link", ":focus-visible"}}
	skipEffects := []final10Effect{
		{"cmd/web/tailwind/base.css", ".site-skip-link:focus-visible", 0, final10Both, "display", "inline-flex"},
		{"cmd/web/tailwind/base.css", ".site-skip-link:focus-visible", 0, final10Both, "position", "fixed"},
		{"cmd/web/tailwind/base.css", ".site-skip-link:focus-visible", 0, final10Both, "inset", "auto"},
		{"cmd/web/tailwind/base.css", ".site-skip-link:focus-visible", 0, final10Both, "inset-block-start", "calc(var(--header-height) + var(--space-sm))"},
		{"cmd/web/tailwind/base.css", ".site-skip-link:focus-visible", 0, final10Both, "inset-inline-start", "var(--space-sm)"},
		{"cmd/web/tailwind/base.css", ".site-skip-link:focus-visible", 0, final10Both, "width", "auto"},
		{"cmd/web/tailwind/base.css", ".site-skip-link:focus-visible", 0, final10Both, "height", "auto"},
		{"cmd/web/tailwind/base.css", ".site-skip-link:focus-visible", 0, final10Both, "clip", "auto"},
		{"cmd/web/tailwind/base.css", ".site-skip-link:focus-visible", 0, final10Both, "overflow", "visible"},
		{"cmd/web/tailwind/base.css", ".site-skip-link:focus-visible", 0, final10Both, "clip-path", "none"},
	}
	for effectIndex := range skipEffects {
		effect := &skipEffects[effectIndex]
		if final10EffectCount(rules, effect) != 1 {
			return fmt.Errorf("skip-link focus clearance owner missing for %s", effect.property)
		}
	}
	for _, rule := range rules {
		if rule.modes&final10Ordinary == 0 {
			continue
		}
		matched, matchErr := final10RuleTargets(rule.selector, &skip)
		if matchErr != nil || !matched {
			if matchErr != nil {
				return matchErr
			}
			continue
		}
		for property, value := range rule.declarations {
			geometry := property == "display" || property == "visibility" || property == "opacity" || property == "content-visibility" || property == "position" || property == "inset" || strings.HasPrefix(property, "inset-") || property == "top" || property == "right" || property == "bottom" || property == "left" || property == "width" || property == "height" || property == "overflow" || property == "clip" || property == "clip-path" || property == "transform" || property == "translate" || property == "margin" || strings.HasPrefix(property, "margin-")
			if !geometry {
				continue
			}
			allowed := false
			for effectIndex := range skipEffects {
				allowed = allowed || final10EffectMatches(&rule, &skipEffects[effectIndex], property, value)
			}
			if !allowed {
				return fmt.Errorf("skip-link focus has competing %s:%s in %q", property, value, rule.selector)
			}
		}
	}
	return nil
}

func validateFinal10ForcedContract(graph final10Graph) error {
	rules, err := collectFinal10Rules(graph)
	if err != nil {
		return err
	}
	required := []final10Effect{
		{"cmd/web/tailwind/base.css", ".site-chrome-header .header-content", 0, final10Forced, "-webkit-backdrop-filter", "none !important"},
		{"cmd/web/tailwind/base.css", ".site-chrome-header .header-content", 0, final10Forced, "backdrop-filter", "none !important"},
		{"cmd/web/tailwind/portal.css", ".portal-state", 0, final10Forced, "-webkit-text-fill-color", "CanvasText !important"},
	}
	for effectIndex := range required {
		effect := &required[effectIndex]
		if final10EffectCount(rules, effect) != 1 {
			return fmt.Errorf("forced-color owner missing: %s %s", effect.selector, effect.property)
		}
	}
	targets := []final10Target{
		{label: "forced header", aliases: []string{".site-chrome-header .header-content", ".header-content"}, guaranteed: []string{".site-chrome-header .header-content"}, allowed: required[0:2]},
		{label: "forced Portal status", aliases: []string{".portal-state", ".portal-state-pending", ".portal-state-running", ".portal-state-stopping", ".portal-state-stopped", ".portal-state-shutting-down", ".portal-state-terminated"}, guaranteed: []string{".portal-state"}, allowed: required[2:3]},
	}
	for targetIndex := range targets {
		target := &targets[targetIndex]
		for _, rule := range rules {
			if rule.modes&final10Forced == 0 {
				continue
			}
			matched, matchErr := final10RuleTargets(rule.selector, target)
			if matchErr != nil {
				return matchErr
			}
			if !matched {
				continue
			}
			if utility := final10UnsafeApply(rule.body, "forced"); utility != "" {
				return fmt.Errorf("%s has unapproved @apply %s in %q", target.label, utility, rule.selector)
			}
			for property, value := range rule.declarations {
				protected := property == "all" || target.label == "forced header" && (property == "backdrop-filter" || property == "-webkit-backdrop-filter") || target.label == "forced Portal status" && property == "-webkit-text-fill-color"
				if !protected {
					continue
				}
				if rule.modes == final10Both && !rule.resetLayer && !strings.Contains(strings.ToLower(value), "!important") {
					continue
				}
				allowed := false
				for effectIndex := range target.allowed {
					allowed = allowed || final10EffectMatches(&rule, &target.allowed[effectIndex], property, value)
				}
				if !allowed {
					return fmt.Errorf("%s has competing %s:%s in %q", target.label, property, value, rule.selector)
				}
			}
		}
	}
	return nil
}

func collectFinal10Rules(graph final10Graph) ([]final10Rule, error) {
	var rules []final10Rule
	for _, source := range graph.sources {
		collected, err := collectFinal10RulesInContext(source, source.css, final10Context{maxWidthRem: math.Inf(1), modes: final10Both})
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", source.path, err)
		}
		rules = append(rules, collected...)
	}
	return rules, nil
}

func collectFinal10RulesInContext(source final10Source, css string, context final10Context) ([]final10Rule, error) {
	blocks, err := parseTask2CSSBlocks(css)
	if err != nil {
		return nil, err
	}
	var rules []final10Rule
	for _, block := range blocks {
		header := strings.TrimSpace(block.header)
		if strings.HasPrefix(header, "@") {
			contexts := []final10Context{context}
			if strings.HasPrefix(strings.ToLower(header), "@media") {
				contexts = final10MediaContexts(header, context)
			} else if final10ResetLayer(header) {
				contexts[0].resetLayer = true
			}
			for _, nestedContext := range contexts {
				nested, nestedErr := collectFinal10RulesInContext(source, block.body, nestedContext)
				if nestedErr != nil {
					return nil, nestedErr
				}
				rules = append(rules, nested...)
			}
			continue
		}
		rules = append(rules, final10Rule{source: source.path, selector: header, body: block.body, declarations: task2Declarations(block.body), minWidthRem: context.minWidthRem, maxWidthRem: context.maxWidthRem, modes: context.modes, unsupportedMedia: context.unsupportedMedia, resetLayer: context.resetLayer})
	}
	return rules, nil
}

func final10ResetLayer(header string) bool {
	fields := strings.Fields(strings.ToLower(task2CanonicalCSS(header)))
	return len(fields) >= 2 && fields[0] == "@layer" && strings.TrimSuffix(fields[1], ";") == "reset"
}

func final10MediaContexts(header string, inherited final10Context) []final10Context {
	query := strings.TrimSpace(header[len("@media"):])
	branches := task2SplitTopLevel(query, ',')
	contexts := make([]final10Context, 0, len(branches))
	for _, branch := range branches {
		context := inherited
		lower := strings.ToLower(task2CanonicalCSS(branch))
		compact := strings.ReplaceAll(lower, " ", "")
		if strings.Contains(compact, "forced-colors") {
			switch {
			case strings.HasPrefix(compact, "notalland("), strings.HasPrefix(compact, "notscreenand("), strings.Contains(compact, "not(forced-colors:active)"), strings.Contains(compact, "forced-colors:none"):
				context.modes &= final10Ordinary
			case strings.Contains(compact, "forced-colors:active") && !regexp.MustCompile(`(?i)(^|[^-a-z])or([^-a-z]|$)`).MatchString(lower):
				context.modes &= final10Forced
			default:
				context.unsupportedMedia = true
			}
		}
		for _, pattern := range final10MinWidthPatterns {
			if match := pattern.FindStringSubmatch(branch); match != nil {
				value, _ := strconv.ParseFloat(match[1], 64)
				context.minWidthRem = math.Max(context.minWidthRem, value)
			}
		}
		for _, pattern := range final10MaxWidthPatterns {
			if match := pattern.FindStringSubmatch(branch); match != nil {
				value, _ := strconv.ParseFloat(match[1], 64)
				context.maxWidthRem = math.Min(context.maxWidthRem, value)
			}
		}
		contexts = append(contexts, context)
	}
	return contexts
}

func final10EffectCount(rules []final10Rule, effect *final10Effect) int {
	count := 0
	for ruleIndex := range rules {
		rule := &rules[ruleIndex]
		if value, ok := rule.declarations[effect.property]; ok && final10EffectMatches(rule, effect, effect.property, value) {
			count++
		}
	}
	return count
}

func final10EffectMatches(rule *final10Rule, effect *final10Effect, property, value string) bool {
	selectorMatches := task2SelectorListContains(rule.selector, effect.selector) || task2CanonicalCSS(rule.selector) == task2CanonicalCSS(effect.selector)
	return rule.source == effect.source && selectorMatches && math.Abs(rule.minWidthRem-effect.minWidthRem) < 0.0001 && rule.modes == effect.modes && property == effect.property && task2CSSValueEqual(value, effect.value)
}

func final10BoundaryProperty(property string) bool {
	if property == "all" || property == "border" {
		return true
	}
	return strings.HasPrefix(property, "border-") && !strings.HasPrefix(property, "border-radius")
}

var final10ApplyPattern = regexp.MustCompile(`(?i)@apply\s+([^;]+);`)

func final10UnsafeApply(body, policy string) string {
	for _, match := range final10ApplyPattern.FindAllStringSubmatch(body, -1) {
		for _, utility := range strings.Fields(match[1]) {
			base := final10UtilityBase(utility)
			lower := strings.ToLower(base)
			if lower == "all" || strings.HasPrefix(lower, "all-") {
				return utility
			}
			switch policy {
			case "border":
				approved := lower == "border" || lower == "border-accent-500" || lower == "border-danger-500"
				if !approved && strings.Contains(lower, "border") {
					return utility
				}
			case "focus":
				if (strings.Contains(lower, "outline") || strings.Contains(lower, "ring")) && (strings.Contains(lower, "none") || strings.Contains(lower, "hidden") || strings.Contains(lower, "transparent") || strings.HasSuffix(lower, "-0") || strings.Contains(lower, ":0")) {
					return utility
				}
			case "skills":
				if lower == "hidden" || lower == "invisible" || lower == "opacity-0" || strings.Contains(lower, "padding") || strings.Contains(lower, "overflow") || strings.Contains(lower, "translate") || regexp.MustCompile(`^[pm][xytrblse]?-`).MatchString(lower) {
					return utility
				}
			case "forced":
				if lower == "all" || strings.HasPrefix(lower, "all-") || strings.Contains(lower, "backdrop-filter") || strings.Contains(lower, "text-fill") {
					return utility
				}
			}
		}
	}
	return ""
}

func final10UtilityBase(utility string) string {
	utility = strings.Trim(strings.TrimSpace(utility), "!")
	depth := 0
	lastColon := -1
	for index, value := range utility {
		switch value {
		case '[', '(':
			depth++
		case ']', ')':
			depth--
		case ':':
			if depth == 0 {
				lastColon = index
			}
		}
	}
	if lastColon >= 0 {
		utility = utility[lastColon+1:]
	}
	return strings.Trim(utility, "!")
}

func final10GlobalFocusSelector(selector string) bool {
	branches, err := final10ExpandSelectorList(selector)
	if err != nil {
		return false
	}
	for _, branch := range branches {
		positive, negatives, stripErr := final10StripNot(branch)
		if stripErr == nil && len(negatives) == 0 {
			canonical := task2CanonicalCSS(positive)
			if canonical == ":focus-visible" || canonical == ".focus-ring:focus-visible" {
				return true
			}
		}
	}
	return false
}

func final10UnsafeFocusValue(property, value string) bool {
	lower := strings.ToLower(strings.ReplaceAll(value, "!important", ""))
	lower = strings.TrimSpace(lower)
	if property == "all" {
		return true
	}
	if property == "outline-offset" {
		match := regexp.MustCompile(`^([+-]?(?:[0-9]+(?:\.[0-9]+)?|\.[0-9]+))(?:px|rem|em)?$`).FindStringSubmatch(lower)
		if match != nil {
			amount, _ := strconv.ParseFloat(match[1], 64)
			return amount <= 0
		}
		return true
	}
	if property == "outline" || strings.HasPrefix(property, "outline-") {
		zeroWidth := regexp.MustCompile(`(^|\s)0(?:px|rem|em)?($|\s)`).MatchString(lower)
		reset := lower == "initial" || lower == "unset" || lower == "inherit" || lower == "revert" || lower == "revert-layer"
		return reset || zeroWidth || strings.Contains(lower, "none") || strings.Contains(lower, "hidden") || strings.Contains(lower, "transparent")
	}
	return false
}

type final10RGB struct{ red, green, blue float64 }

func final10Mix(foreground, background final10RGB, amount float64) final10RGB {
	return final10RGB{
		red:   foreground.red*amount + background.red*(1-amount),
		green: foreground.green*amount + background.green*(1-amount),
		blue:  foreground.blue*amount + background.blue*(1-amount),
	}
}

func final10Contrast(first, second final10RGB) float64 {
	firstLuminance := final10Luminance(first)
	secondLuminance := final10Luminance(second)
	return (math.Max(firstLuminance, secondLuminance) + 0.05) / (math.Min(firstLuminance, secondLuminance) + 0.05)
}

func final10Luminance(color final10RGB) float64 {
	linear := func(channel float64) float64 {
		channel /= 255
		if channel <= 0.04045 {
			return channel / 12.92
		}
		return math.Pow((channel+0.055)/1.055, 2.4)
	}
	return 0.2126*linear(color.red) + 0.7152*linear(color.green) + 0.0722*linear(color.blue)
}
