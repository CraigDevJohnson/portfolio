// Portfolio data compatibility wrappers.
package app

import (
	internalportfolio "portfolio/internal/portfolio"
	"portfolio/types"
)

func gravatarURL(email string, size int) string {
	return internalportfolio.GravatarURL(email, size)
}

func experienceData() []types.Experience {
	return internalportfolio.ExperienceData()
}

func skillsData() []types.SkillCategory {
	return internalportfolio.SkillsData()
}

func projectsData() []types.Project {
	return internalportfolio.ProjectsData()
}
