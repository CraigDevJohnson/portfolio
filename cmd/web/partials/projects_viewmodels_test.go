package partials

import (
	"reflect"
	"strings"
	"testing"

	"portfolio/types"
)

func TestBuildProjectDossiersUsesExplicitFeaturedMetadataAndStableOrder(t *testing.T) {
	projects := []types.Project{
		{ID: 2, Name: "Support A", ImageRatio: types.ProjectImageLandscape},
		{ID: 1, Name: "Lead", Featured: true, ImageRatio: types.ProjectImagePortrait},
		{ID: 3, Name: "Support B", ImageRatio: types.ProjectImageSquare},
	}

	dossiers := buildProjectDossiers(projects)

	gotIDs := make([]int, 0, len(dossiers))
	for _, dossier := range dossiers {
		gotIDs = append(gotIDs, dossier.Project.ID)
	}
	if want := []int{1, 2, 3}; !reflect.DeepEqual(gotIDs, want) {
		t.Fatalf("dossier project IDs = %v, want metadata-led stable order %v", gotIDs, want)
	}
	if !dossiers[0].Project.Featured {
		t.Fatal("first dossier is not the explicitly featured project")
	}
}

func TestBuildProjectDossiersPrefersDemoAndPreservesEveryDestination(t *testing.T) {
	dossiers := buildProjectDossiers([]types.Project{{
		ID:        1,
		Name:      "Portfolio",
		Featured:  true,
		DemoURL:   "https://portfolio.example",
		GitHubURL: "https://github.com/example/portfolio",
	}})

	if len(dossiers) != 1 {
		t.Fatalf("len(dossiers) = %d, want 1", len(dossiers))
	}
	dossier := dossiers[0]
	if dossier.PrimaryDestination.URL != "https://portfolio.example" || dossier.PrimaryDestination.Label != "Live Demo" || !dossier.PrimaryDestination.External {
		t.Errorf("primary destination = %#v, want external live demo", dossier.PrimaryDestination)
	}
	if len(dossier.Destinations) != 2 {
		t.Fatalf("len(destinations) = %d, want demo and GitHub", len(dossier.Destinations))
	}
	if !dossier.Destinations[0].Primary || dossier.Destinations[0].URL != "https://portfolio.example" {
		t.Errorf("first destination = %#v, want preferred demo", dossier.Destinations[0])
	}
	if dossier.Destinations[1].Primary || dossier.Destinations[1].Label != "GitHub" || !dossier.Destinations[1].External {
		t.Errorf("second destination = %#v, want secondary external GitHub", dossier.Destinations[1])
	}
}

func TestBuildProjectDossiersFallsBackToGitHub(t *testing.T) {
	dossier := buildProjectDossiers([]types.Project{{
		ID:        2,
		Name:      "Repository only",
		GitHubURL: "https://github.com/example/repository-only",
	}})[0]

	if dossier.PrimaryDestination.URL != "https://github.com/example/repository-only" || dossier.PrimaryDestination.Label != "GitHub" || !dossier.PrimaryDestination.External {
		t.Errorf("primary destination = %#v, want GitHub fallback", dossier.PrimaryDestination)
	}
}

func TestBuildProjectDossiersKeepsInternalSoccerDestinationInSameTab(t *testing.T) {
	dossier := buildProjectDossiers([]types.Project{{
		ID:        3,
		Name:      "Soccer Schedule Scraper",
		DemoURL:   "/soccer",
		GitHubURL: "https://github.com/example/soccer",
	}})[0]

	if dossier.PrimaryDestination.URL != "/soccer" || dossier.PrimaryDestination.Label != "Live Demo" {
		t.Errorf("primary destination = %#v, want internal Soccer tool", dossier.PrimaryDestination)
	}
	if dossier.PrimaryDestination.External {
		t.Errorf("internal Soccer destination unexpectedly opens a new tab: %#v", dossier.PrimaryDestination)
	}
}

func TestBuildProjectDossiersNormalizesInvalidImageRatio(t *testing.T) {
	dossier := buildProjectDossiers([]types.Project{{
		ID:         4,
		Name:       "Unknown image shape",
		ImageRatio: types.ProjectImageRatio("cinematic"),
	}})[0]

	if dossier.Project.ImageRatio != types.ProjectImageLandscape {
		t.Errorf("invalid image ratio normalized to %q, want %q", dossier.Project.ImageRatio, types.ProjectImageLandscape)
	}
}

func TestProjectDossierPreservesVisibleActionLabels(t *testing.T) {
	dossier := buildProjectDossiers([]types.Project{{
		ID:        1,
		Name:      "Portfolio",
		DemoURL:   "https://portfolio.example",
		GitHubURL: "https://github.com/example/portfolio",
	}})[0]

	html := renderComponent(t, ProjectDossier(dossier))
	for _, marker := range []string{`<span>Live Demo</span>`, `<span>GitHub</span>`} {
		if !strings.Contains(html, marker) {
			t.Errorf("ProjectDossier output does not preserve visible action label %q: %s", marker, html)
		}
	}
}
