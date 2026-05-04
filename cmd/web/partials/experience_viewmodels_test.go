package partials

import (
	"testing"

	"portfolio/types"
)

func TestBuildExperienceOverview(t *testing.T) {
	experiences := []types.Experience{
		{Position: "Principal", Company: "Org A", Duration: "2022 – Present", Technologies: []string{"AWS", "Go"}, SkillAreas: "cloud,automation,devops"},
		{Position: "Engineer", Company: "Org B", Duration: "2018 – 2022", Technologies: []string{"AWS", "Terraform"}, SkillAreas: "cloud,automation"},
		{Position: "Analyst", Company: "Org C", Duration: "2012 – 2018", Technologies: []string{"Windows"}, SkillAreas: "systems"},
	}

	overview := buildExperienceOverview(experiences)

	if overview.TotalRoles != 3 {
		t.Fatalf("TotalRoles = %d, want 3", overview.TotalRoles)
	}

	if overview.TotalCompanies != 3 {
		t.Fatalf("TotalCompanies = %d, want 3", overview.TotalCompanies)
	}

	if overview.CareerStartYear != 2012 {
		t.Fatalf("CareerStartYear = %d, want 2012", overview.CareerStartYear)
	}

	if overview.CurrentRole.Position != "Principal" {
		t.Fatalf("CurrentRole.Position = %q, want %q", overview.CurrentRole.Position, "Principal")
	}

	capabilityCounts := make(map[string]int, len(overview.Capabilities))
	for _, capability := range overview.Capabilities {
		capabilityCounts[capability.Code] = capability.Count
	}

	if capabilityCounts["cloud"] != 2 {
		t.Fatalf("cloud capability count = %d, want 2", capabilityCounts["cloud"])
	}

	if capabilityCounts["automation"] != 2 {
		t.Fatalf("automation capability count = %d, want 2", capabilityCounts["automation"])
	}

	if len(overview.SpotlightTechnologies) == 0 || overview.SpotlightTechnologies[0] != "AWS" {
		t.Fatalf("SpotlightTechnologies = %#v, want AWS to appear first", overview.SpotlightTechnologies)
	}
}

func TestBuildCareerStages(t *testing.T) {
	experiences := []types.Experience{
		{Position: "Principal", Duration: "2022 – Present"},
		{Position: "System Administrator", Duration: "2021 – 2022"},
		{Position: "Systems Engineer", Duration: "2018 – 2021"},
		{Position: "Service Desk Analyst", Duration: "2012 – 2016"},
	}

	stages := buildCareerStages(experiences)

	if len(stages) != 3 {
		t.Fatalf("len(stages) = %d, want 3", len(stages))
	}

	if stages[0].ID != "foundation" {
		t.Fatalf("stages[0].ID = %q, want %q", stages[0].ID, "foundation")
	}

	if len(stages[2].Experiences) != 2 {
		t.Fatalf("len(stages[2].Experiences) = %d, want 2", len(stages[2].Experiences))
	}

	if stages[1].ID != "systems-growth" {
		t.Fatalf("stages[1].ID = %q, want %q", stages[1].ID, "systems-growth")
	}
}
