package portfolio

import (
	"net/http"
	"strconv"
	"time"

	"github.com/a-h/templ"

	"portfolio/cmd/web/pages"
	"portfolio/cmd/web/partials"
	"portfolio/types"
)

func renderComponent(w http.ResponseWriter, r *http.Request, component templ.Component) {
	if err := component.Render(r.Context(), w); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func HomeHandler(w http.ResponseWriter, r *http.Request, careerStartYear int) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	renderComponent(w, r, pages.Home(pages.HomeProps{
		Name:               "Craig Johnson",
		Role:               "Cloud Engineer Principal",
		AvatarURL:          GravatarURL("gravatar@craigdevjohnson.com", 275),
		Description:        "Hi there! I'm a seasoned System Engineer with over a decade of experience in system engineering, administration, and optimization. I specialize in designing, implementing, and maintaining various systems and applications, thriving on performance optimization and security enhancement. I enjoy collaborating with application owners and software engineers to deliver innovative solutions and streamline processes through automation. I'm passionate about modernizing infrastructure and documenting critical processes. Let's connect and share our tech journeys!",
		YearsInTech:        time.Now().Year() - careerStartYear,
		Certifications:     10,
		AutomationProjects: "100",
	}))
}

func AboutHandler(w http.ResponseWriter, r *http.Request, careerStartYear int) {
	props := pages.AboutProps{
		YearsInTech:    time.Now().Year() - careerStartYear,
		Certifications: 10,
		TechUsed:       30,
		CupsOfCoffee:   "∞",
	}
	renderComponent(w, r, pages.About(props))
}

func ExperienceHandler(w http.ResponseWriter, r *http.Request) {
	renderComponent(w, r, pages.Experience())
}

func ExperienceTimelineHandler(w http.ResponseWriter, r *http.Request) {
	props := partials.ExperienceTimelineProps{Experiences: ExperienceData()}
	renderComponent(w, r, partials.ExperienceTimeline(props))
}

func featuredSkills(categories []types.SkillCategory) []types.Skill {
	var featured []types.Skill
	for _, category := range categories {
		for i := range category.Skills {
			if category.Skills[i].Featured {
				category.Skills[i].Category = category.Name
				featured = append(featured, category.Skills[i])
			}
		}
	}
	return featured
}

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

func SkillsHandler(w http.ResponseWriter, r *http.Request) {
	renderComponent(w, r, pages.Skills())
}

func SkillsGridHandler(w http.ResponseWriter, r *http.Request) {
	categories := SkillsData()
	props := partials.SkillsGridProps{
		Categories:     categories,
		FeaturedSkills: featuredSkills(categories),
	}
	renderComponent(w, r, partials.SkillsGrid(props))
}

func SkillsFilteredHandler(w http.ResponseWriter, r *http.Request) {
	props := partials.SkillsFilterableProps{
		Categories:        SkillsData(),
		ActiveCategory:    r.URL.Query().Get("category"),
		ActiveProficiency: r.URL.Query().Get("proficiency"),
	}
	renderComponent(w, r, partials.SkillsFilterableSection(props))
}

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

func ProjectsHandler(w http.ResponseWriter, r *http.Request) {
	renderComponent(w, r, pages.Projects())
}

func ProjectsGridHandler(w http.ResponseWriter, r *http.Request) {
	props := partials.ProjectsGridProps{Projects: ProjectsData()}
	renderComponent(w, r, partials.ProjectsGrid(props))
}

func EducationHandler(w http.ResponseWriter, r *http.Request) {
	props := pages.EducationProps{
		TotalCerts:      10,
		Providers:       5,
		YearsCertifying: time.Now().Year() - 2018,
	}
	renderComponent(w, r, pages.Education(props))
}

func ContactHandler(w http.ResponseWriter, r *http.Request) {
	renderComponent(w, r, pages.Contact())
}
