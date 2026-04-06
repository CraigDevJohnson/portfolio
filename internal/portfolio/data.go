package portfolio

import (
	_ "embed"
	"encoding/json"
	"log"

	"portfolio/types"
)

var (
	//go:embed data/experience.json
	experienceJSON []byte
	//go:embed data/skills.json
	skillsJSON []byte
	//go:embed data/projects.json
	projectsJSON []byte
)

func mustUnmarshal[T any](data []byte, name string) T {
	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		log.Fatalf("failed to unmarshal %s: %v", name, err)
	}
	return result
}

func ExperienceData() []types.Experience {
	return mustUnmarshal[[]types.Experience](experienceJSON, "experience.json")
}

func SkillsData() []types.SkillCategory {
	return mustUnmarshal[[]types.SkillCategory](skillsJSON, "skills.json")
}

func ProjectsData() []types.Project {
	return mustUnmarshal[[]types.Project](projectsJSON, "projects.json")
}
