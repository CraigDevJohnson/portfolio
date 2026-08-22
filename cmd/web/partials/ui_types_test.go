package partials

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func TestHeroVariantClass(t *testing.T) {
	tests := []struct {
		variant HeroVariant
		want    string
	}{
		{HeroNarrative, "page-hero-narrative"},
		{HeroTimeline, "page-hero-timeline"},
		{HeroCatalog, "page-hero-catalog"},
		{HeroCaseStudy, "page-hero-case-study"},
		{HeroInvitation, "page-hero-invitation"},
		{HeroTool, "page-hero-tool"},
		{HeroVariant("unknown"), "page-hero-standard"},
	}
	for _, test := range tests {
		if got := heroVariantClass(test.variant); got != test.want {
			t.Errorf("heroVariantClass(%q) = %q, want %q", test.variant, got, test.want)
		}
	}
}

func TestPageHeroEmitsOnlyCanonicalTypedVariant(t *testing.T) {
	html := renderComponent(t, PageHero(PageHeroProps{Variant: HeroCatalog}))
	if !strings.Contains(html, "page-hero-catalog") {
		t.Fatalf("PageHero output lacks canonical catalog variant: %s", html)
	}
	if strings.Contains(html, "page-kit-hero-variant-") {
		t.Fatalf("PageHero output retains legacy hero variant alias: %s", html)
	}
}

func TestClosedVisualVocabularyClasses(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"tone apricot", toneClass(ToneApricot), "ui-tone-apricot"},
		{"tone rose", toneClass(ToneRose), "ui-tone-rose"},
		{"tone mint", toneClass(ToneMint), "ui-tone-mint"},
		{"tone unknown", toneClass(Tone("unknown")), "ui-tone-quiet"},
		{"action primary", actionVariantClass(ActionPrimary), "ui-action-primary"},
		{"action secondary", actionVariantClass(ActionSecondary), "ui-action-secondary"},
		{"action quiet", actionVariantClass(ActionQuiet), "ui-action-quiet"},
		{"action unknown", actionVariantClass(ActionVariant("unknown")), "ui-action-quiet"},
		{"feedback info", feedbackKindClass(FeedbackInfo), "ui-feedback-info"},
		{"feedback success", feedbackKindClass(FeedbackSuccess), "ui-feedback-success"},
		{"feedback warning", feedbackKindClass(FeedbackWarning), "ui-feedback-warning"},
		{"feedback error", feedbackKindClass(FeedbackError), "ui-feedback-error"},
		{"feedback unknown", feedbackKindClass(FeedbackKind("unknown")), "ui-feedback-quiet"},
		{"stat grid summary", statGridVariantClass(StatGridSummary), "stat-grid-summary"},
		{"stat grid unknown", statGridVariantClass(StatGridVariant("unknown")), "stat-grid-standard"},
		{"trail topology", signalTrailVariantClass(TrailTopology), "signal-trail-topology"},
		{"trail switchback", signalTrailVariantClass(TrailSwitchback), "signal-trail-switchback"},
		{"trail timeline", signalTrailVariantClass(TrailTimeline), "signal-trail-timeline"},
		{"trail workbench", signalTrailVariantClass(TrailWorkbench), "signal-trail-workbench"},
		{"trail dossier", signalTrailVariantClass(TrailDossier), "signal-trail-dossier"},
		{"trail field guide", signalTrailVariantClass(TrailFieldGuide), "signal-trail-field-guide"},
		{"trail correspondence", signalTrailVariantClass(TrailCorrespondence), "signal-trail-correspondence"},
		{"trail matchday", signalTrailVariantClass(TrailMatchday), "signal-trail-matchday"},
		{"trail operator", signalTrailVariantClass(TrailOperator), "signal-trail-operator"},
		{"trail interruption", signalTrailVariantClass(TrailInterruption), "signal-trail-interruption"},
		{"trail unknown", signalTrailVariantClass(SignalTrailVariant("unknown")), "signal-trail-standard"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Errorf("class = %q, want %q", test.got, test.want)
			}
		})
	}
}

func TestStatGridSummaryPreservesRouteOwnedGridClass(t *testing.T) {
	got := statCardGridClasses(StatCardGridProps{
		Variant:   StatGridSummary,
		GridClass: "experience-overview-stats",
	})
	if want := "experience-overview-stats stat-grid-summary"; got != want {
		t.Fatalf("statCardGridClasses(summary) = %q, want %q", got, want)
	}
	for _, leakedLayoutClass := range []string{"grid-cols-2", "xl:grid-cols-4"} {
		if strings.Contains(got, leakedLayoutClass) {
			t.Errorf("summary grid class %q leaks shared layout utility %q", got, leakedLayoutClass)
		}
	}
}

func TestActionLinkExternalSemantics(t *testing.T) {
	html := renderComponent(t, ActionLink(ActionLinkProps{
		Href:     "https://example.com",
		Label:    "Reference",
		Variant:  ActionSecondary,
		External: true,
	}))

	for _, marker := range []string{`target="_blank"`, `rel="noopener noreferrer"`, `(opens in a new tab)`} {
		if !strings.Contains(html, marker) {
			t.Errorf("external ActionLink output does not contain %q: %s", marker, html)
		}
	}
}

func TestPageCTAPreservesActionVariantsAndPrimaryArrow(t *testing.T) {
	secondary := ActionLinkProps{
		Href:    "/secondary",
		Label:   "Secondary",
		Variant: ActionQuiet,
	}
	html := renderComponent(t, PageCTA(PageCTAProps{
		Title:    "Continue",
		Subtitle: "Choose a path.",
		Primary: ActionLinkProps{
			Href:    "/primary",
			Label:   "Primary",
			Variant: ActionPrimary,
		},
		Secondary: &secondary,
	}))

	for _, marker := range []string{"ui-action-primary", "ui-action-quiet", `class="btn-icon" aria-hidden="true">→</span>`} {
		if !strings.Contains(html, marker) {
			t.Errorf("PageCTA output does not contain %q: %s", marker, html)
		}
	}
	if got := strings.Count(html, `class="btn-icon"`); got != 1 {
		t.Errorf("PageCTA decorative arrow count = %d, want 1: %s", got, html)
	}
}

func TestActionButtonDisabledSemantics(t *testing.T) {
	html := renderComponent(t, ActionButton(ActionButtonProps{
		Type:     "submit",
		Label:    "Refresh metrics",
		Variant:  ActionPrimary,
		Disabled: true,
	}))

	for _, marker := range []string{`disabled`, `aria-disabled="true"`} {
		if !strings.Contains(html, marker) {
			t.Errorf("disabled ActionButton output does not contain %q: %s", marker, html)
		}
	}
}

func TestFeedbackErrorUsesAlertRole(t *testing.T) {
	html := renderComponent(t, Feedback(FeedbackProps{
		Kind:    FeedbackError,
		Title:   "Unable to load metrics",
		Message: "Try again shortly.",
	}))

	if !strings.Contains(html, `role="alert"`) {
		t.Errorf("Feedback error output does not use role=alert: %s", html)
	}
}

func TestOverflowRegionDescribesFocusableScrollContainer(t *testing.T) {
	html := renderComponent(t, OverflowRegion(OverflowRegionProps{
		ID:    "credential-table",
		Label: "Credentials",
		Hint:  "Scroll horizontally to view every column.",
	}))

	for _, marker := range []string{`id="credential-table-hint"`, `id="credential-table"`, `aria-describedby="credential-table-hint"`, `tabindex="0"`} {
		if !strings.Contains(html, marker) {
			t.Errorf("OverflowRegion output does not contain %q: %s", marker, html)
		}
	}
}

func TestSignalTrailRendersExactlyOneMarker(t *testing.T) {
	html := renderComponent(t, SignalTrail(SignalTrailProps{Variant: TrailTopology}))
	requireOneFullPageTrail(t, html)
	if !strings.Contains(html, "signal-trail-topology") {
		t.Errorf("SignalTrail output does not contain its typed route-shape class: %s", html)
	}
	if got := strings.Count(html, "<path"); got != 2 {
		t.Errorf("SignalTrail path count = %d, want shadow and line paths", got)
	}
	if got := strings.Count(html, "<circle"); got != 0 {
		t.Errorf("SignalTrail circle count = %d, want no decorative nodes", got)
	}
}

func TestSignalTrailVariantsUseDistinctGradientIDs(t *testing.T) {
	topology := renderComponent(t, SignalTrail(SignalTrailProps{Variant: TrailTopology}))
	timeline := renderComponent(t, SignalTrail(SignalTrailProps{Variant: TrailTimeline}))
	for name, contract := range map[string]struct {
		html string
		id   string
	}{
		"topology": {html: topology, id: "signal-trail-gradient-topology"},
		"timeline": {html: timeline, id: "signal-trail-gradient-timeline"},
	} {
		if !strings.Contains(contract.html, `id="`+contract.id+`"`) || !strings.Contains(contract.html, `stroke="url(#`+contract.id+`)"`) {
			t.Errorf("%s trail does not bind its paths to unique gradient %q: %s", name, contract.id, contract.html)
		}
	}
	if strings.Contains(topology, `id="signal-trail-gradient-timeline"`) || strings.Contains(timeline, `id="signal-trail-gradient-topology"`) {
		t.Error("signal trail variants share gradient IDs")
	}
}

func TestTypedUIIconsRenderDecorativeLocalSVGs(t *testing.T) {
	icons := []UIIconName{
		UIIconMail,
		UIIconLinkedIn,
		UIIconGitHub,
		UIIconArchitecture,
		UIIconInfrastructure,
		UIIconAutomation,
		UIIconSecurity,
		UIIconObservability,
	}
	for _, icon := range icons {
		html := renderComponent(t, UIIcon(icon))
		for _, marker := range []string{"<svg", `class="page-kit-svg-icon"`, `aria-hidden="true"`, `focusable="false"`, `data-ui-icon="` + string(icon) + `"`} {
			if !strings.Contains(html, marker) {
				t.Errorf("UIIcon(%q) missing %q: %s", icon, marker, html)
			}
		}
	}
}

func TestCardsPreferTypedIconsOverTextGlyphs(t *testing.T) {
	feature := renderComponent(t, FeatureCard(FeatureCardProps{Icon: "legacy", IconName: UIIconSecurity, Title: "Security"}))
	row := renderComponent(t, LinkPanelRow(LinkPanelRowProps{Href: "mailto:test@example.com", IconName: UIIconMail, Title: "Email"}))
	for name, html := range map[string]string{"feature": feature, "row": row} {
		if !strings.Contains(html, "<svg") || strings.Contains(html, ">legacy<") {
			t.Errorf("%s card did not prefer its typed SVG icon: %s", name, html)
		}
	}
}

func TestSectionIntroUsesOptionalStableHeadingID(t *testing.T) {
	withID := renderComponent(t, SectionIntro(SectionIntroProps{Label: "Workbench", Title: "Toolkit", HeadingID: "toolkit-title"}))
	if !strings.Contains(withID, `<h2 class="page-section-title" id="toolkit-title">Toolkit</h2>`) {
		t.Fatalf("SectionIntro did not render supplied heading ID: %s", withID)
	}
	withoutID := renderComponent(t, SectionIntro(SectionIntroProps{Label: "Workbench", Title: "Toolkit"}))
	if strings.Contains(withoutID, `<h2 class="page-section-title" id=`) {
		t.Fatalf("SectionIntro rendered empty optional heading ID: %s", withoutID)
	}
}

func requireOneFullPageTrail(t *testing.T, html string) {
	t.Helper()
	if got := strings.Count(html, "data-signal-trail"); got != 1 {
		t.Fatalf("signal trail marker count = %d, want 1", got)
	}
}

func renderComponent(t *testing.T, component templ.Component) string {
	t.Helper()
	var output bytes.Buffer
	if err := component.Render(context.Background(), &output); err != nil {
		t.Fatalf("render component: %v", err)
	}
	return output.String()
}
