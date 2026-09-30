package app

import "testing"

func TestFooterLinksFitTheirGridContent(t *testing.T) {
	css := readTask2Artifact(t, "cmd", "web", "tailwind", "components.css")
	rules, err := collectAboutCSSRules(css, 0, false)
	if err != nil {
		t.Fatalf("parse component CSS: %v", err)
	}

	if !contactHasEffectiveRule(rules, ".site-chrome-footer-link", 0, false, map[string]string{
		"justify-self": "start",
	}) {
		t.Error("footer links stretch their focus and touch area across the navigation grid column")
	}
}

func TestFooterTechnologyPillsHaveHoverFeedback(t *testing.T) {
	css := readTask2Artifact(t, "cmd", "web", "tailwind", "components.css")
	rules, err := collectAboutCSSRules(css, 0, false)
	if err != nil {
		t.Fatalf("parse component CSS: %v", err)
	}

	if !contactHasEffectiveRule(rules, "a.tech-pill:hover", 0, false, map[string]string{
		"border-color": "color-mix(in srgb,var(--pond-mint) 62%,transparent)",
		"background":   "color-mix(in srgb,var(--pond-mint) 17%,var(--cocoa-cedar))",
		"color":        "var(--candle-oat)",
	}) {
		t.Error("linked technology pills do not provide clear hover feedback")
	}
}

func TestFooterCopyrightUsesMutedHierarchy(t *testing.T) {
	css := readTask2Artifact(t, "cmd", "web", "tailwind", "components.css")
	rules, err := collectAboutCSSRules(css, 0, false)
	if err != nil {
		t.Fatalf("parse component CSS: %v", err)
	}

	if !contactHasEffectiveRule(rules, ".site-chrome-footer .copyright", 0, false, map[string]string{
		"color": "var(--color-copy-muted)",
	}) {
		t.Error("footer copyright does not use the footer's muted visual hierarchy")
	}
}

func TestOperatorFooterResponsiveStyleContract(t *testing.T) {
	css := readTask2Artifact(t, "cmd", "web", "tailwind", "components.css")
	rules, err := collectAboutCSSRules(css, 0, false)
	if err != nil {
		t.Fatalf("parse component CSS: %v", err)
	}

	if !contactHasEffectiveRule(rules, ".site-operator-footer-shell", 0, false, map[string]string{
		"flex-wrap": "wrap",
	}) {
		t.Error("operator footer does not wrap before its contents can overflow")
	}

	if !contactHasEffectiveRule(rules, ".site-operator-footer-shell .site-chrome-footer-link", 48, false, map[string]string{
		"align-self": "center",
	}) {
		t.Error("operator footer links do not align with the centered tablet and desktop row")
	}
}
