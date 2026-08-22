package partials

import (
	"net/url"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"portfolio/types"
)

func TestNormalizeSkillFiltersValidatesAxesAndCapsQueryByUnicodeRune(t *testing.T) {
	categories := []types.SkillCategory{{Name: "Cloud Platforms"}, {Name: "Development Tools"}}
	longQuery := "  " + strings.Repeat("界", 79) + "🙂tail  "

	filters := NormalizeSkillFilters(url.Values{
		"q":           []string{longQuery},
		"category":    []string{"Cloud Platforms"},
		"proficiency": []string{"ADVANCED"},
	}, categories)

	if filters.Category != "Cloud Platforms" {
		t.Errorf("Category = %q, want validated category", filters.Category)
	}
	if filters.Proficiency != "advanced" {
		t.Errorf("Proficiency = %q, want normalized advanced", filters.Proficiency)
	}
	if got := utf8.RuneCountInString(filters.Query); got != 80 {
		t.Errorf("query rune count = %d, want 80", got)
	}
	if !strings.HasSuffix(filters.Query, "🙂") {
		t.Errorf("capped query = %q, want complete Unicode rune at boundary", filters.Query)
	}
}

func TestNormalizeSkillFiltersRejectsUnknownCategoryAndProficiency(t *testing.T) {
	filters := NormalizeSkillFilters(url.Values{
		"category":    []string{"Imaginary Platforms"},
		"proficiency": []string{"master"},
	}, []types.SkillCategory{{Name: "Cloud Platforms"}})

	if filters.Category != "" || filters.Proficiency != "" {
		t.Fatalf("invalid axes normalized to %#v, want inactive filters", filters)
	}
}

func TestNormalizeSkillFiltersRejectsConceptsAsCatalogCategory(t *testing.T) {
	filters := NormalizeSkillFilters(url.Values{
		"category": []string{conceptsCategoryName},
	}, skillViewModelFixture())
	if filters.Category != "" {
		t.Fatalf("Concepts & Practices normalized to active catalog category %q", filters.Category)
	}

	props := BuildSkillsGridProps(skillViewModelFixture(), filters)
	for _, option := range props.CategoryOptions {
		if option.Value == conceptsCategoryName {
			t.Fatalf("catalog category options include supporting practice band: %#v", option)
		}
	}
}

func TestBuildSkillsGridPropsFiltersNameDescriptionAndCategoryCaseInsensitively(t *testing.T) {
	props := BuildSkillsGridProps(skillViewModelFixture(), SkillFilters{Query: "CLOUD"})

	if props.VisibleCount != 3 {
		t.Fatalf("VisibleCount = %d, want 3 name/description/category matches", props.VisibleCount)
	}
	got := visibleSkillNames(props.VisibleCategories)
	want := []string{"AWS", "Terraform", "Go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("visible skill names = %v, want %v", got, want)
	}
	if props.TotalCatalogCount != 3 {
		t.Errorf("TotalCatalogCount = %d, want Concepts & Practices excluded", props.TotalCatalogCount)
	}
}

func TestBuildSkillsGridPropsCombinesAllAxesAndKeepsValidEmptyProficiency(t *testing.T) {
	combined := BuildSkillsGridProps(skillViewModelFixture(), SkillFilters{
		Query:       "automation",
		Category:    "Cloud Platforms",
		Proficiency: "advanced",
	})
	if combined.VisibleCount != 1 || len(combined.VisibleCategories) != 1 || combined.VisibleCategories[0].Skills[0].Skill.Name != "Terraform" {
		t.Fatalf("combined filters produced %#v, want only Terraform", combined.VisibleCategories)
	}

	empty := BuildSkillsGridProps(skillViewModelFixture(), SkillFilters{Proficiency: "familiar"})
	if empty.Filters.Proficiency != "familiar" {
		t.Errorf("valid empty proficiency = %q, want familiar retained", empty.Filters.Proficiency)
	}
	if empty.VisibleCount != 0 || len(empty.VisibleCategories) != 0 {
		t.Errorf("familiar filter returned %d skills across %d categories, want valid empty result", empty.VisibleCount, len(empty.VisibleCategories))
	}
	if empty.NoResultsMessage == "" {
		t.Error("valid empty result is missing directional copy")
	}
}

func TestBuildSkillsGridPropsMatchesSecondaryTagsWithoutMovingPrimaryCategory(t *testing.T) {
	categories := []types.SkillCategory{
		{
			Name: "Development Tools",
			Skills: []types.Skill{
				{ID: 45, Name: "GitHub", Proficiency: "expert", Tags: []string{"Collaboration Tools", "CI/CD & Automation"}},
			},
		},
		{
			Name: "Collaboration Tools",
			Skills: []types.Skill{
				{ID: 68, Name: "Slack", Proficiency: "expert"},
			},
		},
	}

	filtered := BuildSkillsGridProps(categories, SkillFilters{Category: "Collaboration Tools"})
	if got, want := visibleSkillNames(filtered.VisibleCategories), []string{"GitHub", "Slack"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("secondary category filter names = %v, want %v", got, want)
	}
	if got := filtered.VisibleCategories[0].Name; got != "Development Tools" {
		t.Fatalf("GitHub rendered under %q, want canonical Development Tools category", got)
	}
	if got := filtered.VisibleCategories[0].Skills[0].Skill.Category; got != "Development Tools" {
		t.Fatalf("GitHub prepared category = %q, want canonical Development Tools category", got)
	}

	searched := BuildSkillsGridProps(categories, SkillFilters{Query: "collaboration tools"})
	if got, want := visibleSkillNames(searched.VisibleCategories), []string{"GitHub", "Slack"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("secondary tag search names = %v, want %v", got, want)
	}
}

func TestBuildSkillsGridPropsDeepCopiesInputAndNestedSkills(t *testing.T) {
	input := skillViewModelFixture()
	input[0].Skills[0].Tags = []string{"Infrastructure as Code"}
	props := BuildSkillsGridProps(input, SkillFilters{})

	props.Categories[0].Name = "Mutated category"
	props.Categories[0].Skills[0].Name = "Mutated skill"
	props.VisibleCategories[0].Skills[0].Skill.Name = "Mutated visible skill"
	props.FeaturedSkills[0].Skill.Name = "Mutated featured skill"
	props.Categories[0].Skills[0].Tags[0] = "Mutated prepared tag"
	props.VisibleCategories[0].Skills[0].Skill.Tags[0] = "Mutated visible tag"

	if input[0].Name != "Cloud Platforms" || input[0].Skills[0].Name != "AWS" {
		t.Fatalf("BuildSkillsGridProps output aliases caller input: %#v", input[0])
	}
	if props.Categories[0].Skills[0].Name == props.VisibleCategories[0].Skills[0].Skill.Name {
		t.Fatal("prepared category and visible catalog share nested Skill storage")
	}
	if got := input[0].Skills[0].Tags[0]; got != "Infrastructure as Code" {
		t.Fatalf("BuildSkillsGridProps output aliases caller tag storage: %q", got)
	}
	if props.Categories[0].Skills[0].Tags[0] == props.VisibleCategories[0].Skills[0].Skill.Tags[0] {
		t.Fatal("prepared category and visible catalog share nested tag storage")
	}
}

func TestSkillDetailShowsSecondaryTagsOnlyInDetailPanel(t *testing.T) {
	html := renderComponent(t, SkillDetail(SkillDetailProps{Skill: types.Skill{
		ID:          45,
		Name:        "GitHub",
		Category:    "Development Tools",
		Proficiency: "expert",
		Tags:        []string{"Collaboration Tools", "CI/CD & Automation"},
	}}))

	for _, marker := range []string{`class="skill-detail-tags"`, `aria-label="Related areas"`, "Collaboration Tools", "CI/CD &amp; Automation"} {
		if !strings.Contains(html, marker) {
			t.Errorf("skill detail missing secondary-tag marker %q: %s", marker, html)
		}
	}
}

func TestBuildSkillsGridPropsUsesStableSemanticIDsAcrossFilterShapes(t *testing.T) {
	all := BuildSkillsGridProps(skillViewModelFixture(), SkillFilters{})
	filtered := BuildSkillsGridProps(skillViewModelFixture(), SkillFilters{Category: "Development Tools"})

	allDev := findCatalogCategory(t, all.VisibleCategories, "Development Tools")
	filteredDev := findCatalogCategory(t, filtered.VisibleCategories, "Development Tools")
	if allDev.ID != filteredDev.ID || allDev.DetailSlotID != filteredDev.DetailSlotID {
		t.Fatalf("category IDs depend on visible position: all=%#v filtered=%#v", allDev, filteredDev)
	}
	if allDev.ID != "skill-category-development-tools" || allDev.DetailSlotID != "skill-detail-development-tools" {
		t.Errorf("semantic IDs = (%q, %q), want stable category-derived IDs", allDev.ID, allDev.DetailSlotID)
	}
	if got := allDev.Skills[0].TriggerID; got != "skill-trigger-3" {
		t.Errorf("skill trigger ID = %q, want skill-ID-derived value", got)
	}
}

func TestBuildSkillsGridPropsEscapesHostileUnicodeQueryInCanonicalURLs(t *testing.T) {
	raw := "  cloud & <script>🙂 " + strings.Repeat("界", 90)
	props := BuildSkillsGridProps(skillViewModelFixture(), SkillFilters{Query: raw})
	option := findSkillFilterOption(t, props.ProficiencyOptions, "expert")

	parsed, err := url.Parse(option.Href)
	if err != nil {
		t.Fatalf("parse canonical href %q: %v", option.Href, err)
	}
	if parsed.Path != "/skills" || parsed.Query().Get("proficiency") != "expert" {
		t.Fatalf("canonical href = %q, want /skills and expert axis", option.Href)
	}
	gotQuery := parsed.Query().Get("q")
	if gotQuery != props.Filters.Query || utf8.RuneCountInString(gotQuery) != 80 {
		t.Fatalf("round-tripped query = %q (%d runes), want normalized 80-rune query %q", gotQuery, utf8.RuneCountInString(gotQuery), props.Filters.Query)
	}
	if strings.Contains(option.Href, "<script>") || !strings.Contains(option.Href, "%26") || !strings.Contains(option.Href, "%3Cscript%3E") {
		t.Errorf("hostile query is not safely encoded in href %q", option.Href)
	}
}

func TestBuildSkillsGridPropsCreatesCompleteCanonicalFilterURLs(t *testing.T) {
	props := BuildSkillsGridProps(skillViewModelFixture(), SkillFilters{
		Query:       "terraform modules",
		Category:    "Cloud Platforms",
		Proficiency: "advanced",
	})

	cloud := findSkillFilterOption(t, props.CategoryOptions, "Cloud Platforms")
	if cloud.Href != "/skills?q=terraform+modules&category=Cloud+Platforms&proficiency=advanced" || !cloud.Active {
		t.Errorf("active category option = %#v, want complete canonical URL", cloud)
	}
	clearCategory := findSkillFilterOption(t, props.CategoryOptions, "")
	if clearCategory.Href != "/skills?q=terraform+modules&proficiency=advanced" {
		t.Errorf("all-category href = %q, want query and proficiency preserved", clearCategory.Href)
	}
	expert := findSkillFilterOption(t, props.ProficiencyOptions, "expert")
	if expert.Href != "/skills?q=terraform+modules&category=Cloud+Platforms&proficiency=expert" {
		t.Errorf("expert href = %q, want query and category preserved", expert.Href)
	}
	if props.ClearHref != "/skills" {
		t.Errorf("ClearHref = %q, want canonical unfiltered route", props.ClearHref)
	}
}

func TestBuildSkillsGridPropsAssignsStableUniqueControlIDs(t *testing.T) {
	props := BuildSkillsGridProps(skillViewModelFixture(), SkillFilters{})
	seen := make(map[string]bool)
	for _, optionSet := range [][]SkillFilterOption{props.CategoryOptions, props.ProficiencyOptions} {
		for _, option := range optionSet {
			if option.ID == "" || seen[option.ID] {
				t.Fatalf("filter option has missing or duplicate ID: %#v", option)
			}
			seen[option.ID] = true
		}
	}
	if !seen["skills-category-all"] || !seen["skills-proficiency-all"] {
		t.Fatalf("stable all-option IDs missing: %v", seen)
	}
}

func skillViewModelFixture() []types.SkillCategory {
	return []types.SkillCategory{
		{
			Name: "Cloud Platforms",
			Skills: []types.Skill{
				{ID: 1, Name: "AWS", Proficiency: "expert", Description: "Public cloud services", Featured: true},
				{ID: 2, Name: "Terraform", Proficiency: "advanced", Description: "Infrastructure automation"},
			},
		},
		{
			Name: "Development Tools",
			Skills: []types.Skill{
				{ID: 3, Name: "Go", Proficiency: "expert", Description: "Cloud service tooling"},
			},
		},
		{
			Name: "Concepts & Practices",
			Skills: []types.Skill{
				{ID: 4, Name: "Site Reliability Engineering", Proficiency: "expert"},
			},
		},
	}
}

func visibleSkillNames(categories []SkillCatalogCategory) []string {
	var names []string
	for _, category := range categories {
		for skillIndex := range category.Skills {
			names = append(names, category.Skills[skillIndex].Skill.Name)
		}
	}
	return names
}

func findCatalogCategory(t *testing.T, categories []SkillCatalogCategory, name string) SkillCatalogCategory {
	t.Helper()
	for _, category := range categories {
		if category.Name == name {
			return category
		}
	}
	t.Fatalf("catalog category %q not found in %#v", name, categories)
	return SkillCatalogCategory{}
}

func findSkillFilterOption(t *testing.T, options []SkillFilterOption, value string) SkillFilterOption {
	t.Helper()
	for _, option := range options {
		if option.Value == value {
			return option
		}
	}
	t.Fatalf("filter option %q not found in %#v", value, options)
	return SkillFilterOption{}
}
