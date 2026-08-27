package lambda

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

var serviceOutputTypes = map[string]any{
	"acm_validation_records": []any{"list", []any{"object", map[string]any{
		"domain_name":           "string",
		"resource_record_name":  "string",
		"resource_record_type":  "string",
		"resource_record_value": "string",
	}}},
	"alarm_arns":                   []any{"list", "string"},
	"alarm_names":                  []any{"list", "string"},
	"api_access_log_group_name":    "string",
	"api_default_url":              "string",
	"api_gateway_domain_targets":   []any{"map", "string"},
	"api_id":                       "string",
	"api_name":                     "string",
	"certificate_arn":              "string",
	"environment":                  "string",
	"google_connection_table_arn":  "string",
	"google_connection_table_name": "string",
	"image_uri":                    "string",
	"lambda_alias_arn":             "string",
	"lambda_alias_name":            "string",
	"lambda_execution_permissions_boundary_arn": "string",
	"lambda_execution_role_name":                "string",
	"lambda_function_arn":                       "string",
	"lambda_function_name":                      "string",
	"lambda_log_group_name":                     "string",
	"lambda_published_version":                  "string",
	"lambda_runtime_policy_name":                "string",
	"oauth_redirect_uris":                       []any{"list", "string"},
	"soccer_session_table_arn":                  "string",
	"soccer_session_table_name":                 "string",
	"ssm_parameter_paths":                       []any{"map", "string"},
}

var artifactOutputTypes = map[string]any{
	"ecr_repository_arn":  "string",
	"ecr_repository_name": "string",
	"ecr_repository_url":  "string",
}

var serviceIAMResourceCounts = map[string]int{
	"aws_iam_role":        1,
	"aws_iam_role_policy": 1,
}

type plannedResource struct {
	Mode string `json:"mode"`
	Type string `json:"type"`
}

type plannedModule struct {
	Resources    []plannedResource `json:"resources"`
	ChildModules []plannedModule   `json:"child_modules"`
}

func TestLambdaInfrastructureLayout(t *testing.T) {
	required := []string{
		"artifacts/backend.hcl",
		"artifacts/main.tf",
		"artifacts/tests/artifact_contract.tftest.hcl",
		"environments/dev/backend.hcl",
		"environments/dev/dev.auto.tfvars",
		"environments/dev/main.tf",
		"environments/dev/outputs.tf",
		"environments/dev/providers.tf",
		"environments/dev/tests/environment_contract.tftest.hcl",
		"environments/dev/variables.tf",
		"environments/dev/versions.tf",
		"environments/prod/backend.hcl",
		"environments/prod/main.tf",
		"environments/prod/outputs.tf",
		"environments/prod/prod.auto.tfvars",
		"environments/prod/providers.tf",
		"environments/prod/tests/environment_contract.tftest.hcl",
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

	runOpenTofu(t, "artifacts", "init", "-backend=false", "-input=false")
	runOpenTofu(t, "artifacts", "fmt", "-check")
	runOpenTofu(t, "artifacts", "validate")
	runOpenTofuTest(t, "artifacts", 1, artifactOutputTypes, nil)

	runOpenTofu(t, "modules/service", "init", "-backend=false", "-input=false")
	runOpenTofuTest(t, "modules/service", 4, serviceOutputTypes, serviceIAMResourceCounts)
	for _, environment := range []string{"dev", "prod"} {
		directory := "environments/" + environment
		runOpenTofu(t, directory, "init", "-backend=false", "-input=false")
		runOpenTofu(t, directory, "fmt", "-check")
		runOpenTofu(t, directory, "validate")
		runOpenTofuTest(t, directory, 1, serviceOutputTypes, serviceIAMResourceCounts)
	}
}

func runOpenTofuTest(t *testing.T, directory string, wantPlans int, outputTypes map[string]any, iamResourceCounts map[string]int) {
	t.Helper()

	command := exec.Command("tofu", "-chdir="+directory, "test", "-json", "-verbose", "-no-color")
	command.Env = terraformTestEnvironment()
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("tofu test in %q: %v\n%s", directory, err, output)
	}

	planCount := 0
	summaryPassed := false
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		var event struct {
			Type     string `json:"type"`
			TestRun  string `json:"@testrun"`
			TestPlan *struct {
				PlannedValues struct {
					Outputs map[string]struct {
						Type any `json:"type"`
					} `json:"outputs"`
					RootModule plannedModule `json:"root_module"`
				} `json:"planned_values"`
			} `json:"test_plan"`
			TestSummary *struct {
				Status  string `json:"status"`
				Failed  int    `json:"failed"`
				Errored int    `json:"errored"`
			} `json:"test_summary"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("decode tofu test event in %q: %v\n%s", directory, err, scanner.Bytes())
		}

		if event.TestPlan != nil {
			planCount++
			assertOutputTypes(t, directory, event.TestRun, event.TestPlan.PlannedValues.Outputs, outputTypes)
			assertManagedIAMResources(t, directory, event.TestRun, event.TestPlan.PlannedValues.RootModule, iamResourceCounts)
		}
		if event.TestSummary != nil {
			summaryPassed = event.TestSummary.Status == "pass" && event.TestSummary.Failed == 0 && event.TestSummary.Errored == 0
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan tofu test events in %q: %v", directory, err)
	}
	if planCount != wantPlans {
		t.Errorf("tofu test plans in %q = %d, want %d", directory, planCount, wantPlans)
	}
	if !summaryPassed {
		t.Errorf("tofu test summary in %q did not report a clean pass", directory)
	}
}

func assertManagedIAMResources(t *testing.T, directory, run string, module plannedModule, wantCounts map[string]int) {
	t.Helper()

	gotCounts := make(map[string]int)
	collectManagedIAMResources(module, gotCounts)

	for resourceType, count := range gotCounts {
		if _, ok := wantCounts[resourceType]; !ok {
			t.Errorf("evaluated plan for %q run %q contains unapproved managed IAM resource type %q (%d)", directory, run, resourceType, count)
		}
	}
	for resourceType, wantCount := range wantCounts {
		if gotCount := gotCounts[resourceType]; gotCount != wantCount {
			t.Errorf("evaluated plan for %q run %q contains %d managed %q resources, want exactly %d", directory, run, gotCount, resourceType, wantCount)
		}
	}
}

func collectManagedIAMResources(module plannedModule, counts map[string]int) {
	for _, resource := range module.Resources {
		if resource.Mode == "managed" && strings.HasPrefix(resource.Type, "aws_iam_") {
			counts[resource.Type]++
		}
	}
	for _, childModule := range module.ChildModules {
		collectManagedIAMResources(childModule, counts)
	}
}

func assertOutputTypes(t *testing.T, directory, run string, outputs map[string]struct {
	Type any `json:"type"`
},
	wantOutputs map[string]any,
) {
	t.Helper()

	if len(outputs) != len(wantOutputs) {
		t.Errorf("evaluated outputs for %q run %q contain %d names, want exactly %d", directory, run, len(outputs), len(wantOutputs))
	}
	for name, wantType := range wantOutputs {
		output, ok := outputs[name]
		if !ok {
			t.Errorf("evaluated outputs for %q run %q are missing %q", directory, run, name)
			continue
		}
		if !reflect.DeepEqual(output.Type, wantType) {
			t.Errorf("evaluated output %q for %q run %q has type %v, want %v", name, directory, run, output.Type, wantType)
		}
	}
}

func terraformTestEnvironment() []string {
	environment := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "TF_VAR_ecr_repository_url=") ||
			strings.HasPrefix(entry, "TF_VAR_image_digest=") ||
			strings.HasPrefix(entry, "TF_VAR_alarm_action_arns=") {
			continue
		}
		environment = append(environment, entry)
	}

	return append(environment,
		"TF_VAR_ecr_repository_url=180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases",
		"TF_VAR_image_digest=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	)
}

func runOpenTofu(t *testing.T, directory string, args ...string) {
	t.Helper()

	commandArgs := append([]string{"-chdir=" + directory}, args...)
	command := exec.Command("tofu", commandArgs...)
	command.Env = terraformTestEnvironment()
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
