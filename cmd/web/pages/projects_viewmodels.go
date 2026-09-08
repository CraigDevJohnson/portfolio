package pages

import (
	"fmt"
	"strconv"

	"portfolio/cmd/web/partials"
	"portfolio/types"
)

func projectsOverviewCards(projects []types.Project) []partials.StatCardProps {
	automationCount := 0
	webCount := 0
	for i := range projects {
		switch projects[i].Category {
		case "Automation":
			automationCount++
		case "Web":
			webCount++
		}
	}

	return []partials.StatCardProps{
		{Value: strconv.Itoa(len(projects)), Label: "Projects", AriaLabel: projectOverviewCountLabel(len(projects), "project"), Tone: partials.ToneApricot},
		{Value: strconv.Itoa(automationCount), Label: "Automation", AriaLabel: projectOverviewCountLabel(automationCount, "automation project"), Tone: partials.ToneRose},
		{Value: strconv.Itoa(webCount), Label: "Web", AriaLabel: projectOverviewCountLabel(webCount, "web project"), Tone: partials.ToneMint},
	}
}

func projectOverviewCountLabel(count int, singular string) string {
	if count == 1 {
		return fmt.Sprintf("1 %s", singular)
	}
	return fmt.Sprintf("%d %ss", count, singular)
}
