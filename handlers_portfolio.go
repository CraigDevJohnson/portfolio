package main

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"portfolio/components/pages"
	"portfolio/components/partials"
	"portfolio/types"
)

func homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	err := pages.Home(pages.HomeProps{
		Name:               "Craig Johnson",
		Role:               "Cloud Engineer Principal",
		AvatarURL:          gravatarURL("gravatar@craigdevjohnson.com", 275),
		Description:        "Hi there! I'm a seasoned System Engineer with over a decade of experience in system engineering, administration, and optimization. I specialize in designing, implementing, and maintaining various systems and applications, thriving on performance optimization and security enhancement. I enjoy collaborating with application owners and software engineers to deliver innovative solutions and streamline processes through automation. I'm passionate about modernizing infrastructure and documenting critical processes. Let's connect and share our tech journeys!",
		YearsInTech:        time.Now().Year() - careerStartYear,
		Certifications:     10,
		AutomationProjects: "100",
	}).Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func aboutHandler(w http.ResponseWriter, r *http.Request) {
	props := pages.AboutProps{
		YearsInTech:    time.Now().Year() - careerStartYear,
		Certifications: 10,
		TechUsed:       30,
		CupsOfCoffee:   "∞",
	}
	err := pages.About(props).Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func experienceHandler(w http.ResponseWriter, r *http.Request) {
	err := pages.Experience().Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func experienceTimelineHandler(w http.ResponseWriter, r *http.Request) {
	props := partials.ExperienceTimelineProps{
		Experiences: experienceData(),
	}
	err := partials.ExperienceTimeline(props).Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func getFeaturedSkills(categories []types.SkillCategory) []types.Skill {
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

func skillsHandler(w http.ResponseWriter, r *http.Request) {
	err := pages.Skills().Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func skillsGridHandler(w http.ResponseWriter, r *http.Request) {
	categories := skillsData()
	props := partials.SkillsGridProps{
		Categories:     categories,
		FeaturedSkills: getFeaturedSkills(categories),
	}
	err := partials.SkillsGrid(props).Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func skillsFilteredHandler(w http.ResponseWriter, r *http.Request) {
	categories := skillsData()
	activeCategory := r.URL.Query().Get("category")
	activeProficiency := r.URL.Query().Get("proficiency")

	props := partials.SkillsFilterableProps{
		Categories:        categories,
		ActiveCategory:    activeCategory,
		ActiveProficiency: activeProficiency,
	}
	err := partials.SkillsFilterableSection(props).Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func skillsDetailHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid skill id", http.StatusBadRequest)
		return
	}

	categories := skillsData()
	var found types.Skill
	var foundCategory string
	for _, cat := range categories {
		for i := range cat.Skills {
			if cat.Skills[i].ID == id {
				found = cat.Skills[i]
				foundCategory = cat.Name
				break
			}
		}
		if found.Name != "" {
			break
		}
	}

	if found.Name == "" {
		http.Error(w, "skill not found", http.StatusNotFound)
		return
	}

	found.Category = foundCategory
	props := partials.SkillDetailProps{
		Skill: found,
	}
	err = partials.SkillDetail(props).Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func projectsHandler(w http.ResponseWriter, r *http.Request) {
	err := pages.Projects().Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func projectsGridHandler(w http.ResponseWriter, r *http.Request) {
	props := partials.ProjectsGridProps{
		Projects: projectsData(),
	}
	err := partials.ProjectsGrid(props).Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func educationHandler(w http.ResponseWriter, r *http.Request) {
	props := pages.EducationProps{
		TotalCerts:      10,
		Providers:       5,
		YearsCertifying: time.Now().Year() - 2018,
	}
	if err := pages.Education(props).Render(context.Background(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func contactHandler(w http.ResponseWriter, r *http.Request) {
	err := pages.Contact().Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
