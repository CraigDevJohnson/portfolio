package partials

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"portfolio/types"
)

func TestExperienceKitStagesRendersIntegratedOrientationPanel(t *testing.T) {
	html := renderComponent(t, ExperienceKitStages(ExperienceKitStagesProps{Experiences: task5CareerFixture()}))
	for _, marker := range []string{
		`class="page-section-tight experience-orientation-section"`,
		`class="experience-orientation"`,
		`class="page-kit-panel-strong experience-technology-panel experience-technology-strip"`,
		`class="experience-technology-chips"`,
		`Tools that followed the work`,
		`These tools recur across roles`,
	} {
		if !strings.Contains(html, marker) {
			t.Errorf("Experience technology panel does not contain %q", marker)
		}
	}
	if got := strings.Count(html, `data-recurring-technology`); got != 8 {
		t.Fatalf("recurring technology count = %d, want 8", got)
	}
	wantOrder := []string{"PowerShell", "AD DS", "Windows", "AWS", "Ansible", "Azure", "Bash", "Go"}
	lastIndex := -1
	for _, technology := range wantOrder {
		index := strings.Index(html, ">"+technology+"</span>")
		if index <= lastIndex {
			t.Fatalf("technology %q index = %d after %d; markup order is wrong", technology, index, lastIndex)
		}
		lastIndex = index
	}
}

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
	experiences := task5CareerFixture()

	stages := buildCareerStages(experiences)

	wantStages := []struct {
		id      string
		title   string
		range_  string
		current bool
		roleIDs []int
	}{
		{id: "foundation", title: "Foundation", range_: "2012–2016", roleIDs: []int{7, 6}},
		{id: "systems-growth", title: "Systems Growth", range_: "2016–2021", roleIDs: []int{5, 4, 3}},
		{id: "cloud-leadership", title: "Cloud Leadership", range_: "2021–Present", current: true, roleIDs: []int{2, 1}},
	}
	if len(stages) != len(wantStages) {
		t.Fatalf("len(stages) = %d, want %d", len(stages), len(wantStages))
	}
	for index, want := range wantStages {
		stage := stages[index]
		if stage.ID != want.id || stage.Title != want.title || stage.Range != want.range_ {
			t.Errorf("stages[%d] metadata = (%q, %q, %q), want (%q, %q, %q)", index, stage.ID, stage.Title, stage.Range, want.id, want.title, want.range_)
		}
		if stage.IsCurrent != want.current {
			t.Errorf("stages[%d].IsCurrent = %t, want %t", index, stage.IsCurrent, want.current)
		}
		if got := task5RoleIDs(stage.Experiences); !reflect.DeepEqual(got, want.roleIDs) {
			t.Errorf("stages[%d] role IDs = %v, want %v", index, got, want.roleIDs)
		}
	}
	if got := task5FlattenedRoleIDs(stages); !reflect.DeepEqual(got, []int{7, 6, 5, 4, 3, 2, 1}) {
		t.Fatalf("flattened role IDs = %v, want chronological [7 6 5 4 3 2 1]", got)
	}
}

func TestBuildCareerStagesIgnoresInputOrder(t *testing.T) {
	fixture := task5CareerFixture()
	shuffled := []types.Experience{fixture[4], fixture[0], fixture[6], fixture[2], fixture[5], fixture[1], fixture[3]}

	stages := buildCareerStages(shuffled)
	if got := task5FlattenedRoleIDs(stages); !reflect.DeepEqual(got, []int{7, 6, 5, 4, 3, 2, 1}) {
		t.Fatalf("flattened shuffled role IDs = %v, want chronological [7 6 5 4 3 2 1]", got)
	}
}

func TestBuildCareerStagesUsesCanonicalYearBoundariesWithoutLoss(t *testing.T) {
	experiences := []types.Experience{
		{ID: 4, Duration: "2021 – Present"},
		{ID: 2, Duration: "2017 – 2018"},
		{ID: 1, Duration: "2016 – 2017"},
		{ID: 3, Duration: "2020 – 2021"},
	}

	stages := buildCareerStages(experiences)
	if len(stages) != 3 {
		t.Fatalf("len(stages) = %d, want 3 nonempty stages", len(stages))
	}
	wantByStage := [][]int{{1}, {2, 3}, {4}}
	seen := make(map[int]int, len(experiences))
	for index, stage := range stages {
		got := task5RoleIDs(stage.Experiences)
		if !reflect.DeepEqual(got, wantByStage[index]) {
			t.Errorf("stage %q role IDs = %v, want %v", stage.ID, got, wantByStage[index])
		}
		for _, id := range got {
			seen[id]++
		}
	}
	for index := range experiences {
		if count := seen[experiences[index].ID]; count != 1 {
			t.Errorf("experience ID %d occurs %d times across stages, want exactly once", experiences[index].ID, count)
		}
	}
}

func TestBuildExperienceOverviewIsInputOrderIndependent(t *testing.T) {
	fixture := task5CareerFixture()
	shuffled := []types.Experience{fixture[3], fixture[6], fixture[1], fixture[5], fixture[0], fixture[4], fixture[2]}

	overview := buildExperienceOverview(shuffled)

	if overview.CurrentRole.ID != 1 {
		t.Errorf("CurrentRole.ID = %d, want newest role ID 1", overview.CurrentRole.ID)
	}
	if overview.TotalRoles != 7 {
		t.Errorf("TotalRoles = %d, want 7", overview.TotalRoles)
	}
	if overview.TotalTechnologies != 19 {
		t.Errorf("TotalTechnologies = %d, want 19", overview.TotalTechnologies)
	}
	if overview.CareerSpanYears != time.Now().Year()-2012 {
		t.Errorf("CareerSpanYears = %d, want current year - 2012", overview.CareerSpanYears)
	}
	wantSpotlight := []string{"PowerShell", "AD DS", "Windows", "AWS", "Ansible", "Azure", "Bash", "Go"}
	if !reflect.DeepEqual(overview.SpotlightTechnologies, wantSpotlight) {
		t.Errorf("SpotlightTechnologies = %v, want %v", overview.SpotlightTechnologies, wantSpotlight)
	}
}

func TestExperienceOverviewCardsPreserveSummaryOrderAndValues(t *testing.T) {
	cards := experienceOverviewCards(experienceOverview{
		CareerSpanYears:   14,
		TotalRoles:        7,
		TotalTechnologies: 19,
	})
	want := []struct {
		label  string
		value  string
		marker string
	}{
		{label: "Years active", value: "14", marker: "years"},
		{label: "Roles", value: "7", marker: "roles"},
		{label: "Technologies", value: "19", marker: "technologies"},
	}
	if len(cards) != len(want) {
		t.Fatalf("len(experienceOverviewCards) = %d, want %d", len(cards), len(want))
	}
	for index, expected := range want {
		card := cards[index]
		if card.Label != expected.label || card.Value != expected.value {
			t.Errorf("cards[%d] = (%q, %q), want (%q, %q)", index, card.Label, card.Value, expected.label, expected.value)
		}
		if got := card.Attributes["data-experience-summary-stat"]; got != expected.marker {
			t.Errorf("cards[%d] summary marker = %#v, want %q", index, got, expected.marker)
		}
	}
}

func task5RoleIDs(experiences []types.Experience) []int {
	ids := make([]int, 0, len(experiences))
	for index := range experiences {
		ids = append(ids, experiences[index].ID)
	}
	return ids
}

func task5FlattenedRoleIDs(stages []experienceStage) []int {
	total := 0
	for index := range stages {
		total += len(stages[index].Experiences)
	}
	ids := make([]int, 0, total)
	for _, stage := range stages {
		ids = append(ids, task5RoleIDs(stage.Experiences)...)
	}
	return ids
}

func task5CareerFixture() []types.Experience {
	return []types.Experience{
		{ID: 1, Position: "Cloud Engineer Principal", Company: "COMPANY REDACTED - A", Duration: "2022 – Present", Technologies: []string{"AWS", "Go", "Terraform", "Ansible"}},
		{ID: 2, Position: "System Administrator", Company: "COMPANY REDACTED - B", Duration: "2021 – 2022", Technologies: []string{"IoT", "SCADA", "RHEL", "Bash"}},
		{ID: 3, Position: "IT Systems Engineer Sr", Company: "COMPANY REDACTED - C", Duration: "2020 – 2021", Technologies: []string{"Azure", "AD DS", "PowerShell"}},
		{ID: 4, Position: "IT Systems Engineer", Company: "COMPANY REDACTED - C", Duration: "2018 – 2020", Technologies: []string{"PowerShell", "AD DS", "O365/Exchange"}},
		{ID: 5, Position: "IT Desktop Engineer", Company: "COMPANY REDACTED - C", Duration: "2017 – 2018", Technologies: []string{"PowerShell", "SCCM", "Intune"}},
		{ID: 6, Position: "IT Service Desk Associate", Company: "COMPANY REDACTED - C", Duration: "2016 – 2017", Technologies: []string{"ServiceNow", "O365", "Windows"}},
		{ID: 7, Position: "Service Desk Student Analyst", Company: "COMPANY REDACTED - D", Duration: "2012 – 2016", Technologies: []string{"Windows", "MacOS", "GoogleApps"}},
	}
}
