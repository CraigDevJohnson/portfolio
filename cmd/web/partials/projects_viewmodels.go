package partials

import (
	"fmt"
	"strings"

	"github.com/a-h/templ"

	"portfolio/types"
)

type projectDestination struct {
	URL             string
	Label           string
	AccessibleLabel string
	External        bool
	Primary         bool
}

type projectDossier struct {
	Project            types.Project
	PrimaryDestination projectDestination
	Destinations       []projectDestination
}

func buildProjectDossiers(projects []types.Project) []projectDossier {
	ordered := make([]types.Project, 0, len(projects))
	for index := range projects {
		project := &projects[index]
		if project.Featured {
			ordered = append(ordered, *project)
		}
	}
	for index := range projects {
		project := &projects[index]
		if !project.Featured {
			ordered = append(ordered, *project)
		}
	}

	dossiers := make([]projectDossier, 0, len(ordered))
	for index := range ordered {
		project := &ordered[index]
		project.ImageRatio = normalizedProjectImageRatio(project.ImageRatio)
		destinations := projectDestinations(project)
		dossier := projectDossier{Project: *project, Destinations: destinations}
		if len(destinations) > 0 {
			dossier.PrimaryDestination = destinations[0]
		}
		dossiers = append(dossiers, dossier)
	}
	return dossiers
}

func normalizedProjectImageRatio(ratio types.ProjectImageRatio) types.ProjectImageRatio {
	switch ratio {
	case types.ProjectImageLandscape, types.ProjectImagePortrait, types.ProjectImageSquare:
		return ratio
	default:
		return types.ProjectImageLandscape
	}
}

func projectDestinations(project *types.Project) []projectDestination {
	destinations := make([]projectDestination, 0, 2)
	if url := strings.TrimSpace(project.DemoURL); url != "" {
		destinations = append(destinations, newProjectDestination(project.Name, url, "Live Demo", true))
	}
	if url := strings.TrimSpace(project.GitHubURL); url != "" {
		destinations = append(destinations, newProjectDestination(project.Name, url, "GitHub", len(destinations) == 0))
	}
	return destinations
}

func newProjectDestination(projectName, url, label string, primary bool) projectDestination {
	external := !strings.HasPrefix(url, "/")
	accessibleLabel := fmt.Sprintf("%s for %s", label, projectName)
	if external {
		accessibleLabel += " (opens in a new tab)"
	}
	return projectDestination{
		URL:             url,
		Label:           label,
		AccessibleLabel: accessibleLabel,
		External:        external,
		Primary:         primary,
	}
}

func projectDestinationAction(destination projectDestination) ActionLinkProps {
	variant := ActionSecondary
	if destination.Primary {
		variant = ActionPrimary
	}
	return ActionLinkProps{
		Href:       destination.URL,
		Label:      destination.Label,
		Variant:    variant,
		External:   destination.External,
		ShowArrow:  destination.Primary,
		ExtraClass: "project-dossier-action",
		Attributes: templ.Attributes{"aria-label": destination.AccessibleLabel},
	}
}
