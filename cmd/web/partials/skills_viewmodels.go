package partials

import (
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"portfolio/types"
)

const (
	conceptsCategoryName = "Concepts & Practices"
	maxSkillQueryRunes   = 80
)

var skillProficiencyOptions = []struct {
	Value string
	Label string
}{
	{Value: "", Label: "All"},
	{Value: "expert", Label: "Expert"},
	{Value: "advanced", Label: "Advanced"},
	{Value: "intermediate", Label: "Intermediate"},
	{Value: "familiar", Label: "Familiar"},
}

// SkillFilters is the canonical, URL-backed state for the searchable catalog.
type SkillFilters struct {
	Query       string
	Category    string
	Proficiency string
}

// SkillFilterOption prepares a progressive filter link for Templ.
type SkillFilterOption struct {
	ID     string
	Label  string
	Value  string
	Href   string
	Active bool
}

// SkillCatalogItem keeps interaction IDs stable while filters reshape the grid.
type SkillCatalogItem struct {
	Skill     types.Skill
	TriggerID string
}

// SkillCatalogCategory is one prepared catalog group with semantic IDs.
type SkillCatalogCategory struct {
	ID           string
	Name         string
	DetailSlotID string
	Skills       []SkillCatalogItem
}

// FeaturedSkillItem carries a stable design slot independently of slice position.
type FeaturedSkillItem struct {
	Skill types.Skill
	Slot  string
}

// SkillsGridProps contains all prepared presentation state for the workbench.
type SkillsGridProps struct {
	Categories         []types.SkillCategory
	FeaturedSkills     []FeaturedSkillItem
	PracticeSkills     []types.Skill
	Filters            SkillFilters
	CategoryOptions    []SkillFilterOption
	ProficiencyOptions []SkillFilterOption
	VisibleCategories  []SkillCatalogCategory
	VisibleCount       int
	TotalCatalogCount  int
	ResultSummary      string
	NoResultsMessage   string
	ClearHref          string
	StateURL           string
}

// NormalizeSkillFilters validates public URL state against the real catalog.
func NormalizeSkillFilters(values url.Values, categories []types.SkillCategory) SkillFilters {
	query := strings.TrimSpace(values.Get("q"))
	if utf8.RuneCountInString(query) > maxSkillQueryRunes {
		query = string([]rune(query)[:maxSkillQueryRunes])
	}

	category := strings.TrimSpace(values.Get("category"))
	if !hasSkillCategory(categories, category) {
		category = ""
	}

	proficiency := strings.ToLower(strings.TrimSpace(values.Get("proficiency")))
	switch proficiency {
	case "familiar", "intermediate", "advanced", "expert":
	default:
		proficiency = ""
	}

	return SkillFilters{Query: query, Category: category, Proficiency: proficiency}
}

// BuildSkillsGridProps prepares one shared state model for full pages and HTMX fragments.
func BuildSkillsGridProps(categories []types.SkillCategory, filters SkillFilters) SkillsGridProps {
	filters = NormalizeSkillFilters(url.Values{
		"q":           []string{filters.Query},
		"category":    []string{filters.Category},
		"proficiency": []string{filters.Proficiency},
	}, categories)

	props := SkillsGridProps{
		Categories:       cloneSkillCategories(categories),
		Filters:          filters,
		ClearHref:        "/skills",
		NoResultsMessage: "No skills match these filters. Try another category, proficiency, or search term.",
	}

	for _, category := range categories {
		if category.Name == conceptsCategoryName {
			props.PracticeSkills = skillsWithCategory(category)
			continue
		}

		props.TotalCatalogCount += len(category.Skills)
		for skillIndex := range category.Skills {
			sourceSkill := &category.Skills[skillIndex]
			if sourceSkill.Featured {
				skill := cloneSkill(sourceSkill)
				skill.Category = category.Name
				props.FeaturedSkills = append(props.FeaturedSkills, FeaturedSkillItem{
					Skill: skill,
					Slot:  featuredSkillSlot(skill.ID),
				})
			}
		}

		categorySlug := skillSlug(category.Name)
		visible := SkillCatalogCategory{
			ID:           "skill-category-" + categorySlug,
			Name:         category.Name,
			DetailSlotID: "skill-detail-" + categorySlug,
		}
		for skillIndex := range category.Skills {
			sourceSkill := &category.Skills[skillIndex]
			if !skillMatchesCategory(sourceSkill, category.Name, filters.Category) {
				continue
			}
			if filters.Proficiency != "" && sourceSkill.Proficiency != filters.Proficiency {
				continue
			}
			if !skillMatchesQuery(sourceSkill, category.Name, filters.Query) {
				continue
			}
			skill := cloneSkill(sourceSkill)
			skill.Category = category.Name
			visible.Skills = append(visible.Skills, SkillCatalogItem{
				Skill:     skill,
				TriggerID: "skill-trigger-" + strconv.Itoa(skill.ID),
			})
		}
		if len(visible.Skills) > 0 {
			props.VisibleCategories = append(props.VisibleCategories, visible)
			props.VisibleCount += len(visible.Skills)
		}
	}

	props.CategoryOptions = buildSkillCategoryOptions(categories, filters)
	props.ProficiencyOptions = buildSkillProficiencyOptions(filters)
	props.StateURL = buildSkillsURL(filters)
	if props.VisibleCount == 1 {
		props.ResultSummary = "1 skill shown"
	} else {
		props.ResultSummary = strconv.Itoa(props.VisibleCount) + " skills shown"
	}
	return props
}

func featuredSkillSlot(id int) string {
	switch id {
	case 2:
		return "anchor"
	case 29:
		return "bridge"
	case 44:
		return "signal"
	default:
		return "standard"
	}
}

func cloneSkillCategories(categories []types.SkillCategory) []types.SkillCategory {
	cloned := make([]types.SkillCategory, len(categories))
	for index, category := range categories {
		cloned[index] = category
		cloned[index].Skills = make([]types.Skill, len(category.Skills))
		for skillIndex := range category.Skills {
			cloned[index].Skills[skillIndex] = cloneSkill(&category.Skills[skillIndex])
		}
	}
	return cloned
}

func cloneSkill(skill *types.Skill) types.Skill {
	cloned := *skill
	cloned.Tags = append([]string(nil), skill.Tags...)
	return cloned
}

func skillSlug(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	var builder strings.Builder
	lastDash := false
	for _, character := range value {
		switch {
		case character >= 'a' && character <= 'z', character >= '0' && character <= '9':
			builder.WriteRune(character)
			lastDash = false
		case !lastDash:
			builder.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(builder.String(), "-")
	if slug == "" {
		return "catalog"
	}
	return slug
}

func skillsWithCategory(category types.SkillCategory) []types.Skill {
	result := make([]types.Skill, 0, len(category.Skills))
	for skillIndex := range category.Skills {
		skill := cloneSkill(&category.Skills[skillIndex])
		skill.Category = category.Name
		result = append(result, skill)
	}
	return result
}

func hasSkillCategory(categories []types.SkillCategory, wanted string) bool {
	if wanted == "" || wanted == conceptsCategoryName {
		return false
	}
	for _, category := range categories {
		if category.Name == wanted {
			return true
		}
	}
	return false
}

func skillMatchesQuery(skill *types.Skill, category, query string) bool {
	if query == "" {
		return true
	}
	fields := make([]string, 0, 3+len(skill.Tags))
	fields = append(fields, skill.Name, skill.Description, category)
	fields = append(fields, skill.Tags...)
	haystack := strings.ToLower(strings.Join(fields, " "))
	return strings.Contains(haystack, strings.ToLower(query))
}

func skillMatchesCategory(skill *types.Skill, primaryCategory, wanted string) bool {
	if wanted == "" || primaryCategory == wanted {
		return true
	}
	for _, tag := range skill.Tags {
		if tag == wanted {
			return true
		}
	}
	return false
}

func buildSkillCategoryOptions(categories []types.SkillCategory, filters SkillFilters) []SkillFilterOption {
	options := []SkillFilterOption{{
		ID:     "skills-category-all",
		Label:  "All",
		Value:  "",
		Active: filters.Category == "",
		Href:   buildSkillsURL(SkillFilters{Query: filters.Query, Proficiency: filters.Proficiency}),
	}}
	for _, category := range categories {
		if category.Name == conceptsCategoryName {
			continue
		}
		next := filters
		next.Category = category.Name
		options = append(options, SkillFilterOption{
			ID:     "skills-category-" + skillSlug(category.Name),
			Label:  category.Name,
			Value:  category.Name,
			Active: filters.Category == category.Name,
			Href:   buildSkillsURL(next),
		})
	}
	return options
}

func buildSkillProficiencyOptions(filters SkillFilters) []SkillFilterOption {
	options := make([]SkillFilterOption, 0, len(skillProficiencyOptions))
	for _, proficiency := range skillProficiencyOptions {
		next := filters
		next.Proficiency = proficiency.Value
		options = append(options, SkillFilterOption{
			ID:     "skills-proficiency-" + skillSlug(proficiency.Label),
			Label:  proficiency.Label,
			Value:  proficiency.Value,
			Active: filters.Proficiency == proficiency.Value,
			Href:   buildSkillsURL(next),
		})
	}
	return options
}

func buildSkillsURL(filters SkillFilters) string {
	parts := make([]string, 0, 3)
	if filters.Query != "" {
		parts = append(parts, "q="+url.QueryEscape(filters.Query))
	}
	if filters.Category != "" {
		parts = append(parts, "category="+url.QueryEscape(filters.Category))
	}
	if filters.Proficiency != "" {
		parts = append(parts, "proficiency="+url.QueryEscape(filters.Proficiency))
	}
	if len(parts) == 0 {
		return "/skills"
	}
	return "/skills?" + strings.Join(parts, "&")
}
