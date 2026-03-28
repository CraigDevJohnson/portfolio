// Portfolio handler compatibility wrappers.
package app

import (
	"net/http"

	"portfolio/internal/config"
	internalportfolio "portfolio/internal/portfolio"
	"portfolio/types"
)

func (app *App) homeHandler(w http.ResponseWriter, r *http.Request) {
	internalportfolio.HomeHandler(w, r, config.CareerStartYear)
}

func (app *App) aboutHandler(w http.ResponseWriter, r *http.Request) {
	internalportfolio.AboutHandler(w, config.CareerStartYear)
}

func (app *App) experienceHandler(w http.ResponseWriter, r *http.Request) {
	internalportfolio.ExperienceHandler(w)
}

func (app *App) experienceTimelineHandler(w http.ResponseWriter, r *http.Request) {
	internalportfolio.ExperienceTimelineHandler(w)
}

func getFeaturedSkills(categories []types.SkillCategory) []types.Skill {
	return internalportfolio.GetFeaturedSkills(categories)
}

func (app *App) skillsHandler(w http.ResponseWriter, r *http.Request) {
	internalportfolio.SkillsHandler(w)
}

func (app *App) skillsGridHandler(w http.ResponseWriter, r *http.Request) {
	internalportfolio.SkillsGridHandler(w)
}

func (app *App) skillsFilteredHandler(w http.ResponseWriter, r *http.Request) {
	internalportfolio.SkillsFilteredHandler(w, r)
}

func (app *App) skillsDetailHandler(w http.ResponseWriter, r *http.Request) {
	internalportfolio.SkillsDetailHandler(w, r)
}

func (app *App) projectsHandler(w http.ResponseWriter, r *http.Request) {
	internalportfolio.ProjectsHandler(w)
}

func (app *App) projectsGridHandler(w http.ResponseWriter, r *http.Request) {
	internalportfolio.ProjectsGridHandler(w)
}

func (app *App) educationHandler(w http.ResponseWriter, r *http.Request) {
	internalportfolio.EducationHandler(w)
}

func (app *App) contactHandler(w http.ResponseWriter, r *http.Request) {
	internalportfolio.ContactHandler(w)
}
