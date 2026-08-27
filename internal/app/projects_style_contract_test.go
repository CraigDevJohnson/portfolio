package app

import (
	"fmt"
	"strings"
	"testing"
)

func TestProjectsRouteCSSContract(t *testing.T) {
	css := readTask2Artifact(t, "cmd", "web", "tailwind", "pages", "projects.css")
	if err := validateProjectsRouteCSS(css); err != nil {
		t.Fatal(err)
	}
}

func TestProjectsRouteCSSValidatorRejectsRegressions(t *testing.T) {
	css := readTask2Artifact(t, "cmd", "web", "tailwind", "pages", "projects.css")
	tests := []struct {
		name string
		old  string
		new  string
	}{
		{name: "tablet supporting dossiers stop using two explicit columns", old: `.project-dossier-support-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }`, new: `.project-dossier-support-grid {
    grid-template-columns: minmax(0, 1fr);
  }`},
		{name: "wide landscape dossier loses metadata placement", old: `.project-dossier-support[data-image-ratio="landscape"] {
    grid-column: span 7;
    grid-row: 1;
  }`, new: `.project-dossier-support[data-image-ratio="landscape"] {
    grid-column: auto;
    grid-row: 1;
  }`},
		{name: "wide square dossier loses metadata placement", old: `.project-dossier-support[data-image-ratio="square"] {
    grid-column: span 5;
    grid-row: 1;
  }`, new: `.project-dossier-support[data-image-ratio="square"] {
    grid-column: auto;
    grid-row: 1;
  }`},
		{name: "70rem lead split disappears", old: `grid-template-columns: minmax(0, 0.9fr) minmax(0, 1.1fr);`, new: `grid-template-columns: minmax(0, 1fr);`},
		{name: "80rem lead ratio disappears", old: `grid-template-columns: minmax(0, 5fr) minmax(0, 7fr);`, new: `grid-template-columns: repeat(2, minmax(0, 1fr));`},
		{name: "support cards stretch to equal heights", old: `align-items: start;`, new: `align-items: stretch;`},
		{name: "support card loses natural height", old: `height: auto;`, new: `height: 100%;`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutated := strings.Replace(css, test.old, test.new, 1)
			if mutated == css {
				t.Fatalf("test mutation target %q not found", test.old)
			}
			if err := validateProjectsRouteCSS(mutated); err == nil {
				t.Fatal("validateProjectsRouteCSS() error = nil, want regression rejected")
			}
		})
	}
}

func TestProjectsCSSIsImportedIntoPagesLayer(t *testing.T) {
	appCSS := readTask2Artifact(t, "cmd", "web", "tailwind", "app.css")
	const routeImport = `@import "./pages/projects.css" layer(pages);`
	if count := strings.Count(task2CanonicalCSS(appCSS), routeImport); count != 1 {
		t.Fatalf("Projects route import count = %d, want exactly one pages-layer import", count)
	}
}

func TestProjectsSelectorsHaveOneRouteOwner(t *testing.T) {
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
			if strings.Contains(selector, ".project-") || strings.Contains(selector, ".projects-") {
				t.Errorf("legacy stylesheet %d still owns Projects selector %q", index, rule.header)
			}
		}
	}
}

func validateProjectsRouteCSS(css string) error {
	rules, err := collectAboutCSSRules(css, 0, false)
	if err != nil {
		return fmt.Errorf("parse Projects route CSS: %w", err)
	}

	required := []struct {
		selector string
		minWidth float64
		want     map[string]string
	}{
		{selector: ".project-dossier-composition", want: map[string]string{"position": "relative", "isolation": "isolate", "display": "grid"}},
		{selector: ".project-dossier-lead-region", want: map[string]string{"position": "relative", "z-index": "1"}},
		{selector: ".project-dossier", want: map[string]string{"display": "grid", "min-width": "0"}},
		{selector: ".project-dossier-support-grid", want: map[string]string{"display": "grid"}},
		{selector: ".project-dossier-support-grid", want: map[string]string{"position": "relative", "z-index": "1"}},
		{selector: ".project-dossier-support-grid", want: map[string]string{"align-items": "start"}},
		{selector: ".project-dossier-support", want: map[string]string{"align-self": "start", "height": "auto", "grid-column": "auto", "grid-row": "auto"}},
		{selector: ".projects-dossier-signal", want: map[string]string{"position": "absolute", "z-index": "0", "display": "block", "width": "var(--line-signal)", "height": "auto", "margin": "0", "pointer-events": "none", "transform": "none"}},
		{selector: ".projects-dossier-signal .signal-trail-svg", want: map[string]string{"display": "none"}},
		{selector: `.project-dossier[data-image-ratio="landscape"]`, want: map[string]string{"--dossier-signal": "var(--campfire-apricot)"}},
		{selector: `.project-dossier[data-image-ratio="square"]`, want: map[string]string{"--dossier-signal": "var(--rosehip)"}},
		{selector: ".project-dossier-support-grid", minWidth: 48, want: map[string]string{"grid-template-columns": "repeat(2, minmax(0, 1fr))"}},
		{selector: ".projects-dossier-signal", minWidth: 48, want: map[string]string{"width": "auto", "height": "auto", "background": "transparent"}},
		{selector: ".projects-dossier-signal .signal-trail-svg", minWidth: 48, want: map[string]string{"display": "block"}},
		{selector: ".project-dossier-support", minWidth: 48, want: map[string]string{"grid-column": "auto", "grid-row": "auto"}},
		{selector: ".project-dossier-lead", minWidth: 70, want: map[string]string{"grid-template-columns": "minmax(0, 0.9fr) minmax(0, 1.1fr)"}},
		{selector: ".project-dossier-support", minWidth: 70, want: map[string]string{"grid-column": "auto", "grid-row": "auto"}},
		{selector: ".project-dossier-lead", minWidth: 80, want: map[string]string{"grid-template-columns": "minmax(0, 5fr) minmax(0, 7fr)"}},
		{selector: ".project-dossier-support-grid", minWidth: 80, want: map[string]string{"grid-template-columns": "repeat(12, minmax(0, 1fr))"}},
		{selector: ".project-dossier-support", minWidth: 80, want: map[string]string{"grid-column": "auto", "grid-row": "auto"}},
		{selector: `.project-dossier-support[data-image-ratio="landscape"]`, minWidth: 80, want: map[string]string{"grid-column": "span 7", "grid-row": "1"}},
		{selector: `.project-dossier-support[data-image-ratio="square"]`, minWidth: 80, want: map[string]string{"grid-column": "span 5", "grid-row": "1"}},
	}
	for _, requirement := range required {
		if !aboutHasRule(rules, requirement.selector, requirement.minWidth, false, requirement.want) {
			return fmt.Errorf("Projects route CSS lacks %q at min-width %.0frem with %v", requirement.selector, requirement.minWidth, requirement.want)
		}
	}
	return nil
}

func TestProjectsHeroUsesTypedCaseStudyOwnerOnly(t *testing.T) {
	for path, source := range styleAuthoritativeUISources(t, styleRepositoryRoot(t)) {
		if err := validateStyleLegacyHeroAliases(source); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
	template := readTask2Artifact(t, "cmd", "web", "pages", "projects.templ")
	for _, leak := range []string{`ExtraClass:`, `ContentClass:`} {
		if strings.Contains(template, leak) {
			t.Errorf("Projects hero leaks route/layout styling through %q", leak)
		}
	}
}
