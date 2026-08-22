package portfolio

import (
	"crypto/md5"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"

	"portfolio/cmd/web/pages"
	"portfolio/cmd/web/partials"
	"portfolio/internal/logging"
	"portfolio/types"
)

const (
	displayName            = "Craig Johnson"
	displayRole            = "Cloud Engineer Principal"
	gravatarEmail          = "gravatar@craigdevjohnson.com"
	gravatarSize           = 275
	certificationCount     = 10
	techUsedCount          = 30
	coffeeCount            = "∞"
	educationProviderCount = 5
	educationStartYear     = 2018
)

// gravatarURL returns the Gravatar image URL for the given email and size.
func gravatarURL(email string, size int) string {
	email = strings.TrimSpace(strings.ToLower(email))
	hash := md5.Sum([]byte(email))
	return "https://www.gravatar.com/avatar/" + hex.EncodeToString(hash[:]) + "?s=" + strconv.Itoa(size)
}

// renderComponent renders a templ component and returns a generic 500 on failure.
func renderComponent(w http.ResponseWriter, r *http.Request, component templ.Component) {
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	if err := component.Render(r.Context(), w); err != nil {
		logging.WithContext(logging.Component("portfolio"), r.Context()).Error(
			"portfolio render failed",
			slog.Any("error", err),
			slog.String("path", r.URL.Path),
		)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

// HomeHandler renders the home page.
func HomeHandler(w http.ResponseWriter, r *http.Request, careerStartYear int) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	renderComponent(w, r, pages.Home(pages.HomeProps{
		Name:               displayName,
		Role:               displayRole,
		AvatarURL:          gravatarURL(gravatarEmail, gravatarSize),
		Description:        "I design and modernize cloud platforms with a focus on automation, security, and operational clarity. Across more than a decade in systems engineering, the throughline has stayed the same: make critical infrastructure easier to change, understand, and trust.",
		YearsInTech:        time.Now().Year() - careerStartYear,
		Certifications:     certificationCount,
		AutomationProjects: "100",
	}))
}

// AboutHandler renders the about page.
func AboutHandler(w http.ResponseWriter, r *http.Request, careerStartYear int) {
	props := pages.AboutProps{
		YearsInTech:    time.Now().Year() - careerStartYear,
		Certifications: certificationCount,
		TechUsed:       techUsedCount,
		CupsOfCoffee:   coffeeCount,
	}
	renderComponent(w, r, pages.About(props))
}

// ExperienceHandler renders the experience page with default or selected kit.
func ExperienceHandler(w http.ResponseWriter, r *http.Request) {
	renderComponent(w, r, pages.Experience(pages.ExperienceProps{
		Experiences: ExperienceData(),
	}))
}

// findSkillByID looks up a skill and returns the matching category name.
func findSkillByID(categories []types.SkillCategory, id int) (types.Skill, string, bool) {
	for _, category := range categories {
		for i := range category.Skills {
			if category.Skills[i].ID != id {
				continue
			}

			return category.Skills[i], category.Name, true
		}
	}

	return types.Skill{}, "", false
}

// SkillsHandler renders the skills page with its primary content server-side.
func SkillsHandler(w http.ResponseWriter, r *http.Request) {
	categories := SkillsData()
	props := buildSkillsPageProps(categories, r.URL.Query())
	setSkillsResponseVary(w)
	if isHTMXRequest(r) && !isHTMXHistoryRestore(r) {
		w.Header().Set("HX-Push-Url", props.Grid.StateURL)
		renderComponent(w, r, partials.SkillsFilterFragment(props.Grid))
		return
	}
	renderComponent(w, r, pages.Skills(props))
}

// SkillsFilteredHandler renders the filterable skills section.
func SkillsFilteredHandler(w http.ResponseWriter, r *http.Request) {
	props := buildSkillsPageProps(SkillsData(), r.URL.Query())
	setSkillsResponseVary(w)
	w.Header().Set("HX-Push-Url", props.Grid.StateURL)
	renderComponent(w, r, partials.SkillsFilterFragment(props.Grid))
}

func isHTMXRequest(r *http.Request) bool {
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("HX-Request")), "true")
}

func isHTMXHistoryRestore(r *http.Request) bool {
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("HX-History-Restore-Request")), "true")
}

func setSkillsResponseVary(w http.ResponseWriter) {
	w.Header().Add("Vary", "HX-Request")
	w.Header().Add("Vary", "HX-History-Restore-Request")
}

// SkillsDetailHandler renders the detail fragment for a single skill.
func SkillsDetailHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	if err != nil {
		http.Error(w, "invalid skill id", http.StatusBadRequest)
		return
	}

	categories := SkillsData()
	found, foundCategory, ok := findSkillByID(categories, id)
	if !ok {
		http.Error(w, "skill not found", http.StatusNotFound)
		return
	}

	found.Category = foundCategory
	renderComponent(w, r, partials.SkillDetail(partials.SkillDetailProps{Skill: found}))
}

// ProjectsHandler renders the projects page shell.
func ProjectsHandler(w http.ResponseWriter, r *http.Request) {
	projects := ProjectsData()

	renderComponent(w, r, pages.Projects(pages.ProjectsProps{
		Projects: projects,
	}))
}

// EducationHandler renders the education page.
func EducationHandler(w http.ResponseWriter, r *http.Request) {
	props := pages.EducationProps{
		TotalCerts:      certificationCount,
		Providers:       educationProviderCount,
		YearsCertifying: time.Now().Year() - educationStartYear,
	}
	renderComponent(w, r, pages.Education(props))
}

// ContactHandler renders the contact page.
func ContactHandler(w http.ResponseWriter, r *http.Request) {
	renderComponent(w, r, pages.Contact())
}
