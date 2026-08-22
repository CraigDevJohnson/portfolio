package lambda

import (
	"errors"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"sort"
	"testing"
)

var (
	outputBlockPattern = regexp.MustCompile(`(?m)^output\s+"([^"]+)"\s+\{`)
	outputAliasPattern = regexp.MustCompile(`(?ms)^output\s+"([^"]+)"\s+\{\s*value\s*=\s*module\.service\.([A-Za-z0-9_]+)\s*\}`)
)

var serviceOutputNames = []string{
	"acm_validation_records",
	"alarm_arns",
	"api_access_log_group_name",
	"api_default_url",
	"api_gateway_domain_targets",
	"api_id",
	"certificate_arn",
	"environment",
	"google_connection_table_arn",
	"google_connection_table_name",
	"image_uri",
	"lambda_alias_arn",
	"lambda_alias_name",
	"lambda_function_arn",
	"lambda_function_name",
	"lambda_log_group_name",
	"lambda_published_version",
	"oauth_redirect_uris",
	"soccer_session_table_arn",
	"soccer_session_table_name",
	"ssm_parameter_paths",
}

func TestLambdaInfrastructureLayout(t *testing.T) {
	required := []string{
		"artifacts/backend.hcl",
		"artifacts/main.tf",
		"environments/dev/backend.hcl",
		"environments/dev/dev.auto.tfvars",
		"environments/dev/main.tf",
		"environments/dev/outputs.tf",
		"environments/dev/providers.tf",
		"environments/dev/variables.tf",
		"environments/dev/versions.tf",
		"environments/prod/backend.hcl",
		"environments/prod/main.tf",
		"environments/prod/outputs.tf",
		"environments/prod/prod.auto.tfvars",
		"environments/prod/providers.tf",
		"environments/prod/variables.tf",
		"environments/prod/versions.tf",
		"modules/service/api.tf",
		"modules/service/domain.tf",
		"modules/service/lambda.tf",
		"modules/service/observability.tf",
		"modules/service/outputs.tf",
		"modules/service/variables.tf",
	}

	for _, path := range required {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("required Lambda infrastructure path %q: %v", path, err)
		}
	}

	for _, path := range []string{
		"infra/lambda/artifacts/.terraform/providers/example",
		"infra/lambda/artifacts/.tofu/providers/example",
		"infra/lambda/artifacts/terraform.tfstate",
		"infra/lambda/artifacts/terraform.tfstate.backup",
		"infra/lambda/artifacts/saved.tfplan",
		"infra/lambda/environments/dev/.terraform/providers/example",
		"infra/lambda/environments/dev/.tofu/providers/example",
		"infra/lambda/environments/dev/terraform.tfstate",
		"infra/lambda/environments/dev/terraform.tfstate.backup",
		"infra/lambda/environments/dev/saved.tfplan",
		"infra/lambda/environments/prod/.terraform/providers/example",
		"infra/lambda/environments/prod/.tofu/providers/example",
		"infra/lambda/environments/prod/terraform.tfstate",
		"infra/lambda/environments/prod/terraform.tfstate.backup",
		"infra/lambda/environments/prod/saved.tfplan",
	} {
		if !gitPathIsIgnored(t, path) {
			t.Errorf("generated OpenTofu path %q must be ignored", path)
		}
	}

	for _, path := range []string{
		"infra/lambda/artifacts/.terraform.lock.hcl",
		"infra/lambda/environments/dev/.terraform.lock.hcl",
		"infra/lambda/environments/prod/.terraform.lock.hcl",
	} {
		if gitPathIsIgnored(t, path) {
			t.Errorf("provider lock file %q must not be ignored", path)
		}
	}

	assertExactTerraformOutputs(t, "modules/service/outputs.tf", serviceOutputNames)
	assertTerraformOutputAliases(t, "environments/dev/outputs.tf", serviceOutputNames)
	assertTerraformOutputAliases(t, "environments/prod/outputs.tf", serviceOutputNames)
	runOpenTofu(t, "modules/service", "init", "-backend=false", "-input=false")
	runOpenTofu(t, "modules/service", "test", "-no-color")
	for _, environment := range []string{"dev", "prod"} {
		directory := "environments/" + environment
		runOpenTofu(t, directory, "init", "-backend=false", "-input=false")
		runOpenTofu(t, directory, "fmt", "-check")
		runOpenTofu(t, directory, "validate")
	}
}

func assertExactTerraformOutputs(t *testing.T, path string, want []string) {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("read Terraform outputs from %q: %v", path, err)
		return
	}

	matches := outputBlockPattern.FindAllStringSubmatch(string(contents), -1)
	got := make([]string, 0, len(matches))
	for _, match := range matches {
		got = append(got, match[1])
	}
	sort.Strings(got)

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Terraform outputs in %q = %v, want exactly %v", path, got, want)
	}
}

func assertTerraformOutputAliases(t *testing.T, path string, want []string) {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("read Terraform output aliases from %q: %v", path, err)
		return
	}

	matches := outputAliasPattern.FindAllStringSubmatch(string(contents), -1)
	got := make([]string, 0, len(matches))
	for _, match := range matches {
		if match[1] != match[2] {
			t.Errorf("Terraform output %q in %q aliases module.service.%s, want the same name", match[1], path, match[2])
		}
		got = append(got, match[1])
	}
	sort.Strings(got)

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Terraform output aliases in %q = %v, want exactly %v", path, got, want)
	}
}

func runOpenTofu(t *testing.T, directory string, args ...string) {
	t.Helper()

	commandArgs := append([]string{"-chdir=" + directory}, args...)
	command := exec.Command("tofu", commandArgs...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("tofu %v in %q: %v\n%s", args, directory, err, output)
	}
}

func gitPathIsIgnored(t *testing.T, path string) bool {
	t.Helper()

	command := exec.Command("git", "check-ignore", "--no-index", "--quiet", path)
	command.Dir = "../.."
	err := command.Run()
	if err == nil {
		return true
	}

	var exitError *exec.ExitError
	if errors.As(err, &exitError) && exitError.ExitCode() == 1 {
		return false
	}

	t.Fatalf("check whether %q is ignored: %v", path, err)
	return false
}
