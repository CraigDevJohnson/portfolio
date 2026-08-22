package app

import (
	"strings"
	"testing"
)

func TestApprovedVisualAdjustmentsUseTypedIconsAndRemoveLowValueLabels(t *testing.T) {
	home := readTask2Artifact(t, "cmd", "web", "pages", "home.templ")
	for _, icon := range []string{
		"partials.UIIconArchitecture",
		"partials.UIIconInfrastructure",
		"partials.UIIconAutomation",
		"partials.UIIconSecurity",
	} {
		if !strings.Contains(home, "IconName: "+icon) {
			t.Errorf("Home capability cards do not use typed icon %s", icon)
		}
	}
	for _, glyph := range []string{`Icon: "☁"`, `Icon: "{ }"`, `Icon: "↗"`, `Icon: "◇"`} {
		if strings.Contains(home, glyph) {
			t.Errorf("Home capability cards retain plain glyph %q", glyph)
		}
	}

	projects := readTask2Artifact(t, "cmd", "web", "partials", "projects_grid.templ")
	if strings.Contains(projects, `Build { fmt.Sprintf("%02d", dossier.Project.ID) }`) {
		t.Error("Projects still render Build ## labels")
	}

	education := readTask2Artifact(t, "cmd", "web", "pages", "education.templ")
	if strings.Contains(education, "educationCredentialCountLabel") {
		t.Error("Education still renders credential-count labels")
	}
}

func TestApprovedAccentTrailsAreBackgroundLayers(t *testing.T) {
	aboutTemplate := readTask2Artifact(t, "cmd", "web", "pages", "about.templ")
	if strings.Contains(aboutTemplate, "about-switchback-transition") || strings.Contains(aboutTemplate, "about-switchback-caption") {
		t.Error("About still renders the accent trail as a labeled separator")
	}
	if !strings.Contains(aboutTemplate, `ExtraClass: "about-timeline-trail"`) {
		t.Error("About timeline is missing its typed background trail")
	}

	experienceTemplate := readTask2Artifact(t, "cmd", "web", "partials", "experience_kit_stages.templ")
	if !strings.Contains(experienceTemplate, `ExtraClass: "experience-orientation-trail"`) {
		t.Error("Experience orientation is missing its typed background trail")
	}

	aboutCSS := readTask2Artifact(t, "cmd", "web", "tailwind", "pages", "about.css")
	aboutRules, err := collectAboutCSSRules(aboutCSS, 0, false)
	if err != nil {
		t.Fatalf("parse About CSS: %v", err)
	}
	for _, requirement := range []struct {
		selector string
		want     map[string]string
	}{
		{".about-switchback", map[string]string{"position": "relative", "isolation": "isolate"}},
		{".about-switchback > *", map[string]string{"position": "relative", "z-index": "1"}},
		{".about-timeline-trail", map[string]string{"position": "absolute", "z-index": "0", "pointer-events": "none"}},
		{".about-timeline-list", map[string]string{"position": "relative", "z-index": "2"}},
	} {
		if !aboutHasRule(aboutRules, requirement.selector, 0, false, requirement.want) {
			t.Errorf("About CSS lacks background-trail rule %q with %v", requirement.selector, requirement.want)
		}
	}

	experienceCSS := readTask2Artifact(t, "cmd", "web", "tailwind", "pages", "experience.css")
	experienceRules, err := collectAboutCSSRules(experienceCSS, 0, false)
	if err != nil {
		t.Fatalf("parse Experience CSS: %v", err)
	}
	for _, requirement := range []struct {
		selector string
		want     map[string]string
	}{
		{".experience-orientation", map[string]string{"position": "relative", "isolation": "isolate"}},
		{".experience-orientation > :not(.experience-orientation-trail)", map[string]string{"position": "relative", "z-index": "1"}},
		{".experience-orientation-trail", map[string]string{"position": "absolute", "z-index": "0", "pointer-events": "none"}},
	} {
		if !aboutHasRule(experienceRules, requirement.selector, 0, false, requirement.want) {
			t.Errorf("Experience CSS lacks background-trail rule %q with %v", requirement.selector, requirement.want)
		}
	}
}

func TestApprovedSkillsAndFooterTreatmentsAreConsistent(t *testing.T) {
	skillsCSS := readTask2Artifact(t, "cmd", "web", "tailwind", "pages", "skills.css")
	skillRules, err := collectAboutCSSRules(skillsCSS, 0, false)
	if err != nil {
		t.Fatalf("parse Skills CSS: %v", err)
	}
	for _, selector := range []string{".skills-featured-icon", ".skills-practice-icon", ".skill-icon-frame"} {
		if !contactHasEffectiveRule(skillRules, selector, 0, false, map[string]string{
			"overflow":   "hidden",
			"background": "var(--candle-oat)",
		}) {
			t.Errorf("Skills icon owner %q does not use the shared framed treatment", selector)
		}
	}

	componentsCSS := readTask2Artifact(t, "cmd", "web", "tailwind", "components.css")
	componentRules, err := collectAboutCSSRules(componentsCSS, 0, false)
	if err != nil {
		t.Fatalf("parse component CSS: %v", err)
	}
	if !contactHasEffectiveRule(componentRules, ".footer-nav-list-portfolio", 48, false, map[string]string{
		"display":               "grid",
		"grid-template-columns": "repeat(2,minmax(0,1fr))",
	}) {
		t.Error("Footer Portfolio links are not compacted into two columns at wider widths")
	}

	baseCSS := readTask2Artifact(t, "cmd", "web", "tailwind", "base.css")
	for path, css := range map[string]string{"base.css": baseCSS, "components.css": componentsCSS} {
		if strings.Contains(css, ".site-chrome-nav-link-active::before") || strings.Contains(css, ".site-chrome-nav-link:hover::before") {
			t.Errorf("%s still renders the active navigation dot", path)
		}
	}
}
