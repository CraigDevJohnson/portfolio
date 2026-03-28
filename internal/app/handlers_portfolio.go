// Portfolio handler compatibility wrappers.
package app

import (
	"net/http"

	internalportfolio "portfolio/internal/portfolio"
	"portfolio/types"
)

func homeHandler(w http.ResponseWriter, r *http.Request) {
	internalportfolio.HomeHandler(w, r, careerStartYear)
}

func aboutHandler(w http.ResponseWriter, r *http.Request) {
	internalportfolio.AboutHandler(w, careerStartYear)
}

func experienceHandler(w http.ResponseWriter, r *http.Request) {
	internalportfolio.ExperienceHandler(w)
}

func experienceTimelineHandler(w http.ResponseWriter, r *http.Request) {
	internalportfolio.ExperienceTimelineHandler(w)
}

func getFeaturedSkills(categories []types.SkillCategory) []types.Skill {
	return internalportfolio.GetFeaturedSkills(categories)
}

func skillsHandler(w http.ResponseWriter, r *http.Request) {
	internalportfolio.SkillsHandler(w)
}

func skillsGridHandler(w http.ResponseWriter, r *http.Request) {
	internalportfolio.SkillsGridHandler(w)
}

func skillsFilteredHandler(w http.ResponseWriter, r *http.Request) {
	internalportfolio.SkillsFilteredHandler(w, r)
}

func skillsDetailHandler(w http.ResponseWriter, r *http.Request) {
	internalportfolio.SkillsDetailHandler(w, r)
}

func projectsHandler(w http.ResponseWriter, r *http.Request) {
	internalportfolio.ProjectsHandler(w)
}

func projectsGridHandler(w http.ResponseWriter, r *http.Request) {
	internalportfolio.ProjectsGridHandler(w)
}

func educationHandler(w http.ResponseWriter, r *http.Request) {
	internalportfolio.EducationHandler(w)
}

func contactHandler(w http.ResponseWriter, r *http.Request) {
	internalportfolio.ContactHandler(w)
}
