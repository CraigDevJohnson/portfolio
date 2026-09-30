package lambda

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// siteIdentityRoots are the separate development and production site sign-in
// roots. Each keeps its own state, account pin, credentials and session path.
var siteIdentityRoots = []string{"dev", "prod"}

func TestSiteIdentityRootsKeepSeparateEncryptedWorkloadsState(t *testing.T) {
	keys := map[string]string{}
	for _, environment := range siteIdentityRoots {
		backend := readSiteIdentityFile(t, environment, "backend.hcl")
		key := "portfolio-lambda-http-api/auth/site/" + environment + "/terraform.tfstate"
		for _, setting := range []string{
			`bucket       = "portfolio-tofu-state-793680745829"`,
			`key          = "` + key + `"`,
			`region       = "us-west-2"`,
			`encrypt      = true`,
			`use_lockfile = true`,
		} {
			if !strings.Contains(backend, setting) {
				t.Errorf("%s site identity backend is missing %q", environment, setting)
			}
		}
		keys[environment] = key
	}
	if keys["dev"] == keys["prod"] {
		t.Errorf("site identity roots must not share state: %v", keys)
	}
}

func TestSiteIdentityProvidersArePinnedToTheWorkloadsAccount(t *testing.T) {
	for _, environment := range siteIdentityRoots {
		provider := readSiteIdentityFile(t, environment, "providers.tf")
		for _, contract := range []string{
			`region              = "us-west-2"`,
			`allowed_account_ids = [var.aws_account_id]`,
			`Environment = "` + environment + `"`,
		} {
			if !strings.Contains(provider, contract) {
				t.Errorf("%s site identity provider is missing %q", environment, contract)
			}
		}
		account := hclBlock(t, readSiteIdentityFile(t, environment, "variables.tf"), "variable", "aws_account_id")
		if !strings.Contains(account, `default     = "793680745829"`) {
			t.Errorf("%s site identity root must default to the workloads account", environment)
		}
	}
}

func TestSiteIdentityGoogleCredentialsAreSensitiveInputsOnly(t *testing.T) {
	for _, environment := range siteIdentityRoots {
		variables := readSiteIdentityFile(t, environment, "variables.tf")
		for _, name := range []string{"google_client_id", "google_client_secret"} {
			block := hclBlock(t, variables, "variable", name)
			if !regexp.MustCompile(`(?m)^\s*sensitive\s*=\s*true\s*$`).MatchString(block) {
				t.Errorf("%s variable %q must be sensitive", environment, name)
			}
			if regexp.MustCompile(`(?m)^\s*default\s*=`).MatchString(block) {
				t.Errorf("%s variable %q must not have a default", environment, name)
			}
		}
		outputs := readSiteIdentityFile(t, environment, "outputs.tf")
		for _, forbidden := range []string{"var.google_client_id", "var.google_client_secret", "terraform_remote_state"} {
			if strings.Contains(outputs, forbidden) {
				t.Errorf("%s site identity outputs must not expose %q", environment, forbidden)
			}
		}
		main := readSiteIdentityFile(t, environment, "main.tf")
		if want := `session_parameter_path = "/portfolio/lambda/` + environment + `/SITE_SESSION_KEY"`; !strings.Contains(main, want) {
			t.Errorf("%s site identity must name only its own session parameter, want %q", environment, want)
		}
	}
}

func readSiteIdentityFile(t *testing.T, environment, name string) string {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("auth", "site", environment, name))
	if err != nil {
		t.Fatalf("read %s site identity %q: %v", environment, name, err)
	}
	return string(contents)
}

// hclBlock returns the text of one top-level HCL block, such as a variable.
func hclBlock(t *testing.T, contents, kind, name string) string {
	t.Helper()
	header := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(kind) + `\s+"` + regexp.QuoteMeta(name) + `"\s*\{`)
	location := header.FindStringIndex(contents)
	if location == nil {
		t.Fatalf("missing %s block %q", kind, name)
	}
	depth := 0
	for i := location[0]; i < len(contents); i++ {
		switch contents[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return contents[location[0] : i+1]
			}
		}
	}
	t.Fatalf("unterminated %s block %q", kind, name)
	return ""
}
