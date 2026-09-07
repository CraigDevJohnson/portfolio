package lambda

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const authRoot = "auth/dev"

func TestCognitoAuthBackendIsIsolatedAndEncrypted(t *testing.T) {
	backend := readAuthFile(t, "backend.hcl")

	for _, setting := range []string{
		`bucket       = "portfolio-tofu-state-180294223248"`,
		`key          = "portfolio-lambda-http-api/auth/dev/terraform.tfstate"`,
		`region       = "us-west-2"`,
		`encrypt      = true`,
		`use_lockfile = true`,
	} {
		if !strings.Contains(backend, setting) {
			t.Errorf("auth backend is missing %q", setting)
		}
	}
}

func TestCognitoAuthCredentialsAreRequiredSensitiveInputsOnly(t *testing.T) {
	variables := readAuthFile(t, "variables.tf")
	for _, name := range []string{"google_client_id", "google_client_secret"} {
		block := authHCLBlock(t, variables, "variable", name)
		if !regexp.MustCompile(`(?m)^\s*sensitive\s*=\s*true\s*$`).MatchString(block) {
			t.Errorf("variable %q must be sensitive", name)
		}
		if regexp.MustCompile(`(?m)^\s*default\s*=`).MatchString(block) {
			t.Errorf("variable %q must not have a default", name)
		}
	}

	allOutputFiles, err := filepath.Glob(filepath.Join(authRoot, "*.tf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range allOutputFiles {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(contents), "terraform_remote_state") {
			t.Errorf("auth root must not access remote state: %s", path)
		}
	}
}

func TestCognitoAuthRootContainsOnlyApprovedResourcesAndOutputs(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(authRoot, "*.tf"))
	if err != nil {
		t.Fatal(err)
	}
	resourcePattern := regexp.MustCompile(`(?m)^resource\s+"([^"]+)"\s+"([^"]+)"\s*\{`)
	outputPattern := regexp.MustCompile(`(?m)^output\s+"([^"]+)"\s*\{`)
	resources := make([]string, 0)
	outputs := make([]string, 0)
	for _, path := range files {
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		for _, match := range resourcePattern.FindAllStringSubmatch(string(contents), -1) {
			resources = append(resources, match[1]+"."+match[2])
		}
		for _, match := range outputPattern.FindAllStringSubmatch(string(contents), -1) {
			outputs = append(outputs, match[1])
		}
	}
	sort.Strings(resources)
	sort.Strings(outputs)

	wantResources := []string{
		"aws_cognito_identity_provider.google",
		"aws_cognito_managed_login_branding.management",
		"aws_cognito_user_pool.management",
		"aws_cognito_user_pool_client.management",
		"aws_cognito_user_pool_domain.management",
	}
	wantOutputs := []string{
		"cognito_client_id",
		"cognito_domain",
		"cognito_issuer",
		"cognito_user_pool_id",
		"google_redirect_uri",
		"management_runtime",
		"session_parameter_path",
	}
	if strings.Join(resources, "\n") != strings.Join(wantResources, "\n") {
		t.Errorf("auth managed resources = %v, want exactly %v", resources, wantResources)
	}
	if strings.Join(outputs, "\n") != strings.Join(wantOutputs, "\n") {
		t.Errorf("auth outputs = %v, want exactly %v", outputs, wantOutputs)
	}

	outputsFile := readAuthFile(t, "outputs.tf")
	for _, forbidden := range []string{"google_client_id", "google_client_secret", "session_key", "MGMT_SESSION_KEY="} {
		if strings.Contains(outputsFile, "var."+forbidden) || strings.Contains(outputsFile, "local."+forbidden+"_value") {
			t.Errorf("auth outputs must not expose secret-bearing value %q", forbidden)
		}
	}
}

func TestCognitoAuthProviderIsPinnedToDevelopmentAccountAndRegion(t *testing.T) {
	provider := readAuthFile(t, "providers.tf")
	for _, contract := range []string{
		`region              = "us-west-2"`,
		`allowed_account_ids = ["180294223248"]`,
	} {
		if !strings.Contains(provider, contract) {
			t.Errorf("auth provider is missing %q", contract)
		}
	}
}

func readAuthFile(t *testing.T, name string) string {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(authRoot, name))
	if err != nil {
		t.Fatalf("read auth root %q: %v", name, err)
	}
	return string(contents)
}

func authHCLBlock(t *testing.T, contents, kind, name string) string {
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
