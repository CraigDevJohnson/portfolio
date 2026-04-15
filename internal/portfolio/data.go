package portfolio

import (
	_ "embed"
	"encoding/json"
	"log"
	"sync"

	"portfolio/types"
)

var (
	//go:embed data/experience.json
	experienceJSON []byte
	//go:embed data/skills.json
	skillsJSON []byte
	//go:embed data/projects.json
	projectsJSON []byte

	experienceDataOnce sync.Once
	experienceData     []types.Experience
	skillsDataOnce     sync.Once
	skillsData         []types.SkillCategory
	projectsDataOnce   sync.Once
	projectsData       []types.Project
)

func mustUnmarshal[T any](data []byte, name string) T {
	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		log.Fatalf("failed to unmarshal %s: %v", name, err)
	}
	return result
}

func ExperienceData() []types.Experience {
	experienceDataOnce.Do(func() {
		experienceData = mustUnmarshal[[]types.Experience](experienceJSON, "experience.json")
	})
	return append([]types.Experience(nil), experienceData...)
}

func SkillsData() []types.SkillCategory {
	skillsDataOnce.Do(func() {
		skillsData = mustUnmarshal[[]types.SkillCategory](skillsJSON, "skills.json")
	})
	return cloneSkillCategories(skillsData)
}

func ProjectsData() []types.Project {
	projectsDataOnce.Do(func() {
		projectsData = mustUnmarshal[[]types.Project](projectsJSON, "projects.json")
	})
	return append([]types.Project(nil), projectsData...)
}

func cloneSkillCategories(categories []types.SkillCategory) []types.SkillCategory {
	cloned := make([]types.SkillCategory, len(categories))
	for i := range categories {
		cloned[i] = categories[i]
		cloned[i].Skills = append([]types.Skill(nil), categories[i].Skills...)
	}
	return cloned
}
