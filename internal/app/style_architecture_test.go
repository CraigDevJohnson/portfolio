package app

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var styleExpectedDirectives = []string{
	`@layer theme, base, components, pages, utilities;`,
	`@import "tailwindcss/theme.css" layer(theme);`,
	`@source "../layouts";`,
	`@source "../pages";`,
	`@source "../partials";`,
	`@source "../static/js";`,
	`@import "./theme.css" layer(theme);`,
	`@import "./shared.css" layer(theme);`,
	`@import "./base.css" layer(base);`,
	`@import "./components.css" layer(components);`,
	`@import "./pages/home.css" layer(pages);`,
	`@import "./pages/about.css" layer(pages);`,
	`@import "./pages/experience.css" layer(pages);`,
	`@import "./pages/skills.css" layer(pages);`,
	`@import "./pages/projects.css" layer(pages);`,
	`@import "./pages/education.css" layer(pages);`,
	`@import "./pages/contact.css" layer(pages);`,
	`@import "./soccer.css" layer(pages);`,
	`@import "./portal.css" layer(pages);`,
	`@import "tailwindcss/utilities.css" layer(utilities) source(none);`,
}

var styleProjectSources = []string{
	"theme.css",
	"shared.css",
	"base.css",
	"components.css",
	"pages/home.css",
	"pages/about.css",
	"pages/experience.css",
	"pages/skills.css",
	"pages/projects.css",
	"pages/education.css",
	"pages/contact.css",
	"soccer.css",
	"portal.css",
}

var styleMigrationSources = []string{
	"emberglass.css",
	"emberglass-responsive.css",
	"emberglass-accessibility.css",
}

var styleRouteOwners = []struct {
	families []string
	owner    string
	sentinel string
}{
	{[]string{"home"}, "pages/home.css", ".home-page"},
	{[]string{"about"}, "pages/about.css", ".about-page"},
	{[]string{"experience"}, "pages/experience.css", ".experience-page"},
	{[]string{"skill", "skills"}, "pages/skills.css", ".skills-page"},
	{[]string{"project", "projects"}, "pages/projects.css", ".projects-page-showcase"},
	{[]string{"education"}, "pages/education.css", ".education-page"},
	{[]string{"contact"}, "pages/contact.css", ".contact-page"},
	{[]string{"soccer"}, "soccer.css", ".soccer-page"},
	{[]string{"portal"}, "portal.css", ".portal-page"},
}

func TestTailwindSourceArchitecture(t *testing.T) {
	root := styleRepositoryRoot(t)
	tailwindRoot := filepath.Join(root, "cmd", "web", "tailwind")

	t.Run("complete directive graph", func(t *testing.T) {
		appCSS := styleReadFile(t, filepath.Join(tailwindRoot, "app.css"))
		if err := validateStyleDirectiveGraph(appCSS); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("migration sheets are absent", func(t *testing.T) {
		if err := validateStyleMigrationPaths(tailwindRoot); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("app owns every project layer", func(t *testing.T) {
		for _, name := range styleProjectSources {
			css := styleReadFile(t, filepath.Join(tailwindRoot, filepath.FromSlash(name)))
			if err := validateNoOuterLayer(css); err != nil {
				t.Errorf("%s: %v", name, err)
			}
		}
	})

	t.Run("closed breakpoint namespace", func(t *testing.T) {
		if err := validateStyleBreakpoints(styleReadFile(t, filepath.Join(tailwindRoot, "theme.css"))); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("canonical source media and utility vocabulary", func(t *testing.T) {
		for path, source := range styleAuthoritativeUISources(t, root) {
			if strings.HasSuffix(path, ".css") {
				if err := validateStyleMediaWidths(source); err != nil {
					t.Errorf("%s: %v", path, err)
				}
			}
			if err := validateStyleResponsiveVariants(source); err != nil {
				t.Errorf("%s: %v", path, err)
			}
		}
	})

	t.Run("sole JavaScript width contract", func(t *testing.T) {
		js := styleReadFile(t, filepath.Join(root, "cmd", "web", "static", "js", "main.js"))
		if err := validateStyleJavaScriptWidths(js); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("six-color core", func(t *testing.T) {
		for path, source := range styleAuthoritativeUISources(t, root) {
			if err := validateStyleForbiddenColor(source); err != nil {
				t.Errorf("%s: %v", path, err)
			}
		}
	})

	t.Run("one universal reduced-motion reset", func(t *testing.T) {
		if err := validateStyleReducedMotion(styleReadFile(t, filepath.Join(tailwindRoot, "base.css"))); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("one route owner per selector family", func(t *testing.T) {
		if err := validateStyleRouteOwners(styleTailwindCSSSources(t, tailwindRoot)); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("stable route sentinels", func(t *testing.T) {
		if err := validateStyleRouteSentinels(styleTailwindCSSSources(t, tailwindRoot)); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("legacy hero aliases are absent", func(t *testing.T) {
		for path, source := range styleAuthoritativeUISources(t, root) {
			if err := validateStyleLegacyHeroAliases(source); err != nil {
				t.Errorf("%s: %v", path, err)
			}
		}
	})

	t.Run("generic forced-color trail remains structural", func(t *testing.T) {
		if err := validateStyleForcedColorTrail(styleReadFile(t, filepath.Join(tailwindRoot, "base.css"))); err != nil {
			t.Fatal(err)
		}
	})

	if os.Getenv("VERIFY_COMPILED_CSS") == "1" {
		t.Run("compiled media widths remain canonical", func(t *testing.T) {
			compiled := styleReadFile(t, filepath.Join(root, "cmd", "web", "static", "css", "tailwind.css"))
			if err := validateStyleMediaWidths(compiled); err != nil {
				t.Fatal(err)
			}
		})
		t.Run("explicit source closure blocks repository discovery", func(t *testing.T) {
			if err := validateStyleExplicitSourceClosure(t, root); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestStyleArchitectureMutationShields(t *testing.T) {
	validGraph := strings.Join(styleExpectedDirectives, "\n")
	directiveMutations := []struct {
		name string
		old  string
		new  string
	}{
		{"adjacent imports swapped", styleExpectedDirectives[6] + "\n" + styleExpectedDirectives[7], styleExpectedDirectives[7] + "\n" + styleExpectedDirectives[6]},
		{"import removed", styleExpectedDirectives[10] + "\n", ""},
		{"import duplicated", styleExpectedDirectives[11], styleExpectedDirectives[11] + "\n" + styleExpectedDirectives[11]},
		{"import added", styleExpectedDirectives[18], styleExpectedDirectives[18] + "\n" + `@import "./extra.css" layer(pages);`},
		{"source removed", styleExpectedDirectives[3] + "\n", ""},
		{"source reordered", styleExpectedDirectives[2] + "\n" + styleExpectedDirectives[3], styleExpectedDirectives[3] + "\n" + styleExpectedDirectives[2]},
		{"source added", styleExpectedDirectives[5], styleExpectedDirectives[5] + "\n" + `@source "../wrong";`},
		{"source closure removed", " source(none)", ""},
		{"layer declaration altered", styleExpectedDirectives[0], `@layer theme, components, base, pages, utilities;`},
	}
	for _, mutation := range directiveMutations {
		t.Run(mutation.name, func(t *testing.T) {
			mutated := strings.Replace(validGraph, mutation.old, mutation.new, 1)
			if mutated == validGraph {
				t.Fatalf("mutation target %q not found", mutation.old)
			}
			if err := validateStyleDirectiveGraph(mutated); err == nil {
				t.Fatal("directive mutation unexpectedly passed")
			}
		})
	}

	t.Run("outer layer wrapper", func(t *testing.T) {
		for _, source := range []string{
			`@layer components { .surface-card { display: block; } }`,
			`@source inline("lg:w-[17.25rem]");`,
			`@import "tailwindcss/utilities.css" layer(utilities);`,
			`@charset "UTF-8"; @layer components { .surface-card { display: block; } }`,
			`@charset "UTF-8"; @source inline("lg:w-[17.25rem]");`,
			`@charset "UTF-8"; @import "tailwindcss/utilities.css" layer(utilities);`,
		} {
			if err := validateNoOuterLayer(source); err == nil {
				t.Errorf("project directive mutation unexpectedly passed: %s", source)
			}
		}
		for _, source := range []string{
			`.fixture::before { content: "@source inline('lg:grid')"; }`,
			`.fixture::before { content: "@layer pages"; }`,
			`/* @source inline("lg:grid"); */ .fixture { display: block; }`,
		} {
			if err := validateNoOuterLayer(source); err != nil {
				t.Errorf("harmless directive text %q rejected: %v", source, err)
			}
		}
	})

	t.Run("migration sheet recreation", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, styleMigrationSources[0])
		if err := os.WriteFile(path, []byte("/* recreated */"), 0o600); err != nil {
			t.Fatalf("create migration mutation: %v", err)
		}
		if err := validateStyleMigrationPaths(root); err == nil {
			t.Fatal("recreated migration stylesheet unexpectedly passed")
		}
	})

	t.Run("breakpoint mutations", func(t *testing.T) {
		valid := `@theme { --breakpoint-*: initial; --breakpoint-sm: 30rem; --breakpoint-md: 48rem; --breakpoint-lg: 70rem; --breakpoint-xl: 80rem; }`
		for _, replacement := range []string{"40rem", "64rem", "68rem", "72rem", "1024px"} {
			mutated := strings.Replace(valid, "70rem", replacement, 1)
			if mutated == valid {
				t.Fatalf("breakpoint target not found for %s", replacement)
			}
			if err := validateStyleBreakpoints(mutated); err == nil {
				t.Errorf("breakpoint mutation %s unexpectedly passed", replacement)
			}
		}
	})

	t.Run("responsive class mutations", func(t *testing.T) {
		for _, className := range []string{"xs:grid", "2xl:grid", "`2xl:grid`", "min-[41rem]:grid", "max-[63rem]:grid"} {
			if err := validateStyleResponsiveVariants(`<div class="` + className + `"></div>`); err == nil {
				t.Errorf("responsive mutation %q unexpectedly passed", className)
			}
		}
		for _, allowed := range []string{"max-sm:grid", "max-md:grid", "sm:max-md:grid", "max-w-[42rem]", "[overflow-wrap:anywhere]", "--space-xs: 1rem;"} {
			if err := validateStyleResponsiveVariants(allowed); err != nil {
				t.Errorf("allowed source %q rejected: %v", allowed, err)
			}
		}
	})

	t.Run("raw media mutations", func(t *testing.T) {
		for _, query := range []string{"(min-width: 40rem)", "(min-width: 64rem)", "(max-width: 68rem)", "(width >= 72rem)", "(min-width: 768px)", "(40rem <= width)", "(40rem <= width < 70rem)", "(min-width: calc(70rem))", "(width = 70rem)", "(width: 70rem)", "(min-device-width: 70rem)", "(min-width: 80rem) and (max-width: 30rem)", "(min-width: 30rem), (max-width: 80rem)"} {
			if err := validateStyleMediaWidths(`@media ` + query + ` { .fixture { display: grid; } }`); err == nil {
				t.Errorf("media mutation %q unexpectedly passed", query)
			}
		}
		for _, query := range []string{"(width < 30rem)", "(width < 48rem)", "(width < 70rem)", "(width >= 30rem)", "(width >= 48rem)", "(width >= 70rem)", "(width >= 80rem)"} {
			if err := validateStyleMediaWidths(`@media ` + query + ` { .fixture { display: grid; } }`); err != nil {
				t.Errorf("canonical range query %q rejected: %v", query, err)
			}
		}
		if err := validateStyleMediaWidths(`@media not all and (min-width: 30rem) { .fixture { display: grid; } }`); err == nil {
			t.Error("negated width media mutation unexpectedly passed")
		}
	})

	t.Run("JavaScript width mutations", func(t *testing.T) {
		valid := `window.matchMedia('(prefers-reduced-motion: reduce)'); window.matchMedia('(max-width: 69.999rem)');`
		for _, mutated := range []string{
			strings.Replace(valid, "69.999rem", "68rem", 1),
			valid + ` window.matchMedia('(min-width: 48rem)');`,
			valid + ` window.matchMedia ('(min-width: 48rem)');`,
			valid + ` window["matchMedia"]('(min-width: 48rem)');`,
			valid + " window.matchMedia(`(min-width: 48rem)`);",
			valid + ` window.matchMedia(widthQuery);`,
			valid + ` window["match" + "Media"]('(min-width: 40rem)');`,
		} {
			if err := validateStyleJavaScriptWidths(mutated); err == nil {
				t.Errorf("JavaScript width mutation unexpectedly passed: %s", mutated)
			}
		}
		for _, harmless := range []string{`// matchMedia('(min-width: 40rem)')`, `const label = "matchMedia('(min-width: 40rem)')"`} {
			if err := validateStyleJavaScriptWidths(valid + "\n" + harmless); err != nil {
				t.Errorf("harmless matchMedia text rejected: %v", err)
			}
		}
	})

	t.Run("forbidden color mutations", func(t *testing.T) {
		for _, source := range []string{"--pollen-gold: orange;", "color: #fFd166;"} {
			if err := validateStyleForbiddenColor(source); err == nil {
				t.Errorf("forbidden color mutation %q unexpectedly passed", source)
			}
		}
	})

	t.Run("reduced motion mutations", func(t *testing.T) {
		valid := `@media (prefers-reduced-motion: reduce) { *, *::before, *::after { animation-duration: 0.01ms !important; animation-delay: 0s !important; animation-iteration-count: 1 !important; transition-duration: 0.01ms !important; scroll-behavior: auto !important; } }`
		mutations := []struct{ old, new string }{
			{"animation-delay: 0s !important;", ""},
			{"animation-delay: 0s !important;", "animation-delay: 1s !important;"},
			{"animation-delay: 0s !important;", "animation-delay: 0s;"},
		}
		for _, mutation := range mutations {
			mutated := strings.Replace(valid, mutation.old, mutation.new, 1)
			if mutated == valid {
				t.Fatalf("reduced-motion target %q not found", mutation.old)
			}
			if err := validateStyleReducedMotion(mutated); err == nil {
				t.Errorf("reduced-motion mutation %q unexpectedly passed", mutation.new)
			}
		}
		moved := strings.Replace(valid, "animation-delay: 0s !important;", "", 1) + ` * { animation-delay: 0s !important; }`
		if err := validateStyleReducedMotion(moved); err == nil {
			t.Error("out-of-query delay mutation unexpectedly passed")
		}
		for _, mutated := range []string{
			strings.Replace(valid, "@media (prefers-reduced-motion: reduce)", "@media not (prefers-reduced-motion: reduce)", 1),
			strings.Replace(valid, "*, *::before, *::after {", "@media (min-width: 80rem) { *, *::before, *::after {", 1) + " }",
			strings.Replace(valid, "animation-delay: 0s !important;", `animation-del\61 y: 1s !important; animation-delay: 0s !important;`, 1),
			strings.TrimSuffix(valid, " }") + ` .specific { animation: hero 1s 1s !important; } }`,
			`@supports (display: grid) { ` + valid + ` }`,
			`.fixture { ` + valid + ` }`,
			valid + ` @supports (display: grid) { @media (prefers-reduced-motion: reduce) { .specific { animation: hero 1s !important; } } }`,
			valid + ` .fixture { @media (prefers-reduced-motion: reduce) { .specific { animation: hero 1s !important; } } }`,
		} {
			if err := validateStyleReducedMotion(mutated); err == nil {
				t.Errorf("reduced-motion bypass unexpectedly passed: %s", mutated)
			}
		}
	})

	t.Run("legacy hero alias mutation", func(t *testing.T) {
		for _, source := range []string{`return "page-kit-hero-variant-catalog"`, `return "page-kit-hero-" + "variant-catalog"`, `return "page-kit-hero-" /* split */ + "variant-catalog"`} {
			if err := validateStyleLegacyHeroAliases(source); err == nil {
				t.Errorf("legacy hero alias mutation unexpectedly passed: %s", source)
			}
		}
	})

	t.Run("route owner and sentinel mutations", func(t *testing.T) {
		valid := make(map[string]string)
		for _, route := range styleRouteOwners {
			valid[route.owner] = route.sentinel + ` { display: block; }`
		}
		wrongOwners := []string{"components.css", "shared.css", "base.css", "pages/contact.css"}
		for _, route := range styleRouteOwners {
			for _, family := range route.families {
				for _, wrongOwner := range wrongOwners {
					if wrongOwner == route.owner {
						wrongOwner = "pages/home.css"
					}
					mutated := cloneStyleSources(valid)
					mutated[wrongOwner] += "." + family + `-leak { display: block; }`
					if err := validateStyleRouteOwners(mutated); err == nil {
						t.Errorf("route family %q leak in %s unexpectedly passed", family, wrongOwner)
					}
				}
			}
			mutated := cloneStyleSources(valid)
			mutated[route.owner] = strings.Replace(mutated[route.owner], route.sentinel, route.sentinel+"-old", 1)
			if err := validateStyleRouteSentinels(mutated); err == nil {
				t.Errorf("sentinel removal for %s unexpectedly passed", route.owner)
			}
		}
		for _, source := range []string{`[class~="home-team"] { display: block; }`, `[class="home-team"] { display: block; }`, `.shared { & .home-team { display: block; } }`, `@scope (.home-team) { .shared { display: block; } }`, `.h\6f me-team { display: block; }`} {
			mutated := cloneStyleSources(valid)
			mutated["components.css"] = source
			if err := validateStyleRouteOwners(mutated); err == nil {
				t.Errorf("owner selector bypass unexpectedly passed: %s", source)
			}
		}
		mutated := cloneStyleSources(valid)
		mutated["pages/home.css"] = `@supports (display: impossible) { .home-page { display: block; } }`
		if err := validateStyleRouteSentinels(mutated); err == nil {
			t.Error("sentinel nested under @supports unexpectedly passed")
		}
		mutated = cloneStyleSources(valid)
		mutated["pages/home.css"] = `.home-page:not(*) { display: block; }`
		if err := validateStyleRouteSentinels(mutated); err == nil {
			t.Error("impossible sentinel selector unexpectedly passed")
		}
	})

	t.Run("forced-color trail mutations", func(t *testing.T) {
		valid := `@media (forced-colors: active) { [data-signal-trail] { forced-color-adjust: none; border-block-start: var(--line-signal) solid CanvasText !important; } [data-signal-trail]::before, [data-signal-trail]::after { display: none !important; } [data-signal-trail] .signal-trail-svg { display: none !important; } }`
		for _, mutation := range []struct{ old, new string }{
			{"forced-color-adjust: none;", ""},
			{"solid CanvasText !important", "solid transparent !important"},
			{"display: none !important;", "display: block;"},
			{"[data-signal-trail]::before, [data-signal-trail]::after { display: none !important; }", "[data-signal-trail]::before, [data-signal-trail]::after { display: block !important; }"},
		} {
			mutated := strings.Replace(valid, mutation.old, mutation.new, 1)
			if mutated == valid {
				t.Fatalf("forced-color target %q not found", mutation.old)
			}
			if err := validateStyleForcedColorTrail(mutated); err == nil {
				t.Errorf("forced-color trail mutation %q unexpectedly passed", mutation.new)
			}
		}
		for _, mutated := range []string{
			strings.Replace(valid, `[data-signal-trail] .signal-trail-svg { display: none !important; }`, `@media (min-width: 80rem) { [data-signal-trail] .signal-trail-svg { display: none !important; } }`, 1),
			strings.Replace(valid, `.signal-trail-svg`, `.not-signal-trail-svg`, 1),
			strings.Replace(valid, `display: none !important;`, `display: none !important; displ\61 y: block !important;`, 1),
			strings.Replace(valid, `display: none !important;`, `display: none !important; } [data-signal-trail] .strong .signal-trail-svg { display: block !important;`, 1),
			`@supports (forced-color-adjust: none) { ` + valid + ` }`,
			`.fixture { ` + valid + ` }`,
			valid + ` @supports (display: grid) { @media (forced-colors: active) { [data-signal-trail] .signal-trail-svg { display: block !important; } } }`,
			valid + ` .fixture { @media (forced-colors: active) { [data-signal-trail] .signal-trail-svg { display: block !important; } } }`,
			valid + ` @media (forced-colors: active) { .page [data-signal-trail]::before { display: block !important; } }`,
			valid + ` @media (forced-colors: active) { [data-signal-trail]::after { all: revert !important; } }`,
		} {
			if err := validateStyleForcedColorTrail(mutated); err == nil {
				t.Errorf("forced-color bypass unexpectedly passed: %s", mutated)
			}
		}
	})
}

func validateStyleDirectiveGraph(css string) error {
	commentFree, err := task2StripCSSComments(css)
	if err != nil {
		return fmt.Errorf("strip app.css comments: %w", err)
	}
	var got []string
	for _, line := range strings.Split(commentFree, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "@") {
			got = append(got, task2CanonicalCSS(line))
			continue
		}
		return fmt.Errorf("app.css contains non-directive content %q", line)
	}
	want := make([]string, len(styleExpectedDirectives))
	for index, directive := range styleExpectedDirectives {
		want[index] = task2CanonicalCSS(directive)
	}
	if !reflect.DeepEqual(got, want) {
		return fmt.Errorf("app.css directives = %#v, want %#v", got, want)
	}
	return nil
}

func validateNoOuterLayer(css string) error {
	commentFree, err := task2StripCSSComments(css)
	if err != nil {
		return err
	}
	for index := 0; index < len(commentFree); {
		if commentFree[index] == '\'' || commentFree[index] == '"' {
			next, skipErr := task2SkipCSSString(commentFree, index)
			if skipErr != nil {
				return skipErr
			}
			index = next
			continue
		}
		if commentFree[index] != '@' {
			index++
			continue
		}
		nameEnd := index + 1
		for nameEnd < len(commentFree) && (commentFree[nameEnd] >= 'a' && commentFree[nameEnd] <= 'z' || commentFree[nameEnd] >= 'A' && commentFree[nameEnd] <= 'Z' || commentFree[nameEnd] == '-') {
			nameEnd++
		}
		name := strings.ToLower(commentFree[index+1 : nameEnd])
		switch name {
		case "layer":
			return fmt.Errorf("project stylesheet contains outer @layer ownership")
		case "source":
			return fmt.Errorf("project stylesheet contains @source discovery directive")
		case "import":
			statementEnd := strings.IndexAny(commentFree[nameEnd:], ";\n")
			if statementEnd < 0 {
				statementEnd = len(commentFree) - nameEnd
			}
			if strings.Contains(strings.ToLower(commentFree[nameEnd:nameEnd+statementEnd]), "tailwindcss/utilities.css") {
				return fmt.Errorf("project stylesheet imports Tailwind utilities outside app.css")
			}
		}
		index = nameEnd
	}
	return nil
}

func validateStyleMigrationPaths(tailwindRoot string) error {
	var violations []string
	for _, name := range styleMigrationSources {
		path := filepath.Join(tailwindRoot, name)
		_, err := os.Stat(path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			continue
		case err != nil:
			violations = append(violations, fmt.Sprintf("stat migration stylesheet %s: %v", name, err))
		default:
			violations = append(violations, fmt.Sprintf("migration stylesheet still exists: %s", name))
		}
	}
	if len(violations) > 0 {
		return errors.New(strings.Join(violations, "; "))
	}
	return nil
}

func validateStyleBreakpoints(css string) error {
	commentFree, err := task2StripCSSComments(css)
	if err != nil {
		return err
	}
	re := regexp.MustCompile(`(?m)--breakpoint-([*a-zA-Z0-9_-]+)\s*:\s*([^;]+);`)
	got := make(map[string]string)
	for _, match := range re.FindAllStringSubmatch(commentFree, -1) {
		if _, duplicate := got[match[1]]; duplicate {
			return fmt.Errorf("duplicate --breakpoint-%s declaration", match[1])
		}
		got[match[1]] = strings.TrimSpace(match[2])
	}
	want := map[string]string{"*": "initial", "sm": "30rem", "md": "48rem", "lg": "70rem", "xl": "80rem"}
	if !reflect.DeepEqual(got, want) {
		return fmt.Errorf("breakpoint namespace = %#v, want %#v", got, want)
	}
	return nil
}

func validateStyleResponsiveVariants(source string) error {
	forbidden := regexp.MustCompile("(?i)(?:^|[^a-z0-9_-])((?:[a-z0-9_-]+:)*(?:xs:|2xl:|min-\\[[^\\]\\s\\\"'`]+\\]:|max-\\[[^\\]\\s\\\"'`]+\\]:)[^\\s\\\"'`]*)")
	if match := forbidden.FindStringSubmatch(source); len(match) > 1 {
		return fmt.Errorf("forbidden responsive class token %q", match[1])
	}
	return nil
}

func validateStyleMediaWidths(css string) error {
	commentFree, err := task2StripCSSComments(css)
	if err != nil {
		return err
	}
	mediaRE := regexp.MustCompile(`(?is)@media\s*([^\{]+)\{`)
	colonWidthRE := regexp.MustCompile(`(?i)\(\s*(min|max)-width\s*:\s*([0-9]+(?:\.[0-9]+)?)\s*(px|rem)\s*\)`)
	rangeWidthRE := regexp.MustCompile(`(?i)\(\s*width\s*(<|>=)\s*([0-9]+(?:\.[0-9]+)?)\s*(px|rem)\s*\)`)
	allowed := map[float64]bool{29.999: true, 30: true, 47.999: true, 48: true, 69.999: true, 70: true, 79.999: true, 80: true}
	for _, media := range mediaRE.FindAllStringSubmatch(commentFree, -1) {
		condition := media[1]
		lowerCondition := strings.ToLower(condition)
		if !strings.Contains(lowerCondition, "width") {
			continue
		}
		if strings.Contains(condition, "\\") || strings.Contains(lowerCondition, "calc(") || strings.Contains(condition, ",") || regexp.MustCompile(`(?i)\b(?:or|not)\b`).FindStringIndex(condition) != nil {
			return fmt.Errorf("unsupported width media composition %q", task2CanonicalCSS(condition))
		}
		colonMatches := colonWidthRE.FindAllStringSubmatch(condition, -1)
		rangeMatches := rangeWidthRE.FindAllStringSubmatch(condition, -1)
		if len(colonMatches)+len(rangeMatches) == 0 {
			return fmt.Errorf("unrecognized width media syntax %q", task2CanonicalCSS(condition))
		}
		matchedText := colonWidthRE.ReplaceAllString(condition, "")
		matchedText = rangeWidthRE.ReplaceAllString(matchedText, "")
		if regexp.MustCompile(`(?i)width`).FindStringIndex(matchedText) != nil {
			return fmt.Errorf("unsupported or reversed width media syntax %q", task2CanonicalCSS(condition))
		}
		var minimums []float64
		var maximums []float64
		for _, width := range colonMatches {
			value, parseErr := strconv.ParseFloat(width[2], 64)
			if parseErr != nil {
				return fmt.Errorf("parse media width %q: %w", width[0], parseErr)
			}
			if strings.EqualFold(width[3], "px") || !allowed[value] {
				return fmt.Errorf("noncanonical width media feature %q", width[0])
			}
			if strings.EqualFold(width[1], "min") {
				minimums = append(minimums, value)
			} else {
				maximums = append(maximums, value)
			}
		}
		for _, width := range rangeMatches {
			value, parseErr := strconv.ParseFloat(width[2], 64)
			if parseErr != nil {
				return fmt.Errorf("parse media width %q: %w", width[0], parseErr)
			}
			if strings.EqualFold(width[3], "px") || !allowed[value] {
				return fmt.Errorf("noncanonical width media feature %q", width[0])
			}
			if width[1] == ">=" {
				minimums = append(minimums, value)
			} else {
				maximums = append(maximums, value)
			}
		}
		for _, minimum := range minimums {
			for _, maximum := range maximums {
				if minimum > maximum {
					return fmt.Errorf("impossible width media range %q", task2CanonicalCSS(condition))
				}
			}
		}
	}
	return nil
}

func validateStyleJavaScriptWidths(js string) error {
	var widths []string
	executable, err := styleJavaScriptExecutable(js)
	if err != nil {
		return err
	}
	matchMediaRE := regexp.MustCompile(`\bmatchMedia\b`)
	for _, location := range matchMediaRE.FindAllStringIndex(executable, -1) {
		argument := strings.TrimLeft(js[location[1]:], " \t\r\n")
		if !strings.HasPrefix(argument, "(") {
			return fmt.Errorf("matchMedia uses unsupported property access or call syntax")
		}
		argument = argument[1:]
		argument = strings.TrimLeft(argument, " \t\r\n")
		if argument == "" || argument[0] != '\'' && argument[0] != '"' {
			return fmt.Errorf("matchMedia argument is not a quoted literal")
		}
		end, err := task2SkipCSSString(argument, 0)
		if err != nil {
			return fmt.Errorf("parse matchMedia literal: %w", err)
		}
		query := argument[1 : end-1]
		tail := strings.TrimLeft(argument[end:], " \t\r\n")
		if !strings.HasPrefix(tail, ")") {
			return fmt.Errorf("matchMedia call has nonliteral argument expression")
		}
		if strings.Contains(strings.ToLower(query), "width") {
			widths = append(widths, query)
		}
	}
	if styleHasComputedMatchMedia(js) {
		return fmt.Errorf("matchMedia uses computed property access")
	}
	want := []string{"(max-width: 69.999rem)"}
	if !reflect.DeepEqual(widths, want) {
		return fmt.Errorf("JavaScript width matchMedia queries = %#v, want %#v", widths, want)
	}
	return nil
}

func styleJavaScriptExecutable(js string) (string, error) {
	executable := []byte(js)
	for index := 0; index < len(js); {
		switch {
		case strings.HasPrefix(js[index:], "//"):
			end := strings.IndexByte(js[index:], '\n')
			if end < 0 {
				end = len(js) - index
			}
			for cursor := index; cursor < index+end; cursor++ {
				executable[cursor] = ' '
			}
			index += end
		case strings.HasPrefix(js[index:], "/*"):
			end := strings.Index(js[index+2:], "*/")
			if end < 0 {
				return "", fmt.Errorf("unterminated JavaScript comment at byte %d", index)
			}
			end += index + 4
			for cursor := index; cursor < end; cursor++ {
				executable[cursor] = ' '
			}
			index = end
		case js[index] == '\'' || js[index] == '"' || js[index] == '`':
			quote := js[index]
			end := index + 1
			for end < len(js) {
				if js[end] == '\\' {
					end += 2
					continue
				}
				if js[end] == quote {
					end++
					break
				}
				end++
			}
			if end > len(js) || end == len(js) && js[len(js)-1] != quote {
				return "", fmt.Errorf("unterminated JavaScript string at byte %d", index)
			}
			for cursor := index; cursor < end; cursor++ {
				executable[cursor] = ' '
			}
			index = end
		default:
			index++
		}
	}
	return string(executable), nil
}

func styleHasComputedMatchMedia(js string) bool {
	type token struct {
		kind  byte
		value string
	}
	var tokens []token
	for index := 0; index < len(js); {
		switch {
		case js[index] == ' ' || js[index] == '\t' || js[index] == '\r' || js[index] == '\n':
			index++
		case strings.HasPrefix(js[index:], "//"):
			end := strings.IndexByte(js[index:], '\n')
			if end < 0 {
				return false
			}
			index += end
		case strings.HasPrefix(js[index:], "/*"):
			end := strings.Index(js[index+2:], "*/")
			if end < 0 {
				return false
			}
			index += end + 4
		case js[index] == '\'' || js[index] == '"':
			quote := js[index]
			end := index + 1
			for end < len(js) && js[end] != quote {
				if js[end] == '\\' {
					end++
				}
				end++
			}
			if end >= len(js) {
				return false
			}
			tokens = append(tokens, token{kind: 's', value: js[index+1 : end]})
			index = end + 1
		default:
			tokens = append(tokens, token{kind: js[index], value: string(js[index])})
			index++
		}
	}
	for index := 0; index+6 < len(tokens); index++ {
		if tokens[index].value == "[" && tokens[index+1].kind == 's' && strings.EqualFold(tokens[index+1].value, "match") && tokens[index+2].value == "+" && tokens[index+3].kind == 's' && strings.EqualFold(tokens[index+3].value, "media") && tokens[index+4].value == "]" && tokens[index+5].value == "(" {
			return true
		}
	}
	for index := 0; index+3 < len(tokens); index++ {
		if tokens[index].value == "[" && tokens[index+1].kind == 's' && strings.EqualFold(tokens[index+1].value, "matchmedia") && tokens[index+2].value == "]" && tokens[index+3].value == "(" {
			return true
		}
	}
	return false
}

func validateStyleForbiddenColor(source string) error {
	if regexp.MustCompile(`(?i)--pollen-gold\b`).FindStringIndex(source) != nil {
		return fmt.Errorf("forbidden seventh core token --pollen-gold")
	}
	if regexp.MustCompile(`(?i)#ffd166\b`).FindStringIndex(source) != nil {
		return fmt.Errorf("forbidden seventh core literal #FFD166")
	}
	return nil
}

func validateStyleReducedMotion(css string) error {
	blocks, err := collectStyleBlocksWithDepth(css, 0)
	if err != nil {
		return err
	}
	var reduced []task2CSSBlock
	for _, nestedBlock := range blocks {
		block := nestedBlock.block
		header := strings.NewReplacer(" ", "", "\t", "", "\r", "", "\n", "").Replace(strings.ToLower(block.header))
		if strings.Contains(header, "prefers-reduced-motion") && header != "@media(prefers-reduced-motion:reduce)" {
			return fmt.Errorf("base.css has noncanonical reduced-motion media context %q", block.header)
		}
		if header == "@media(prefers-reduced-motion:reduce)" {
			if nestedBlock.depth != 0 {
				return fmt.Errorf("reduced-motion media is nested at depth %d", nestedBlock.depth)
			}
			if strings.Contains(block.body, "\\") {
				return fmt.Errorf("reduced-motion media contains escaped syntax")
			}
			nested, nestedErr := parseTask2CSSBlocks(block.body)
			if nestedErr != nil {
				return nestedErr
			}
			for _, candidate := range nested {
				if strings.HasPrefix(strings.TrimSpace(candidate.header), "@") {
					return fmt.Errorf("reduced-motion reset is nested under %q", candidate.header)
				}
			}
			reduced = append(reduced, block)
		}
	}
	if len(reduced) != 1 {
		return fmt.Errorf("base.css has %d reduced-motion media blocks; want one", len(reduced))
	}
	rules, err := collectTask2StyleRules(reduced[0].body)
	if err != nil {
		return err
	}
	want := map[string]string{
		"animation-duration":        "0.01ms !important",
		"animation-delay":           "0s !important",
		"animation-iteration-count": "1 !important",
		"transition-duration":       "0.01ms !important",
		"scroll-behavior":           "auto !important",
	}
	matchingRules := 0
	for _, rule := range rules {
		declarations := task2Declarations(rule.body)
		if task2CanonicalCSS(rule.header) != "*,*::before,*::after" {
			for _, property := range []string{"animation", "animation-duration", "animation-delay", "animation-iteration-count", "transition", "transition-duration", "scroll-behavior"} {
				if declarations[property] != "" {
					return fmt.Errorf("reduced-motion media has competing %s declaration in %q", property, rule.header)
				}
			}
			continue
		}
		matchingRules++
		if declarations["animation"] != "" || declarations["transition"] != "" {
			return fmt.Errorf("reduced-motion universal reset contains an animation or transition shorthand")
		}
		for property, value := range want {
			if !task2CSSValueEqual(declarations[property], value) {
				return fmt.Errorf("reduced-motion universal %s = %q, want %q", property, declarations[property], value)
			}
		}
	}
	if matchingRules != 1 {
		return fmt.Errorf("reduced-motion media has %d universal element/pseudo resets; want one", matchingRules)
	}
	return nil
}

func validateStyleRouteOwners(sources map[string]string) error {
	var violations []string
	for path, css := range sources {
		selectors, err := collectStyleSelectorHeaders(css)
		if err != nil {
			return fmt.Errorf("parse nested selectors in %s: %w", path, err)
		}
		for _, selector := range selectors {
			for _, className := range styleSelectorClassTokens(selector) {
				for _, route := range styleRouteOwners {
					for _, family := range route.families {
						if (className == family || strings.HasPrefix(className, family+"-")) && path != route.owner {
							violations = append(violations, fmt.Sprintf("%s owns .%s in selector %q; want %s", path, className, selector, route.owner))
						}
					}
				}
			}
		}
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		return fmt.Errorf("route selector ownership violations:\n%s", strings.Join(violations, "\n"))
	}
	return nil
}

func validateStyleRouteSentinels(sources map[string]string) error {
	var missing []string
	for _, route := range styleRouteOwners {
		css, found := sources[route.owner]
		if !found {
			missing = append(missing, fmt.Sprintf("%s missing", route.owner))
			continue
		}
		rules, err := parseTask2CSSBlocks(css)
		if err != nil {
			return fmt.Errorf("parse %s: %w", route.owner, err)
		}
		foundSentinel := false
		for _, rule := range rules {
			if strings.HasPrefix(strings.TrimSpace(rule.header), "@") {
				continue
			}
			for _, selector := range task2SplitTopLevel(rule.header, ',') {
				if styleSelectorHasExactClass(selector, strings.TrimPrefix(route.sentinel, ".")) && !strings.Contains(selector, ":") {
					foundSentinel = true
					break
				}
			}
		}
		if !foundSentinel {
			missing = append(missing, fmt.Sprintf("%s lacks %s", route.owner, route.sentinel))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("route sentinel violations: %s", strings.Join(missing, "; "))
	}
	return nil
}

func validateStyleLegacyHeroAliases(source string) error {
	commentFree := regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(source, "")
	joined := regexp.MustCompile(`[\s"'`+"`"+`+()]`).ReplaceAllString(commentFree, "")
	for _, alias := range []string{"page-kit-hero-variant-", "page-kit-hero-skills", "page-kit-hero-soccer"} {
		if strings.Contains(commentFree, alias) || strings.Contains(joined, alias) {
			return fmt.Errorf("legacy hero alias %q remains", alias)
		}
	}
	return nil
}

func validateStyleForcedColorTrail(css string) error {
	blocks, err := collectStyleBlocksWithDepth(css, 0)
	if err != nil {
		return err
	}
	forcedContexts := 0
	for _, nestedBlock := range blocks {
		block := nestedBlock.block
		header := strings.NewReplacer(" ", "", "\t", "", "\r", "", "\n", "").Replace(strings.ToLower(block.header))
		if !strings.Contains(header, "forced-colors") {
			continue
		}
		if header != "@media(forced-colors:active)" {
			return fmt.Errorf("base.css has noncanonical forced-color media context %q", block.header)
		}
		if nestedBlock.depth != 0 {
			return fmt.Errorf("forced-color media is nested at depth %d", nestedBlock.depth)
		}
		forcedContexts++
		if strings.Contains(block.body, "\\") {
			return fmt.Errorf("forced-color media contains escaped syntax")
		}
		nested, nestedErr := parseTask2CSSBlocks(block.body)
		if nestedErr != nil {
			return nestedErr
		}
		for _, candidate := range nested {
			if strings.HasPrefix(strings.TrimSpace(candidate.header), "@") {
				return fmt.Errorf("forced-color semantics are nested under %q", candidate.header)
			}
		}
	}
	if forcedContexts != 1 {
		return fmt.Errorf("base.css has %d exact forced-color contexts; want one", forcedContexts)
	}
	rules, err := collectTask2ForcedColorRules(css)
	if err != nil {
		return err
	}
	structural := false
	hidesSVG := false
	pseudoHideCounts := map[string]int{"::before": 0, "::after": 0}
	for _, rule := range rules {
		declarations := task2Declarations(rule.body)
		for _, selector := range task2SplitTopLevel(rule.header, ',') {
			canonical := task2CanonicalCSS(selector)
			if canonical == "[data-signal-trail]" {
				forcedAdjust := task2CSSValueEqual(declarations["forced-color-adjust"], "none")
				blockBorder := strings.Contains(strings.ToLower(task2CanonicalCSS(declarations["border-block-start"])), "canvastext")
				inlineBorder := strings.Contains(strings.ToLower(task2CanonicalCSS(declarations["border-inline-start"])), "canvastext")
				structural = structural || forcedAdjust && (blockBorder || inlineBorder)
			}
			if styleSelectorHasExactClass(canonical, "signal-trail-svg") {
				display := declarations["display"]
				if display != "" && !task2CSSValueEqual(display, "none !important") {
					return fmt.Errorf("forced colors can unhide signal-trail SVG with display %q", display)
				}
				hidesSVG = hidesSVG || task2CSSValueEqual(display, "none !important")
			}
			for pseudo := range pseudoHideCounts {
				if !strings.Contains(canonical, "[data-signal-trail]") || !strings.Contains(canonical, pseudo) {
					continue
				}
				if value := declarations["all"]; value != "" {
					return fmt.Errorf("forced colors can reset signal-trail %s with all:%s in %q", pseudo, value, selector)
				}
				display := declarations["display"]
				if display == "" {
					continue
				}
				if canonical != "[data-signal-trail]"+pseudo || !task2CSSValueEqual(display, "none !important") {
					return fmt.Errorf("forced colors have competing signal-trail %s display %q in %q", pseudo, display, selector)
				}
				pseudoHideCounts[pseudo]++
			}
		}
	}
	if !structural {
		return fmt.Errorf("forced colors lack a structural [data-signal-trail] system-color rule")
	}
	if !hidesSVG {
		return fmt.Errorf("forced colors do not hide the decorative signal-trail SVG")
	}
	for pseudo, count := range pseudoHideCounts {
		if count != 1 {
			return fmt.Errorf("forced colors have %d canonical signal-trail %s hide owners; want one", count, pseudo)
		}
	}
	return nil
}

func collectStyleSelectorHeaders(css string) ([]string, error) {
	blocks, err := parseTask2CSSBlocks(css)
	if err != nil {
		return nil, err
	}
	var selectors []string
	for _, block := range blocks {
		header := strings.TrimSpace(block.header)
		if strings.Contains(header, "\\") {
			return nil, fmt.Errorf("selector or at-rule prelude contains escaped syntax %q", header)
		}
		selectors = append(selectors, header)
		if strings.Contains(block.body, "{") {
			nested, nestedErr := collectStyleSelectorHeaders(block.body)
			if nestedErr != nil {
				return nil, nestedErr
			}
			selectors = append(selectors, nested...)
		}
	}
	return selectors, nil
}

type styleBlockWithDepth struct {
	block task2CSSBlock
	depth int
}

func collectStyleBlocksWithDepth(css string, depth int) ([]styleBlockWithDepth, error) {
	blocks, err := parseTask2CSSBlocks(css)
	if err != nil {
		return nil, err
	}
	var collected []styleBlockWithDepth
	for _, block := range blocks {
		collected = append(collected, styleBlockWithDepth{block: block, depth: depth})
		if !strings.Contains(block.body, "{") {
			continue
		}
		nested, nestedErr := collectStyleBlocksWithDepth(block.body, depth+1)
		if nestedErr != nil {
			return nil, nestedErr
		}
		collected = append(collected, nested...)
	}
	return collected, nil
}

func styleSelectorClassTokens(selector string) []string {
	dotClasses := regexp.MustCompile(`\.([_a-zA-Z][_a-zA-Z0-9-]*)`)
	attributeClasses := regexp.MustCompile(`(?i)\[\s*class\s*(?:~?=)\s*["']([_a-zA-Z][_a-zA-Z0-9-]*)["']\s*\]`)
	classes := make([]string, 0, len(dotClasses.FindAllStringSubmatch(selector, -1))+len(attributeClasses.FindAllStringSubmatch(selector, -1)))
	for _, match := range dotClasses.FindAllStringSubmatch(selector, -1) {
		classes = append(classes, match[1])
	}
	for _, match := range attributeClasses.FindAllStringSubmatch(selector, -1) {
		classes = append(classes, match[1])
	}
	return classes
}

func styleSelectorHasExactClass(selector, className string) bool {
	for _, got := range styleSelectorClassTokens(selector) {
		if got == className {
			return true
		}
	}
	return false
}

func validateStyleExplicitSourceClosure(t *testing.T, root string) error {
	t.Helper()
	fixtureRoot := t.TempDir()
	inputPath := filepath.Join(fixtureRoot, "input.css")
	explicitPath := filepath.Join(fixtureRoot, "explicit.templ")
	strayPath := filepath.Join(fixtureRoot, "stray.md")
	closedOutput := filepath.Join(fixtureRoot, "closed.css")
	openOutput := filepath.Join(fixtureRoot, "open.css")
	closedInput := `@import "tailwindcss/theme.css" layer(theme);
@source "./explicit.templ";
@import "tailwindcss/utilities.css" layer(utilities) source(none);`
	for path, contents := range map[string]string{
		inputPath:    closedInput,
		explicitPath: `<div class="w-[11rem]"></div>`,
		strayPath:    `<div class="w-[13rem]"></div>`,
	} {
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			return fmt.Errorf("write explicit-source fixture %s: %w", path, err)
		}
	}
	compile := func(output string) error {
		command := exec.Command(filepath.Join(root, ".tools", "tailwind", "tailwindcss"), "--cwd", fixtureRoot, "-i", inputPath, "-o", output)
		combined, err := command.CombinedOutput()
		if err != nil {
			return fmt.Errorf("compile explicit-source fixture: %w\n%s", err, combined)
		}
		return nil
	}
	if err := compile(closedOutput); err != nil {
		return err
	}
	closed, err := os.ReadFile(closedOutput)
	if err != nil {
		return fmt.Errorf("read closed fixture output: %w", err)
	}
	if !strings.Contains(string(closed), `width: 11rem`) || strings.Contains(string(closed), `width: 13rem`) {
		return fmt.Errorf("source(none) fixture did not keep only explicit source utility")
	}
	openInput := strings.Replace(closedInput, " source(none)", "", 1)
	if openInput == closedInput {
		return fmt.Errorf("source(none) mutation target not found")
	}
	if err := os.WriteFile(inputPath, []byte(openInput), 0o600); err != nil {
		return fmt.Errorf("write open-discovery fixture: %w", err)
	}
	if err := compile(openOutput); err != nil {
		return err
	}
	openCSS, err := os.ReadFile(openOutput)
	if err != nil {
		return fmt.Errorf("read open fixture output: %w", err)
	}
	if !strings.Contains(string(openCSS), `width: 13rem`) {
		return fmt.Errorf("removing source(none) did not reintroduce source-less fixture utility")
	}
	return nil
}

func styleRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate style architecture test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func styleReadFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}

func styleTailwindCSSSources(t *testing.T, tailwindRoot string) map[string]string {
	t.Helper()
	sources := make(map[string]string)
	err := filepath.WalkDir(tailwindRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".css" || filepath.Base(path) == "app.css" {
			return nil
		}
		relative, err := filepath.Rel(tailwindRoot, path)
		if err != nil {
			return err
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sources[filepath.ToSlash(relative)] = string(contents)
		return nil
	})
	if err != nil {
		t.Fatalf("read Tailwind sources: %v", err)
	}
	return sources
}

func styleAuthoritativeUISources(t *testing.T, root string) map[string]string {
	t.Helper()
	sources := make(map[string]string)
	for _, tree := range []string{filepath.Join(root, "cmd", "web", "tailwind"), filepath.Join(root, "cmd", "web")} {
		err := filepath.WalkDir(tree, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if path != tree && tree == filepath.Join(root, "cmd", "web") && filepath.Base(path) == "tailwind" {
					return filepath.SkipDir
				}
				return nil
			}
			extension := filepath.Ext(path)
			if extension != ".css" && extension != ".templ" && path != filepath.Join(root, "cmd", "web", "static", "js", "main.js") {
				return nil
			}
			if extension == ".css" && !strings.HasPrefix(path, filepath.Join(root, "cmd", "web", "tailwind")+string(filepath.Separator)) {
				return nil
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			sources[filepath.ToSlash(relative)] = string(contents)
			return nil
		})
		if err != nil {
			t.Fatalf("read authoritative UI sources below %s: %v", tree, err)
		}
	}
	uiTypes := filepath.Join(root, "cmd", "web", "partials", "ui_types.go")
	sources["cmd/web/partials/ui_types.go"] = styleReadFile(t, uiTypes)
	return sources
}

func cloneStyleSources(sources map[string]string) map[string]string {
	clone := make(map[string]string, len(sources))
	for path, source := range sources {
		clone[path] = source
	}
	return clone
}
