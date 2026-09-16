package pages

import (
	"fmt"
	"strconv"

	"portfolio/cmd/web/partials"
)

func skillsOverviewCards(grid *partials.SkillsGridProps) []partials.StatCardProps {
	categoryCount := 0
	for _, category := range grid.CategoryOptions {
		if category.Value != "" {
			categoryCount++
		}
	}
	practiceCount := len(grid.PracticeSkills)

	return []partials.StatCardProps{
		{Value: strconv.Itoa(grid.TotalCatalogCount), Label: "Skills", AriaLabel: fmt.Sprintf("%d skills in the catalog", grid.TotalCatalogCount), Tone: partials.ToneApricot},
		{Value: strconv.Itoa(categoryCount), Label: "Categories", AriaLabel: fmt.Sprintf("%d skill categories", categoryCount), Tone: partials.ToneRose},
		{Value: strconv.Itoa(practiceCount), Label: "Practices", AriaLabel: fmt.Sprintf("%d operating practices", practiceCount), Tone: partials.ToneMint},
	}
}
