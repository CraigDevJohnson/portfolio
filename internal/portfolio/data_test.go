package portfolio

import (
	"reflect"
	"strings"
	"testing"

	"portfolio/types"
)

func TestEmbeddedDataProjectsUseExplicitDossierMetadata(t *testing.T) {
	projects := ProjectsData()
	if len(projects) != 3 {
		t.Fatalf("len(ProjectsData()) = %d, want 3", len(projects))
	}

	allowedRatios := map[types.ProjectImageRatio]bool{
		types.ProjectImageLandscape: true,
		types.ProjectImagePortrait:  true,
		types.ProjectImageSquare:    true,
	}
	featuredCount := 0
	gotIDs := make([]int, 0, len(projects))
	for _, project := range projects {
		gotIDs = append(gotIDs, project.ID)
		if project.Featured {
			featuredCount++
		}
		if !allowedRatios[project.ImageRatio] {
			t.Errorf("project %d ImageRatio = %q, want landscape, portrait, or square", project.ID, project.ImageRatio)
		}
		for field, value := range map[string]string{
			"problem":  project.Problem,
			"approach": project.Approach,
			"outcome":  project.Outcome,
		} {
			if strings.TrimSpace(value) == "" {
				t.Errorf("project %d %s is empty", project.ID, field)
			}
		}
	}
	if featuredCount != 1 {
		t.Errorf("featured project count = %d, want exactly 1", featuredCount)
	}
	if want := []int{1, 2, 3}; !reflect.DeepEqual(gotIDs, want) {
		t.Errorf("embedded project order = %v, want %v", gotIDs, want)
	}
}

func TestEmbeddedDataProjectDossiersPreserveFactsAndDestinations(t *testing.T) {
	projects := ProjectsData()
	want := []struct {
		id         int
		name       string
		problem    string
		approach   string
		outcome    string
		githubURL  string
		demoURL    string
		technology []string
	}{
		{
			id:         1,
			name:       "Personal Portfolio Website",
			problem:    "Bring my projects, skills, certifications, and the soccer schedule tool together in one place.",
			approach:   "A fast server-rendered portfolio built with Go, Templ, HTMX, and Tailwind CSS.",
			outcome:    "Showcases the work with a focus on accessibility and maintainability.",
			githubURL:  "https://github.com/CraigDevJohnson/portfolio",
			demoURL:    "https://craigdevjohnson.com",
			technology: []string{"Go", "Templ", "HTMX", "Tailwind CSS", "AWS", "GitHub Actions"},
		},
		{
			id:         2,
			name:       "New User Account Provisioning",
			problem:    "New user information arrived through a database push and required complete account creation and configuration.",
			approach:   "PowerShell scripts created the new user's Active Directory account, email account in O365/Exchange, and role-based group memberships.",
			outcome:    "Fully automated new user account creation and configuration.",
			technology: []string{"PowerShell", "Git", "APIs", "AD DS", "O365/Exchange"},
		},
		{
			id:         3,
			name:       "Soccer Schedule Scraper",
			problem:    "Pull and parse team schedules for download as an ICS file.",
			approach:   "A multi-function Python script deployed on AWS Lambda scrapes soccer team schedules.",
			outcome:    "Returns ICS files for broadly supported calendar importing.",
			githubURL:  "https://github.com/CraigDevJohnson/soccer-scraper",
			demoURL:    "/soccer",
			technology: []string{"Python", "AWS Lambda", "GitHub", "APIs"},
		},
	}
	if len(projects) != len(want) {
		t.Fatalf("len(projects) = %d, want %d", len(projects), len(want))
	}
	for index, expected := range want {
		project := projects[index]
		if project.ID != expected.id || project.Name != expected.name {
			t.Errorf("projects[%d] identity = (%d, %q), want (%d, %q)", index, project.ID, project.Name, expected.id, expected.name)
		}
		if project.Problem != expected.problem || project.Approach != expected.approach || project.Outcome != expected.outcome {
			t.Errorf("project %d dossier copy changed: problem=%q approach=%q outcome=%q", project.ID, project.Problem, project.Approach, project.Outcome)
		}
		if project.GitHubURL != expected.githubURL || project.DemoURL != expected.demoURL {
			t.Errorf("project %d destinations = (%q, %q), want (%q, %q)", project.ID, project.GitHubURL, project.DemoURL, expected.githubURL, expected.demoURL)
		}
		if !reflect.DeepEqual(project.Technologies, expected.technology) {
			t.Errorf("project %d technologies = %v, want %v", project.ID, project.Technologies, expected.technology)
		}
	}
}

func TestEmbeddedSkillsUseValidSecondaryTagsWithoutChangingPrimaryOwnership(t *testing.T) {
	categories := SkillsData()
	validTags := make(map[string]bool, len(categories))
	for _, category := range categories {
		if category.Name != "Concepts & Practices" {
			validTags[category.Name] = true
		}
	}

	githubFound := false
	taggedSkills := 0
	for _, category := range categories {
		for skillIndex := range category.Skills {
			skill := &category.Skills[skillIndex]
			if len(skill.Tags) > 0 {
				taggedSkills++
			}
			seen := make(map[string]bool, len(skill.Tags))
			for _, tag := range skill.Tags {
				if !validTags[tag] {
					t.Errorf("skill %q uses unknown secondary tag %q", skill.Name, tag)
				}
				if tag == category.Name {
					t.Errorf("skill %q repeats its primary category %q as a secondary tag", skill.Name, tag)
				}
				if seen[tag] {
					t.Errorf("skill %q repeats secondary tag %q", skill.Name, tag)
				}
				seen[tag] = true
			}
			if skill.Name == "GitHub" {
				githubFound = true
				if category.Name != "Development Tools" {
					t.Errorf("GitHub primary category = %q, want Development Tools", category.Name)
				}
				if !seen["Collaboration Tools"] {
					t.Error("GitHub lacks Collaboration Tools secondary tag")
				}
			}
		}
	}
	if !githubFound {
		t.Fatal("embedded skills lack GitHub")
	}
	if taggedSkills < 10 {
		t.Fatalf("tagged skill count = %d, want an audited cross-category set", taggedSkills)
	}
}
