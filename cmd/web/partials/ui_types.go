package partials

import (
	"fmt"
	"strconv"

	"github.com/a-h/templ"
)

type Tone string

const (
	ToneApricot Tone = "apricot"
	ToneRose    Tone = "rose"
	ToneMint    Tone = "mint"
)

type UIIconName string

const (
	UIIconMail           UIIconName = "mail"
	UIIconLinkedIn       UIIconName = "linkedin"
	UIIconGitHub         UIIconName = "github"
	UIIconArchitecture   UIIconName = "architecture"
	UIIconInfrastructure UIIconName = "infrastructure"
	UIIconAutomation     UIIconName = "automation"
	UIIconSecurity       UIIconName = "security"
	UIIconObservability  UIIconName = "observability"
)

type HeroVariant string

const (
	HeroIdentity   HeroVariant = "identity"
	HeroNarrative  HeroVariant = "narrative"
	HeroTimeline   HeroVariant = "timeline"
	HeroCatalog    HeroVariant = "catalog"
	HeroCaseStudy  HeroVariant = "case-study"
	HeroInvitation HeroVariant = "invitation"
	HeroTool       HeroVariant = "tool"
)

type ActionVariant string

const (
	ActionPrimary   ActionVariant = "primary"
	ActionSecondary ActionVariant = "secondary"
	ActionQuiet     ActionVariant = "quiet"
	ActionDanger    ActionVariant = "danger"
)

type FeedbackKind string

const (
	FeedbackInfo    FeedbackKind = "info"
	FeedbackSuccess FeedbackKind = "success"
	FeedbackWarning FeedbackKind = "warning"
	FeedbackError   FeedbackKind = "error"
)

type StatGridVariant string

const (
	StatGridHero    StatGridVariant = "hero"
	StatGridCompact StatGridVariant = "compact"
	StatGridSummary StatGridVariant = "summary"
)

type SignalTrailVariant string

const (
	TrailTopology       SignalTrailVariant = "topology"
	TrailSwitchback     SignalTrailVariant = "switchback"
	TrailTimeline       SignalTrailVariant = "timeline"
	TrailWorkbench      SignalTrailVariant = "workbench"
	TrailDossier        SignalTrailVariant = "dossier"
	TrailFieldGuide     SignalTrailVariant = "field-guide"
	TrailCorrespondence SignalTrailVariant = "correspondence"
	TrailMatchday       SignalTrailVariant = "matchday"
	TrailOperator       SignalTrailVariant = "operator"
	TrailInterruption   SignalTrailVariant = "interruption"
)

type PageHeroProps struct {
	ImageURL      string
	ImagePosition string
	ImageWidth    int
	ImageHeight   int
	Caption       string
	Variant       HeroVariant
	ExtraClass    string
	ContentClass  string
}

type PageHeroIntroProps struct {
	Eyebrow        string
	Title          string
	Status         string
	MaxWidthClass  string
	ExtraClass     string
	TitleClass     string
	AnimatedStatus bool
}

type SectionIntroProps struct {
	Label       string
	Title       string
	Description string
	ExtraClass  string
	HeadingID   string
}

type ActionLinkProps struct {
	Href       string
	Label      string
	Variant    ActionVariant
	External   bool
	ShowArrow  bool
	ExtraClass string
	Attributes templ.Attributes
}

type ActionButtonProps struct {
	Type       string
	Label      string
	Variant    ActionVariant
	Disabled   bool
	Loading    bool
	ExtraClass string
	Attributes templ.Attributes
}

type PageCTAProps struct {
	Title     string
	Subtitle  string
	Primary   ActionLinkProps
	Secondary *ActionLinkProps
}

func pageCTAActionProps(props *ActionLinkProps, primary bool) ActionLinkProps {
	action := *props
	action.ExtraClass = mergeClasses("w-full justify-center sm:w-auto sm:min-w-[11rem]", action.ExtraClass)
	if primary {
		action.ShowArrow = true
	}
	return action
}

type FeedbackProps struct {
	ID         string
	Kind       FeedbackKind
	Title      string
	Message    string
	ExtraClass string
}

type OverflowRegionProps struct {
	ID         string
	Label      string
	Hint       string
	ExtraClass string
}

type FeatureCardProps struct {
	Icon        string
	IconName    UIIconName
	Title       string
	Description string
	Tone        Tone
	ExtraClass  string
}

type LinkPanelRowProps struct {
	Href        string
	Title       string
	Label       string
	Description string
	Icon        string
	IconName    UIIconName
	Tone        Tone
	Variant     ActionVariant
	AriaLabel   string
	External    bool
	ExtraClass  string
}

type StatCardProps struct {
	Value      string
	Label      string
	AriaLabel  string
	Suffix     string
	Icon       string
	Tone       Tone
	CornerIcon string
	ExtraClass string
	ValueClass string
	LabelClass string
	Attributes templ.Attributes
}

type StatCardGridProps struct {
	Cards     []StatCardProps
	Variant   StatGridVariant
	GridClass string
}

type SignalTrailProps struct {
	Variant    SignalTrailVariant
	ExtraClass string
}

func mergeClasses(base, extra string) string {
	if extra == "" {
		return base
	}
	return base + " " + extra
}

func heroVariantClass(variant HeroVariant) string {
	switch variant {
	case HeroIdentity:
		return "page-hero-identity"
	case HeroNarrative:
		return "page-hero-narrative"
	case HeroTimeline:
		return "page-hero-timeline"
	case HeroCatalog:
		return "page-hero-catalog"
	case HeroCaseStudy:
		return "page-hero-case-study"
	case HeroInvitation:
		return "page-hero-invitation"
	case HeroTool:
		return "page-hero-tool"
	default:
		return "page-hero-standard"
	}
}

func toneClass(tone Tone) string {
	switch tone {
	case ToneApricot:
		return "ui-tone-apricot"
	case ToneRose:
		return "ui-tone-rose"
	case ToneMint:
		return "ui-tone-mint"
	default:
		return "ui-tone-quiet"
	}
}

func actionVariantClass(variant ActionVariant) string {
	switch variant {
	case ActionPrimary:
		return "ui-action-primary"
	case ActionSecondary:
		return "ui-action-secondary"
	case ActionDanger:
		return "ui-action-danger"
	case ActionQuiet:
		return "ui-action-quiet"
	default:
		return "ui-action-quiet"
	}
}

func feedbackKindClass(kind FeedbackKind) string {
	switch kind {
	case FeedbackInfo:
		return "ui-feedback-info"
	case FeedbackSuccess:
		return "ui-feedback-success"
	case FeedbackWarning:
		return "ui-feedback-warning"
	case FeedbackError:
		return "ui-feedback-error"
	default:
		return "ui-feedback-quiet"
	}
}

func statGridVariantClass(variant StatGridVariant) string {
	switch variant {
	case StatGridHero:
		return "stat-grid-hero"
	case StatGridCompact:
		return "stat-grid-compact"
	case StatGridSummary:
		return "stat-grid-summary"
	default:
		return "stat-grid-standard"
	}
}

func signalTrailVariantClass(variant SignalTrailVariant) string {
	switch variant {
	case TrailTopology, TrailSwitchback, TrailTimeline, TrailWorkbench, TrailDossier,
		TrailFieldGuide, TrailCorrespondence, TrailMatchday, TrailOperator, TrailInterruption:
		return "signal-trail-" + string(variant)
	default:
		return "signal-trail-standard"
	}
}

func signalTrailGradientID(variant SignalTrailVariant) string {
	if signalTrailVariantClass(variant) == "signal-trail-standard" {
		return "signal-trail-gradient-standard"
	}
	return "signal-trail-gradient-" + string(variant)
}

func signalTrailGradientURL(variant SignalTrailVariant) string {
	return "url(#" + signalTrailGradientID(variant) + ")"
}

func pageHeroIntroClasses(props *PageHeroIntroProps) string {
	base := "flex flex-col gap-5"
	if props.MaxWidthClass != "" {
		base += " " + props.MaxWidthClass
	} else {
		base += " max-w-[42rem]"
	}
	return mergeClasses(base, props.ExtraClass)
}

func pageHeroPhotoStyle(props *PageHeroProps) string {
	if props.ImagePosition == "" {
		return ""
	}
	return fmt.Sprintf("--page-kit-hero-photo-position: %s;", props.ImagePosition)
}

func pageHeroImageDimension(value, fallback int) string {
	if value <= 0 {
		value = fallback
	}
	return strconv.Itoa(value)
}

func pageHeroStatusDotClasses(animated bool) string {
	if animated {
		return "page-kit-status-dot motion-safe:animate-pulse"
	}
	return "page-kit-status-dot"
}

func pageHeroClasses(props *PageHeroProps) string {
	return mergeClasses(
		"page-kit-shell page-kit-hero relative isolate overflow-hidden p-0 "+heroVariantClass(props.Variant),
		props.ExtraClass,
	)
}

func pageKitLegacyToneClass(tone Tone) string {
	switch tone {
	case ToneMint:
		return "primary"
	case ToneApricot:
		return "secondary"
	default:
		return "accent"
	}
}

func pageKitIconToneClass(tone Tone) string {
	return "page-kit-icon-frame-" + pageKitLegacyToneClass(tone)
}

func pageKitToneTextClass(tone Tone) string {
	switch tone {
	case ToneMint:
		return "text-primary-200"
	case ToneApricot:
		return "text-secondary-200"
	default:
		return "text-accent-200"
	}
}

func pageKitToneHoverBorderClass(tone Tone) string {
	switch tone {
	case ToneMint:
		return "hover:border-primary-400"
	case ToneApricot:
		return "hover:border-secondary-400"
	default:
		return "hover:border-accent-400"
	}
}

func pageKitToneSurfaceClass(tone Tone) string {
	return toneClass(tone) + " page-kit-tone-" + pageKitLegacyToneClass(tone)
}

func linkPanelRowAriaLabel(props *LinkPanelRowProps) string {
	label := props.AriaLabel
	if props.External {
		if label == "" {
			label = props.Title
		}
		label += " (opens in a new tab)"
	}
	return label
}

func statCardValueClasses(extra string) string {
	return mergeClasses("text-gradient-brand block font-mono text-4xl font-extrabold", extra)
}

func statCardLabelClasses(extra string) string {
	return mergeClasses("mt-2 block font-mono text-xs font-medium uppercase tracking-wide text-copy-muted", extra)
}

func statCardDisplayValue(value, suffix string) string {
	if suffix == "" {
		return value
	}
	return value + suffix
}

func statCardIsNumeric(value string) bool {
	_, err := strconv.Atoi(value)
	return err == nil
}

func statCardGridClasses(props StatCardGridProps) string {
	base := "stats-grid grid grid-cols-2 gap-3 sm:gap-4 xl:grid-cols-4"
	if props.GridClass != "" {
		base = props.GridClass
	}
	return mergeClasses(base, statGridVariantClass(props.Variant))
}

func actionClasses(variant ActionVariant, extra string) string {
	base := "page-kit-action " + actionVariantClass(variant)
	if variant == ActionPrimary {
		base += " page-kit-action-primary"
	}
	return mergeClasses(base, extra)
}

func actionButtonType(value string) string {
	if value == "" {
		return "button"
	}
	return value
}

func feedbackARIArole(kind FeedbackKind) string {
	if kind == FeedbackError {
		return "alert"
	}
	return "status"
}
