package portfolio

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
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
		slog.Default().With(slog.String("component", "portfolio_data")).Error(
			"embedded portfolio data could not be unmarshaled",
			slog.String("file_name", name),
			slog.Any("error", err),
		)
		panic(fmt.Sprintf("failed to unmarshal %s: %v", name, err))
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
	return append([]types.SkillCategory(nil), skillsData...)
}

func ProjectsData() []types.Project {
	projectsDataOnce.Do(func() {
		projectsData = mustUnmarshal[[]types.Project](projectsJSON, "projects.json")
	})
	return append([]types.Project(nil), projectsData...)
}
