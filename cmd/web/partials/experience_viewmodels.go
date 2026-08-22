package partials

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"portfolio/types"
)

type experienceOverview struct {
	CurrentRole           types.Experience
	TotalRoles            int
	TotalCompanies        int
	TotalTechnologies     int
	CareerStartYear       int
	CareerSpanYears       int
	SpotlightTechnologies []string
	Capabilities          []experienceCapabilityCount
}

type experienceCapabilityCount struct {
	Code        string
	Label       string
	Description string
	Count       int
}

type experienceStage struct {
	ID          string
	Title       string
	Range       string
	Summary     string
	IsCurrent   bool
	Experiences []types.Experience
}

type rankedLabel struct {
	Name  string
	Count int
}

func buildExperienceOverview(experiences []types.Experience) experienceOverview {
	overview := experienceOverview{TotalRoles: len(experiences)}
	if len(experiences) == 0 {
		return overview
	}

	overview.CurrentRole = experiences[0]
	companySet := make(map[string]struct{})
	techFrequency := make(map[string]int)
	skillFrequency := make(map[string]int)
	startYear := time.Now().Year()

	for index := range experiences {
		experience := &experiences[index]
		if experienceIsMoreRecent(experience, &overview.CurrentRole) {
			overview.CurrentRole = *experience
		}
		if company := strings.TrimSpace(experience.Company); company != "" {
			companySet[company] = struct{}{}
		}

		if year := experienceStartYear(experience.Duration); year > 0 && year < startYear {
			startYear = year
		}

		for _, technology := range experience.Technologies {
			technology = strings.TrimSpace(technology)
			if technology == "" {
				continue
			}
			techFrequency[technology]++
		}

		for _, code := range splitSkillAreas(experience.SkillAreas) {
			skillFrequency[code]++
		}
	}

	overview.TotalCompanies = len(companySet)
	overview.TotalTechnologies = len(techFrequency)
	overview.CareerStartYear = startYear
	overview.CareerSpanYears = max(1, time.Now().Year()-startYear)
	overview.SpotlightTechnologies = topRankedLabels(techFrequency, 8)
	overview.Capabilities = capabilityCounts(skillFrequency)

	return overview
}

func buildCareerStages(experiences []types.Experience) []experienceStage {
	baseStages := []experienceStage{
		{
			ID:      "foundation",
			Title:   "Foundation",
			Range:   "2012–2016",
			Summary: "Built the service mindset, documentation habits, and support discipline that still shape every platform decision.",
		},
		{
			ID:      "systems-growth",
			Title:   "Systems Growth",
			Range:   "2016–2021",
			Summary: "Expanded from endpoint and directory operations into infrastructure engineering, automation, and enterprise systems ownership.",
		},
		{
			ID:        "cloud-leadership",
			Title:     "Cloud Leadership",
			Range:     "2021–Present",
			Summary:   "Shifted into regulated environments, cloud platforms, GitOps, and infrastructure leadership across modern delivery pipelines.",
			IsCurrent: true,
		},
	}

	stages := make(map[string]*experienceStage, len(baseStages))
	for i := range baseStages {
		stages[baseStages[i].ID] = &baseStages[i]
	}

	chronological := append([]types.Experience(nil), experiences...)
	sort.SliceStable(chronological, func(i, j int) bool {
		leftYear := experienceStartYear(chronological[i].Duration)
		rightYear := experienceStartYear(chronological[j].Duration)
		if leftYear != rightYear {
			return leftYear < rightYear
		}
		if chronological[i].ID != chronological[j].ID {
			return chronological[i].ID > chronological[j].ID
		}
		return chronological[i].Position < chronological[j].Position
	})

	for index := range chronological {
		experience := &chronological[index]
		stageID := stageIDForExperience(experience)
		if stage, ok := stages[stageID]; ok {
			stage.Experiences = append(stage.Experiences, *experience)
		}
	}

	result := make([]experienceStage, 0, len(baseStages))
	for _, stage := range baseStages {
		if len(stage.Experiences) == 0 {
			continue
		}
		result = append(result, stage)
	}

	return result
}

func experienceIsMoreRecent(candidate, current *types.Experience) bool {
	candidateYear := experienceStartYear(candidate.Duration)
	currentYear := experienceStartYear(current.Duration)
	if candidateYear != currentYear {
		return candidateYear > currentYear
	}
	candidatePresent := strings.Contains(strings.ToLower(candidate.Duration), "present")
	currentPresent := strings.Contains(strings.ToLower(current.Duration), "present")
	if candidatePresent != currentPresent {
		return candidatePresent
	}
	if candidate.ID != current.ID {
		return candidate.ID < current.ID
	}
	return candidate.Position < current.Position
}

func splitSkillAreas(raw string) []string {
	parts := strings.Split(raw, ",")
	areas := make([]string, 0, len(parts))
	for _, part := range parts {
		normalized := strings.ToLower(strings.TrimSpace(part))
		if normalized == "" {
			continue
		}
		areas = append(areas, normalized)
	}
	return areas
}

func experienceStartYear(duration string) int {
	fields := strings.Fields(duration)
	if len(fields) == 0 {
		return 0
	}

	year, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0
	}

	return year
}

func capabilityCounts(frequency map[string]int) []experienceCapabilityCount {
	definitions := []experienceCapabilityCount{
		{
			Code:        "cloud",
			Label:       "Cloud Platforms",
			Description: "Infrastructure design, migration, and modernization.",
		},
		{
			Code:        "automation",
			Label:       "Automation & IaC",
			Description: "Repeatable delivery through scripting, pipelines, and configuration as code.",
		},
		{
			Code:        "security",
			Label:       "Security & Compliance",
			Description: "Controls, policy enforcement, and operational hardening.",
		},
		{
			Code:        "systems",
			Label:       "Systems Engineering",
			Description: "Platform ownership across endpoints, identity, messaging, and operations.",
		},
		{
			Code:        "devops",
			Label:       "DevOps Delivery",
			Description: "Release flow, CI/CD, GitOps, and deployment enablement.",
		},
		{
			Code:        "scripting",
			Label:       "Scripting",
			Description: "PowerShell, Bash, Python, and Go used to remove manual work.",
		},
	}

	result := make([]experienceCapabilityCount, 0, len(definitions))
	for _, definition := range definitions {
		definition.Count = frequency[definition.Code]
		if definition.Count == 0 {
			continue
		}
		result = append(result, definition)
	}

	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Count == result[j].Count {
			return result[i].Label < result[j].Label
		}
		return result[i].Count > result[j].Count
	})

	return result
}

func topRankedLabels(items map[string]int, limit int) []string {
	ranked := make([]rankedLabel, 0, len(items))
	for name, count := range items {
		ranked = append(ranked, rankedLabel{Name: name, Count: count})
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Count == ranked[j].Count {
			return ranked[i].Name < ranked[j].Name
		}
		return ranked[i].Count > ranked[j].Count
	})

	if limit > 0 && len(ranked) > limit {
		ranked = ranked[:limit]
	}

	labels := make([]string, 0, len(ranked))
	for _, item := range ranked {
		labels = append(labels, item.Name)
	}

	return labels
}

func stageIDForExperience(experience *types.Experience) string {
	startYear := experienceStartYear(experience.Duration)
	switch {
	case startYear >= 2021:
		return "cloud-leadership"
	case startYear >= 2017:
		return "systems-growth"
	default:
		return "foundation"
	}
}
