package portfolio

import (
	"net/url"

	"portfolio/cmd/web/pages"
	"portfolio/cmd/web/partials"
	"portfolio/types"
)

func buildSkillsPageProps(categories []types.SkillCategory, values url.Values) pages.SkillsProps {
	filters := partials.NormalizeSkillFilters(values, categories)
	return pages.SkillsProps{Grid: partials.BuildSkillsGridProps(categories, filters)}
}
