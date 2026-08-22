package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillsCSSIsImportedIntoPagesLayer(t *testing.T) {
	appCSS := readTask2Artifact(t, "cmd", "web", "tailwind", "app.css")
	const routeImport = `@import "./pages/skills.css" layer(pages);`
	if count := strings.Count(task2CanonicalCSS(appCSS), routeImport); count != 1 {
		t.Fatalf("Skills route import count = %d, want exactly one pages-layer import", count)
	}
}

func TestSkillsSelectorsHaveOneRouteOwner(t *testing.T) {
	for _, path := range []string{"shared.css", "base.css", "components.css"} {
		css := readTask2Artifact(t, "cmd", "web", "tailwind", path)
		rules, err := collectTask2StyleRules(css)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, rule := range rules {
			selector := task2CanonicalCSS(rule.header)
			if strings.Contains(selector, ".skills-") || strings.Contains(selector, ".skill-") || strings.Contains(selector, ".filter-tabs") {
				t.Errorf("%s still owns Skills selector %q", path, rule.header)
			}
		}
	}
}

func TestSkillsRouteCSSUsesApprovedWorkbenchTiers(t *testing.T) {
	css := readTask2Artifact(t, "cmd", "web", "tailwind", "pages", "skills.css")
	rules, err := collectAboutCSSRules(css, 0, false)
	if err != nil {
		t.Fatalf("parse Skills route CSS: %v", err)
	}
	required := []struct {
		selector string
		width    float64
		want     map[string]string
	}{
		{".skills-featured-icon", 0, map[string]string{"background": "var(--candle-oat)"}},
		{".skill-asset", 0, map[string]string{"object-fit": "contain"}},
		{".skills-filter-group", 0, map[string]string{"display": "grid"}},
		{".filter-tabs", 0, map[string]string{"display": "flex", "flex-wrap": "wrap", "overflow": "visible"}},
		{".skills-featured-mosaic", 0, map[string]string{"grid-template-columns": "minmax(0,1fr)"}},
		{".skills-practice-grid", 0, map[string]string{"grid-template-columns": "minmax(0,1fr)"}},
		{".skill-card-grid", 0, map[string]string{"grid-template-columns": "minmax(0,1fr)"}},
		{".skill-card-footer", 0, map[string]string{"display": "flex", "flex-wrap": "wrap"}},
		{".skills-filterable", 0, map[string]string{"position": "relative", "isolation": "isolate", "overflow": "hidden"}},
		{".skills-filterable > :not(.skills-workbench-trail)", 0, map[string]string{"position": "relative", "z-index": "1"}},
		{".skills-workbench-trail", 0, map[string]string{"position": "absolute", "z-index": "0", "width": "var(--line-signal)", "height": "auto", "pointer-events": "none"}},
		{".skills-workbench-trail .signal-trail-svg", 0, map[string]string{"display": "none"}},
		{".skill-card-grid", 30, map[string]string{"grid-template-columns": "repeat(2,minmax(0,1fr))"}},
		{".skill-card-grid", 48, map[string]string{"grid-template-columns": "repeat(3,minmax(0,1fr))"}},
		{".skill-card-grid", 70, map[string]string{"grid-template-columns": "repeat(4,minmax(0,1fr))"}},
		{".skill-card-grid", 80, map[string]string{"grid-template-columns": "repeat(5,minmax(0,1fr))"}},
		{".skills-practice-grid", 80, map[string]string{"grid-template-columns": "repeat(4,minmax(0,1fr))"}},
		{".skills-featured-mosaic", 80, map[string]string{"grid-template-columns": "repeat(12,minmax(0,1fr))"}},
		{".skills-workbench-trail", 80, map[string]string{"width": "auto", "height": "auto", "background": "transparent", "transform": "scaleX(-1)"}},
		{".skills-workbench-trail .signal-trail-svg", 80, map[string]string{"display": "block"}},
	}
	for _, requirement := range required {
		if !contactHasEffectiveRule(rules, requirement.selector, requirement.width, false, requirement.want) {
			t.Errorf("Skills CSS lacks %q at %.0frem with %v", requirement.selector, requirement.width, requirement.want)
		}
	}
	for _, rule := range rules {
		if task2CanonicalCSS(rule.selector) == ".skill-asset" && !rule.forcedColors {
			if _, filtered := rule.declarations["filter"]; filtered {
				t.Error("Skills assets still use a global color filter that distorts vendor logos")
			}
		}
	}
	allowed := map[float64]bool{0: true, 30: true, 48: true, 70: true, 80: true}
	for _, rule := range rules {
		selector := task2CanonicalCSS(rule.selector)
		if rule.forcedColors || !strings.Contains(selector, ".skill") {
			continue
		}
		if !allowed[rule.minWidthRem] {
			t.Errorf("Skills route uses noncanonical %.3frem breakpoint for %q", rule.minWidthRem, rule.selector)
		}
	}
}

func TestSkillsFiltersUseCompactWrappingGroups(t *testing.T) {
	template := readTask2Artifact(t, "cmd", "web", "partials", "skills_grid.templ")
	for _, contract := range []struct{ label, dataAttribute string }{
		{"Filter by category", "data-skill-filter-category"},
		{"Filter by proficiency", "data-skill-filter-proficiency"},
	} {
		group := `class="filter-tabs" role="group" aria-label="` + contract.label + `"`
		if strings.Count(template, group) != 1 || strings.Count(template, contract.dataAttribute) != 1 {
			t.Errorf("Skills %s wrapping group aliases are incomplete", contract.label)
		}
	}
	for _, marker := range []struct {
		value string
		count int
	}{
		{`classes := "skills-filter-tab filter-tab"`, 1},
		{`classes += " skills-filter-tab-active"`, 1},
		{`aria-current="page"`, 2},
	} {
		if got := strings.Count(template, marker.value); got != marker.count {
			t.Errorf("Skills filter markup contract count for %q = %d, want %d", marker.value, got, marker.count)
		}
	}
	for _, obsolete := range []string{"Swipe to scan", "skills-filter-rail", "skills-filter-hint", "skill-filter-dot", "data-filter-rail"} {
		if strings.Contains(template, obsolete) {
			t.Errorf("Skills filters retain obsolete horizontal-scroller marker %q", obsolete)
		}
	}
}

func TestSkillsFilterWrappingContract(t *testing.T) {
	css := readTask2Artifact(t, "cmd", "web", "tailwind", "pages", "skills.css")
	if err := validateSkillsFilterClearance(css); err != nil {
		t.Fatal(err)
	}
	for _, obsolete := range []string{".skills-filter-rail", ".skills-filter-hint", ".skill-filter-dot", "overflow-x: auto", "scroll-padding-inline", "overscroll-behavior-inline"} {
		if strings.Contains(css, obsolete) {
			t.Errorf("Skills CSS retains obsolete horizontal-scroller behavior %q", obsolete)
		}
	}
}

func TestSkillsFilterWrappingMutationGate(t *testing.T) {
	css := readTask2Artifact(t, "cmd", "web", "tailwind", "pages", "skills.css")
	mutations := []struct{ name, old, replacement string }{
		{"groups stop using grid", ".skills-filter-groups {\n  display: grid;", ".skills-filter-groups {\n  display: block;"},
		{"group stops using grid", ".skills-filter-group {\n  display: grid;", ".skills-filter-group {\n  display: block;"},
		{"tabs stop wrapping", "flex-wrap: wrap;", "flex-wrap: nowrap;"},
		{"horizontal scrolling returns", "overflow: visible;", "overflow-x: auto;"},
		{"focused tab loses lift", "transform: translateY(-0.08rem);", "transform: none;"},
		{"competing tabs rule disables wrap", "__append__", "\n.skills-page .filter-tabs { flex-wrap: nowrap !important; }\n"},
		{"competing tabs rule restores scroll", "__append__", "\n.skills-page .filter-tabs { overflow-x: scroll !important; }\n"},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			mutated := css + mutation.replacement
			if mutation.old != "__append__" {
				mutated = strings.Replace(css, mutation.old, mutation.replacement, 1)
			}
			if mutated == css {
				t.Fatalf("mutation target %q was not found", mutation.old)
			}
			if err := validateSkillsFilterClearance(mutated); err == nil {
				t.Fatal("wrapping validator accepted protected mutation")
			}
		})
	}
}

func TestSkillsAuthoredAppGraphMutationGate(t *testing.T) {
	graph := readFinal10Graph(t)
	mutations := []struct{ name, css string }{
		{"direct app disables filter wrapping", ".skills-page .filter-tabs { flex-wrap: nowrap !important; }"},
		{"direct app weakens catalog grid", ".skills-page .skill-card-grid { grid-template-columns: minmax(0, 1fr) !important; }"},
		{"direct app clips card footer", ".skills-page .skill-card-footer { flex-wrap: nowrap !important; }"},
		{"direct app turns compact trail into slab", ".skills-page .skills-workbench-trail { width: 100% !important; }"},
		{"direct app hides wide trail SVG", "@media (min-width: 80rem) { .skills-page .skills-workbench-trail .signal-trail-svg { display: none !important; } }"},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			mutated, changed := graph.mutate("cmd/web/tailwind/app.css", "", "", "\n"+mutation.css+"\n")
			if !changed {
				t.Fatal("app.css mutation target was not found")
			}
			if err := validateSkillsAuthoredGraph(mutated); err == nil {
				t.Fatal("Skills authored-graph validator accepted protected override")
			}
		})
	}
}

func validateSkillsFilterClearance(css string) error {
	const sourcePath = "cmd/web/tailwind/pages/skills.css"
	graph, err := loadFinal10Graph(filepath.Join("..", "..", "cmd", "web", "tailwind", "app.css"))
	if err != nil {
		return fmt.Errorf("load authored Skills graph: %w", err)
	}
	found := false
	for index := range graph.sources {
		if graph.sources[index].path == sourcePath {
			graph.sources[index].css = css
			found = true
		}
	}
	if !found {
		return fmt.Errorf("authored Skills graph lacks %s", sourcePath)
	}
	return validateSkillsAuthoredGraph(graph)
}

func validateSkillsAuthoredGraph(graph final10Graph) error {
	if err := validateSkillsFilterGraph(graph); err != nil {
		return err
	}
	return validateSkillsStructureGraph(graph)
}

type skillsProtectedTarget struct {
	target     final10Target
	properties map[string]bool
	allowed    []final10Effect
}

func validateSkillsProtectedRules(rules []final10Rule, targets []skillsProtectedTarget) error {
	for ruleIndex := range rules {
		rule := &rules[ruleIndex]
		if rule.modes&final10Ordinary == 0 {
			continue
		}
		for targetIndex := range targets {
			protected := &targets[targetIndex]
			matched, matchErr := final10RuleTargets(rule.selector, &protected.target)
			if matchErr != nil {
				return matchErr
			}
			if !matched {
				continue
			}
			if rule.unsupportedMedia {
				return fmt.Errorf("Skills %s uses unsupported media in %q", protected.target.label, rule.selector)
			}
			if utility := final10UnsafeApply(rule.body, "skills"); utility != "" {
				return fmt.Errorf("Skills %s has unapproved @apply %s in %q", protected.target.label, utility, rule.selector)
			}
			for property, value := range rule.declarations {
				if !protected.properties[property] {
					continue
				}
				allowed := false
				for effectIndex := range protected.allowed {
					allowed = allowed || final10EffectMatches(rule, &protected.allowed[effectIndex], property, value)
				}
				if !allowed {
					return skillsContractError(protected.target.label, rule.selector, property, value)
				}
			}
		}
	}
	return nil
}

func validateSkillsFilterGraph(graph final10Graph) error {
	rules, err := collectFinal10Rules(graph)
	if err != nil {
		return fmt.Errorf("parse Skills wrapping-filter CSS: %w", err)
	}
	const sourcePath = "cmd/web/tailwind/pages/skills.css"
	effects := []final10Effect{
		{sourcePath, ".skills-filter-groups", 0, final10Both, "display", "grid"},
		{sourcePath, ".skills-filter-group", 0, final10Both, "display", "grid"},
		{sourcePath, ".filter-tabs", 0, final10Both, "display", "flex"},
		{sourcePath, ".filter-tabs", 0, final10Both, "flex-wrap", "wrap"},
		{sourcePath, ".filter-tabs", 0, final10Both, "overflow", "visible"},
		{sourcePath, ".filter-tabs", 0, final10Both, "align-content", "flex-start"},
		{sourcePath, ".filter-tabs", 0, final10Both, "align-items", "flex-start"},
		{sourcePath, ".skills-filter-tab:focus-visible", 0, final10Both, "transform", "translateY(-0.08rem)"},
	}
	for effectIndex := range effects {
		effect := &effects[effectIndex]
		if final10EffectCount(rules, effect) != 1 {
			return fmt.Errorf("Skills wrapping-filter owner missing: %s %s at %.0frem", effect.selector, effect.property, effect.minWidthRem)
		}
	}
	targets := []skillsProtectedTarget{
		{final10Target{label: "filter groups", aliases: []string{".skills-filter-groups"}, guaranteed: []string{".skills-filter-groups"}}, map[string]bool{"display": true}, effects[0:1]},
		{final10Target{label: "filter group", aliases: []string{".skills-filter-group"}, guaranteed: []string{".skills-filter-group"}}, map[string]bool{"display": true}, effects[1:2]},
		{final10Target{label: "wrapping filter tabs", aliases: []string{".filter-tabs", "[aria-label=Filter by category]", "[aria-label=Filter by proficiency]"}, guaranteed: []string{".filter-tabs"}}, map[string]bool{"align-content": true, "align-items": true, "display": true, "flex-flow": true, "flex-wrap": true, "overflow": true, "overflow-x": true, "overflow-inline": true}, effects[2:7]},
		{final10Target{label: "focused filter tab", aliases: []string{".skills-filter-tab", ".skills-filter-tab-active", "[data-skill-filter-category]", "[data-skill-filter-proficiency]"}, guaranteed: []string{".skills-filter-tab", ":focus-visible"}}, map[string]bool{"transform": true, "translate": true}, effects[7:8]},
	}
	return validateSkillsProtectedRules(rules, targets)
}

func validateSkillsStructureGraph(graph final10Graph) error {
	rules, err := collectFinal10Rules(graph)
	if err != nil {
		return fmt.Errorf("parse Skills structure CSS: %w", err)
	}
	const sourcePath = "cmd/web/tailwind/pages/skills.css"
	effects := []final10Effect{
		{sourcePath, ".skill-card-grid", 0, final10Both, "display", "grid"},
		{sourcePath, ".skill-card-grid", 0, final10Both, "grid-template-columns", "minmax(0, 1fr)"},
		{sourcePath, ".skill-card-grid", 30, final10Both, "grid-template-columns", "repeat(2, minmax(0, 1fr))"},
		{sourcePath, ".skill-card-grid", 48, final10Both, "grid-template-columns", "repeat(3, minmax(0, 1fr))"},
		{sourcePath, ".skill-card-grid", 70, final10Both, "grid-template-columns", "repeat(4, minmax(0, 1fr))"},
		{sourcePath, ".skill-card-grid", 80, final10Both, "grid-template-columns", "repeat(5, minmax(0, 1fr))"},
		{sourcePath, ".skill-card-footer", 0, final10Both, "display", "flex"},
		{sourcePath, ".skill-card-footer", 0, final10Both, "flex-wrap", "wrap"},
		{sourcePath, ".skills-workbench-trail", 0, final10Both, "display", "block"},
		{sourcePath, ".skills-workbench-trail", 0, final10Both, "width", "var(--line-signal)"},
		{sourcePath, ".skills-workbench-trail", 80, final10Both, "width", "auto"},
		{sourcePath, ".skills-workbench-trail .signal-trail-svg", 0, final10Both, "display", "none"},
		{sourcePath, ".skills-workbench-trail .signal-trail-svg", 80, final10Both, "display", "block"},
	}
	for effectIndex := range effects {
		effect := &effects[effectIndex]
		if final10EffectCount(rules, effect) != 1 {
			return fmt.Errorf("Skills structure owner missing: %s %s at %.0frem", effect.selector, effect.property, effect.minWidthRem)
		}
	}
	targets := []skillsProtectedTarget{
		{final10Target{label: "card grid", aliases: []string{".skill-card-grid"}, guaranteed: []string{".skill-card-grid"}}, map[string]bool{"display": true, "grid": true, "grid-template": true, "grid-template-columns": true}, effects[0:6]},
		{final10Target{label: "card footer", aliases: []string{".skill-card-footer"}, guaranteed: []string{".skill-card-footer"}}, map[string]bool{"display": true, "flex-flow": true, "flex-wrap": true}, effects[6:8]},
		{final10Target{label: "workbench trail", aliases: []string{".skills-workbench-trail"}, guaranteed: []string{".skills-workbench-trail"}}, map[string]bool{"display": true, "inline-size": true, "width": true}, effects[8:11]},
		{final10Target{label: "workbench trail SVG", aliases: []string{".skills-workbench-trail .signal-trail-svg"}, guaranteed: []string{".skills-workbench-trail .signal-trail-svg"}}, map[string]bool{"display": true}, effects[11:13]},
	}
	return validateSkillsProtectedRules(rules, targets)
}

func skillsContractError(label, selector, property, value string) error {
	return fmt.Errorf("Skills %s has competing %s:%s in %q", label, property, value, selector)
}
