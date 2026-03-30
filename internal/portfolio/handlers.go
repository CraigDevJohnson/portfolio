package portfolio

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"portfolio/cmd/web/pages"
	"portfolio/cmd/web/partials"
	"portfolio/types"
)

func HomeHandler(w http.ResponseWriter, r *http.Request, careerStartYear int) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if err := pages.Home(pages.HomeProps{
		Name:               "Craig Johnson",
		Role:               "Cloud Engineer Principal",
		AvatarURL:          GravatarURL("gravatar@craigdevjohnson.com", 275),
		Description:        "Hi there! I'm a seasoned System Engineer with over a decade of experience in system engineering, administration, and optimization. I specialize in designing, implementing, and maintaining various systems and applications, thriving on performance optimization and security enhancement. I enjoy collaborating with application owners and software engineers to deliver innovative solutions and streamline processes through automation. I'm passionate about modernizing infrastructure and documenting critical processes. Let's connect and share our tech journeys!",
		YearsInTech:        time.Now().Year() - careerStartYear,
		Certifications:     10,
		AutomationProjects: "100",
	}).Render(context.Background(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func AboutHandler(w http.ResponseWriter, careerStartYear int) {
	props := pages.AboutProps{
		YearsInTech:    time.Now().Year() - careerStartYear,
		Certifications: 10,
		TechUsed:       30,
		CupsOfCoffee:   "∞",
	}
	if err := pages.About(props).Render(context.Background(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func ExperienceHandler(w http.ResponseWriter) {
	if err := pages.Experience().Render(context.Background(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func ExperienceTimelineHandler(w http.ResponseWriter) {
	props := partials.ExperienceTimelineProps{Experiences: ExperienceData()}
	if err := partials.ExperienceTimeline(props).Render(context.Background(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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

func SkillsHandler(w http.ResponseWriter) {
	if err := pages.Skills().Render(context.Background(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func SkillsGridHandler(w http.ResponseWriter) {
	categories := SkillsData()
	props := partials.SkillsGridProps{
		Categories:     categories,
		FeaturedSkills: featuredSkills(categories),
	}
	if err := partials.SkillsGrid(props).Render(context.Background(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func SkillsFilteredHandler(w http.ResponseWriter, r *http.Request) {
	props := partials.SkillsFilterableProps{
		Categories:        SkillsData(),
		ActiveCategory:    r.URL.Query().Get("category"),
		ActiveProficiency: r.URL.Query().Get("proficiency"),
	}
	if err := partials.SkillsFilterableSection(props).Render(context.Background(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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
	if err := partials.SkillDetail(partials.SkillDetailProps{Skill: found}).Render(context.Background(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func ProjectsHandler(w http.ResponseWriter) {
	if err := pages.Projects().Render(context.Background(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func ProjectsGridHandler(w http.ResponseWriter) {
	props := partials.ProjectsGridProps{Projects: ProjectsData()}
	if err := partials.ProjectsGrid(props).Render(context.Background(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func EducationHandler(w http.ResponseWriter) {
	props := pages.EducationProps{
		TotalCerts:      10,
		Providers:       5,
		YearsCertifying: time.Now().Year() - 2018,
	}
	if err := pages.Education(props).Render(context.Background(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func ContactHandler(w http.ResponseWriter) {
	if err := pages.Contact().Render(context.Background(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
