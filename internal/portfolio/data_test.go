package portfolio

import (
	"testing"

	"portfolio/types"
)

func TestProjectStats(t *testing.T) {
	tests := []struct {
		name     string
		projects []types.Project
		want     projectStatsSummary
	}{
		{
			name: "counts unique technologies categories and public repos",
			projects: []types.Project{
				{
					Name:         "Portfolio",
					Category:     "Web",
					GitHubURL:    "https://github.com/CraigDevJohnson/portfolio",
					Technologies: []string{"Go", "Templ", "HTMX", "Tailwind CSS"},
				},
				{
					Name:         "Provisioning",
					Category:     "Automation",
					Technologies: []string{"PowerShell", "APIs", "Git"},
				},
				{
					Name:         "Soccer",
					Category:     "Automation",
					GitHubURL:    "https://github.com/CraigDevJohnson/soccer-scraper",
					Technologies: []string{"Python", "APIs", "GitHub"},
				},
			},
			want: projectStatsSummary{
				TotalProjects:         3,
				UniqueTechnologies:    9,
				CategoryCount:         2,
				PublicRepositoryCount: 2,
			},
		},
		{
			name: "ignores blank values and normalizes case",
			projects: []types.Project{
				{
					Category:     " Web ",
					GitHubURL:    "   ",
					Technologies: []string{" Go ", "go", "", "HTMX"},
				},
				{
					Category:     "web",
					GitHubURL:    "https://example.com/repo",
					Technologies: []string{"htmx", "Templ"},
				},
			},
			want: projectStatsSummary{
				TotalProjects:         2,
				UniqueTechnologies:    3,
				CategoryCount:         1,
				PublicRepositoryCount: 1,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := projectStats(test.projects); got != test.want {
				t.Fatalf("projectStats() = %+v, want %+v", got, test.want)
			}
		})
	}
}
